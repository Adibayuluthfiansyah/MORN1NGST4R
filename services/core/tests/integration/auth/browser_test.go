package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/request"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// These tests drive a real headless Chromium against the composed server. They
// cover the two browser behaviors the design could only argue about: DNS
// rebinding (T-55) and cookie sharing between local ports (T-57). They are
// skipped when no browser is installed, unless MORNINGSTAR_REQUIRE_BROWSER_TESTS
// is 1, which turns a missing browser into a failure.

func browserBinary(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	if os.Getenv("MORNINGSTAR_REQUIRE_BROWSER_TESTS") == "1" {
		t.Fatal("MORNINGSTAR_REQUIRE_BROWSER_TESTS=1 and no Chromium was found")
	}
	t.Skip("no Chromium on PATH: browser-level tests did not run")
	return ""
}

// dumpDOM loads page in a fresh headless profile and returns the text of
// <pre id="out"> once the page's script has written it.
func dumpDOM(t *testing.T, page string, resolverRules string) map[string]any {
	t.Helper()
	bin := browserBinary(t)
	profile := t.TempDir()
	args := []string{
		"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--disable-background-networking", "--user-data-dir=" + profile,
		"--virtual-time-budget=20000", "--dump-dom",
	}
	if resolverRules != "" {
		args = append(args, "--host-resolver-rules="+resolverRules)
	}
	args = append(args, page)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		t.Fatalf("browser failed: %v", err)
	}
	m := regexp.MustCompile(`(?s)<pre id="out">(.*?)</pre>`).FindSubmatch(out)
	if m == nil {
		t.Fatalf("no result element in the page:\n%s", out)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(html.UnescapeString(string(m[1]))), &res); err != nil {
		t.Fatalf("the page did not finish (%q): %v", m[1], err)
	}
	return res
}

type seen struct {
	Method, Path, Host, Origin string
	Cookie                     bool
	Status                     int
	AllowOrigin, Code          string
}

// front serves the test pages itself and forwards /api/ to the server with the
// Host header the browser sent, recording what the server saw and answered.
type front struct {
	srv  *httptest.Server
	mu   sync.Mutex
	logs []seen
}

func newFront(t *testing.T, ln net.Listener, api string, pages map[string]string) *front {
	t.Helper()
	target, err := url.Parse(api)
	if err != nil {
		t.Fatal(err)
	}
	f := &front{}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(target); r.Out.Host = r.In.Host },
		ModifyResponse: func(resp *http.Response) error {
			_, hasCookie := requestCookie(resp.Request, request.CookieSession)
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
			resp.Body = io.NopCloser(bytes.NewReader(body))
			var env struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(body, &env)
			f.mu.Lock()
			f.logs = append(f.logs, seen{
				Method: resp.Request.Method, Path: resp.Request.URL.Path, Host: resp.Request.Host,
				Origin: resp.Request.Header.Get("Origin"), Cookie: hasCookie, Status: resp.StatusCode,
				AllowOrigin: resp.Header.Get("Access-Control-Allow-Origin"), Code: env.Error.Code,
			})
			f.mu.Unlock()
			return nil
		},
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	for path, body := range pages {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body))
		})
	}
	f.srv = httptest.NewUnstartedServer(mux)
	f.srv.Listener = ln
	f.srv.Start()
	t.Cleanup(f.srv.Close)
	return f
}

func requestCookie(r *http.Request, name string) (string, bool) {
	c, err := r.Cookie(name)
	if err != nil {
		return "", false
	}
	return c.Value, true
}

func (f *front) seen() []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seen(nil), f.logs...)
}

func listenLoopback(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return ln, port
}

func apiWithAccount(t *testing.T, c *pgtest.Cluster, o options) *app {
	t.Helper()
	c.Fresh(t)
	a := start(t, c, o)
	if r := newBrowser(a).setup(a.secret(), username, goodPass); r.status != 201 {
		t.Fatalf("setup = %d %s", r.status, r.body)
	}
	return a
}

const pageHelpers = `<!doctype html><meta charset="utf-8"><pre id="out">pending</pre><script>
const res = {};
const finish = () => { document.getElementById('out').textContent = JSON.stringify(res); };
const call = async (method, path, body, csrf) => {
  const headers = {};
  if (body) headers['Content-Type'] = 'application/json';
  if (csrf) headers['X-CSRF-Token'] = csrf;
  const r = await fetch(path, {method, credentials: 'same-origin', headers, body: body ? JSON.stringify(body) : undefined});
  let b = null;
  try { b = await r.json(); } catch (e) {}
  return {status: r.status, body: b};
};
`

func TestBrowserDNSRebindingIsRefusedByTheHostCheck(t *testing.T) {
	browserBinary(t)
	c := pgtest.Get(t)
	ln, port := listenLoopback(t)
	a := apiWithAccount(t, c, options{hosts: []string{"127.0.0.1:" + port}})
	page := pageHelpers + `
(async () => {
  try { const r = await call('GET', '/api/v1/session'); res.status = r.status; res.code = r.body && r.body.error && r.body.error.code; res.state = r.body && r.body.state; }
  catch (e) { res.error = e.name; }
  finish();
})();
</script>`
	f := newFront(t, ln, a.url, map[string]string{"/attack.html": page})

	t.Run("control: the allowed Host is served", func(t *testing.T) {
		res := dumpDOM(t, "http://127.0.0.1:"+port+"/attack.html", "")
		if res["status"] != float64(200) || res["state"] != "anonymous" {
			t.Fatalf("control request = %v", res)
		}
	})

	t.Run("rebound name reaches the loopback address and is refused", func(t *testing.T) {
		res := dumpDOM(t, "http://evil.example:"+port+"/attack.html", "MAP evil.example 127.0.0.1")
		if res["status"] != float64(403) || res["code"] != "host_denied" {
			t.Fatalf("rebinding request = %v", res)
		}
		var refused bool
		for _, s := range f.seen() {
			if s.Host == "evil.example:"+port {
				refused = true
				if s.Status != 403 {
					t.Fatalf("the server answered %d to the rebound Host", s.Status)
				}
			}
		}
		if !refused {
			t.Fatal("the rebound request never reached the server, so the test proves nothing")
		}
	})
}

