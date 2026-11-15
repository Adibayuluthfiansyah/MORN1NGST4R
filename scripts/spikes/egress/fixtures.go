package egress

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Fixture is a local HTTP or HTTPS server that counts connections/requests
// so tests can prove whether a target was contacted.
type Fixture struct {
	Name  string
	Ln    net.Listener
	Srv   *http.Server
	conns atomic.Int64
	reqs  atomic.Int64

	mu       sync.Mutex
	ConnFrom []string // client remote addr of each accepted connection
	Seen     []string // request summary lines
}

// StartFixture serves on network/addr. tlsCfg==nil serves plain HTTP.
// h2 enables HTTP/2 (only meaningful with TLS).
func StartFixture(name, network, addr string, tlsCfg *tls.Config, h2 bool) (*Fixture, error) {
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}
	f := &Fixture{Name: name, Ln: ln}
	mux := http.NewServeMux()
	mux.HandleFunc("/", f.echo)
	mux.HandleFunc("/ok", f.echo)
	mux.HandleFunc("/r", f.redirect)
	mux.HandleFunc("/loop/a", func(w http.ResponseWriter, r *http.Request) { f.hit(r); setRedirect(w, 302, "/loop/b") })
	mux.HandleFunc("/loop/b", func(w http.ResponseWriter, r *http.Request) { f.hit(r); setRedirect(w, 302, "/loop/a") })
	f.Srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ErrorLog: log.New(io.Discard, "", 0),
		ConnState: func(c net.Conn, s http.ConnState) {
			if s == http.StateNew {
				f.conns.Add(1)
				f.mu.Lock()
				f.ConnFrom = append(f.ConnFrom, c.RemoteAddr().String())
				f.mu.Unlock()
			}
		}}
	if tlsCfg != nil {
		f.Srv.TLSConfig = tlsCfg.Clone()
		if !h2 {
			f.Srv.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
		}
		go f.Srv.ServeTLS(ln, "", "")
	} else {
		go f.Srv.Serve(ln)
	}
	return f, nil
}

func (f *Fixture) Port() int {
	return f.Ln.Addr().(*net.TCPAddr).Port
}
func (f *Fixture) Conns() int64 { return f.conns.Load() }
func (f *Fixture) Reqs() int64  { return f.reqs.Load() }
func (f *Fixture) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	f.Srv.Shutdown(ctx)
	f.Srv.Close()
}

func (f *Fixture) hit(r *http.Request) {
	f.reqs.Add(1)
	sni := ""
	if r.TLS != nil {
		sni = r.TLS.ServerName
	}
	f.mu.Lock()
	f.Seen = append(f.Seen, fmt.Sprintf("%s %s host=%s sni=%s proto=%s from=%s", r.Method, r.URL.RequestURI(), r.Host, sni, r.Proto, r.RemoteAddr))
	f.mu.Unlock()
}

func (f *Fixture) echo(w http.ResponseWriter, r *http.Request) {
	f.hit(r)
	sni := ""
	if r.TLS != nil {
		sni = r.TLS.ServerName
	}
	fmt.Fprintf(w, "fixture=%s host=%s path=%s remote=%s proto=%s sni=%s\n", f.Name, r.Host, r.URL.Path, r.RemoteAddr, r.Proto, sni)
}

// setRedirect writes a raw Location value (no normalization by net/http).
func setRedirect(w http.ResponseWriter, code int, loc string) {
	w.Header()["Location"] = []string{loc}
	w.WriteHeader(code)
}

// /r?code=302&loc=<raw location>
func (f *Fixture) redirect(w http.ResponseWriter, r *http.Request) {
	f.hit(r)
	code, _ := strconv.Atoi(r.URL.Query().Get("code"))
	if code == 0 {
		code = 302
	}
	setRedirect(w, code, r.URL.Query().Get("loc"))
}