func TestBrowserCookieIsSharedAcrossLocalPortsButNotUsable(t *testing.T) {
	browserBinary(t)
	c := pgtest.Get(t)
	ln, port := listenLoopback(t)
	attackerLn, attackerPort := listenLoopback(t)
	own := "http://localhost:" + port
	attacker := "http://localhost:" + attackerPort
	a := apiWithAccount(t, c, options{
		hosts:   []string{"localhost:" + port},
		origins: []string{webOrigin, own},
	})

	attackPage := fmt.Sprintf(`<!doctype html><meta charset="utf-8"><script>
const out = {};
(async () => {
  try { const r = await fetch(%q + '/api/v1/session', {credentials: 'include'}); out.read = {status: r.status}; }
  catch (e) { out.read = {error: e.name}; }
  try { await fetch(%q + '/api/v1/session/logout', {method: 'POST', mode: 'no-cors', credentials: 'include', headers: {'Content-Type': 'text/plain'}, body: '{}'}); out.post = {sent: true}; }
  catch (e) { out.post = {error: e.name}; }
  out.documentCookie = document.cookie;
  parent.postMessage(out, %q);
})();
</script>`, own, own, own)
	newFront(t, attackerLn, a.url, map[string]string{"/attack.html": attackPage})

	page := pageHelpers + fmt.Sprintf(`
window.addEventListener('message', async (e) => {
  if (e.origin !== %q) return;
  res.attacker = e.data;
  try {
    const s = await call('GET', '/api/v1/session');
    res.afterAttack = s.body.state;
    res.logout = (await call('POST', '/api/v1/session/logout', {}, s.body.csrfToken)).status;
    res.afterLogout = (await call('GET', '/api/v1/session')).body.state;
  } catch (err) { res.error = err.name; }
  finish();
});
(async () => {
  try {
    const s = await call('GET', '/api/v1/session');
    const l = await call('POST', '/api/v1/session/login', {username: %q, password: %q}, s.body.csrfToken);
    res.login = l.status;
    res.documentCookie = document.cookie;
    const f = document.createElement('iframe');
    f.src = %q + '/attack.html';
    document.body.appendChild(f);
  } catch (err) { res.error = err.name; finish(); }
})();
</script>`, attacker, username, goodPass, attacker)
	f := newFront(t, ln, a.url, map[string]string{"/page.html": page})

	res := dumpDOM(t, own+"/page.html", "")
	if res["error"] != nil {
		t.Fatalf("page error: %v", res)
	}
	if res["login"] != float64(201) && res["login"] != float64(200) {
		t.Fatalf("login in the browser = %v", res["login"])
	}
	if strings.Contains(res["documentCookie"].(string), request.CookieSession) {
		t.Fatalf("the session cookie is readable by script: %q", res["documentCookie"])
	}
	att, _ := res["attacker"].(map[string]any)
	if att == nil {
		t.Fatalf("the attacker frame never reported: %v", res)
	}
	if read, _ := att["read"].(map[string]any); read["error"] != "TypeError" {
		t.Fatalf("the cross-origin page could read the response: %v", att["read"])
	}
	if strings.Contains(att["documentCookie"].(string), request.CookieSession) {
		t.Fatalf("the cross-origin page can read the session cookie: %q", att["documentCookie"])
	}
	if res["afterAttack"] != "authenticated" {
		t.Fatalf("the cross-origin request ended the session: %v", res["afterAttack"])
	}
	if res["logout"] != float64(204) || res["afterLogout"] != "anonymous" {
		t.Fatalf("same-origin logout = %v then %v", res["logout"], res["afterLogout"])
	}

	// Cookies do not isolate by port, so the browser did send the session
	// cookie with the other page's requests. The server must have refused the
	// state change and must not allow the other origin to read.
	var sharedGET, refusedPOST bool
	for _, s := range f.seen() {
		if s.Origin != attacker {
			continue
		}
		if s.AllowOrigin != "" {
			t.Fatalf("a CORS header was sent to %s: %q", s.Origin, s.AllowOrigin)
		}
		switch s.Method {
		case "GET":
			sharedGET = s.Cookie
		case "POST":
			if s.Status < 400 {
				t.Fatalf("a state-changing request from %s got %d", s.Origin, s.Status)
			}
			refusedPOST = true
			t.Logf("cross-port POST: cookie sent=%v, answered %d %s", s.Cookie, s.Status, s.Code)
		}
	}
	if !sharedGET {
		t.Log("this browser did not send the session cookie with the cross-port GET")
	}
	if !refusedPOST {
		t.Fatal("the cross-port POST never reached the server, so the test proves nothing about the Origin check")
	}
}
