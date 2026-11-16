package bootstrap_test

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// rawExchange sends raw bytes to the server and returns the status code and
// the body of the first response. It is how a client that is not a browser
// can phrase a request: absolute-form targets, duplicate headers, HTTP/1.0.
func rawExchange(t *testing.T, addr, request string) (int, string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	_, _ = body.ReadFrom(resp.Body)
	return resp.StatusCode, body.String()
}

func hostPort(s running) string { return strings.TrimPrefix(s.url, "http://") }

func TestRequestFormsAndHostAmbiguity(t *testing.T) {
	s := start(t, nil)
	addr := hostPort(s)
	const path = "/api/v1/health/live"

	tests := []struct {
		name    string
		request string
		want    []int // acceptable statuses. Never 200 for a denied host.
	}{
		// Origin-form with the right Host is the normal case.
		{"origin-form, allowed Host", "GET " + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{200}},
		{"origin-form, denied Host", "GET " + path + " HTTP/1.1\r\nHost: evil.example\r\nConnection: close\r\n\r\n", []int{403}},

		// RFC 9112 3.2.2: with an absolute-form target the server ignores the
		// Host header and uses the authority of the target. The allow-list is
		// applied to that effective host, so the target decides.
		{"absolute-form to a denied host, allowed Host header", "GET http://evil.example" + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{403}},
		{"absolute-form to the allowed host", "GET http://" + addr + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{200}},
		{"absolute-form to the allowed host, denied Host header", "GET http://" + addr + path + " HTTP/1.1\r\nHost: evil.example\r\nConnection: close\r\n\r\n", []int{200}},
		{"absolute-form, userinfo trick towards a denied host", "GET http://" + addr + "@evil.example" + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{403, 400}},
		{"absolute-form, allowed host as userinfo of a denied host", "GET http://user" + ":pw@evil.example" + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{403, 400}},
		{"absolute-form, denied host with an allowed prefix", "GET http://" + addr + ".evil.example" + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{403, 400}},
		{"absolute-form without a path", "GET http://evil.example HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{403, 400}},
		{"absolute-form with another scheme", "GET ftp://evil.example" + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{403, 400}},

		// Duplicate and malformed Host headers must not reach a handler.
		{"duplicate Host, allowed first", "GET " + path + " HTTP/1.1\r\nHost: " + addr + "\r\nHost: evil.example\r\nConnection: close\r\n\r\n", []int{400}},
		{"duplicate Host, denied first", "GET " + path + " HTTP/1.1\r\nHost: evil.example\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{400}},
		{"duplicate identical Host", "GET " + path + " HTTP/1.1\r\nHost: " + addr + "\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{400}},
		{"HTTP/1.1 without Host", "GET " + path + " HTTP/1.1\r\nConnection: close\r\n\r\n", []int{400}},
		{"HTTP/1.1 with an empty Host", "GET " + path + " HTTP/1.1\r\nHost:\r\nConnection: close\r\n\r\n", []int{400, 403}},
		{"Host with a list", "GET " + path + " HTTP/1.1\r\nHost: " + addr + ", evil.example\r\nConnection: close\r\n\r\n", []int{400, 403}},
		{"Host with a space", "GET " + path + " HTTP/1.1\r\nHost: " + addr + " evil.example\r\nConnection: close\r\n\r\n", []int{400, 403}},
		{"Host with userinfo", "GET " + path + " HTTP/1.1\r\nHost: user@" + addr + "\r\nConnection: close\r\n\r\n", []int{400, 403}},
		{"Host with a path", "GET " + path + " HTTP/1.1\r\nHost: " + addr + "/x\r\nConnection: close\r\n\r\n", []int{400, 403}},
		{"Host with a trailing dot", "GET " + path + " HTTP/1.1\r\nHost: localhost.:" + strings.Split(addr, ":")[1] + "\r\nConnection: close\r\n\r\n", []int{403}},
		{"obsolete line folding in Host", "GET " + path + " HTTP/1.1\r\nHost: " + addr + "\r\n evil.example\r\nConnection: close\r\n\r\n", []int{400, 403}},
		{"X-Forwarded-Host is not trusted", "GET " + path + " HTTP/1.1\r\nHost: evil.example\r\nX-Forwarded-Host: " + addr + "\r\nConnection: close\r\n\r\n", []int{403}},
		{"Forwarded is not trusted", "GET " + path + " HTTP/1.1\r\nHost: evil.example\r\nForwarded: host=" + addr + "\r\nConnection: close\r\n\r\n", []int{403}},

		// HTTP/1.0 has no mandatory Host. No Host means no allowed Host.
		{"HTTP/1.0 without Host", "GET " + path + " HTTP/1.0\r\n\r\n", []int{403}},
		{"HTTP/1.0 with an allowed Host", "GET " + path + " HTTP/1.0\r\nHost: " + addr + "\r\n\r\n", []int{200}},

		// Other request forms must fail safely and never answer as health.
		// net/http answers "OPTIONS *" itself, before any handler, unless
		// DisableGeneralOptionsHandler is set. That would bypass the Host
		// allow-list, so the server must route it like any other request.
		{"asterisk-form, allowed Host", "OPTIONS * HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{404, 405}},
		{"asterisk-form, denied Host", "OPTIONS * HTTP/1.1\r\nHost: evil.example\r\nConnection: close\r\n\r\n", []int{403}},
		{"CONNECT to the allowed authority", "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{400, 403, 404, 405}},
		{"CONNECT to a denied authority", "CONNECT evil.example:443 HTTP/1.1\r\nHost: evil.example:443\r\nConnection: close\r\n\r\n", []int{400, 403, 404, 405}},
		{"TRACE", "TRACE " + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{405}},
		{"unknown method", "BREW " + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{405, 400, 501}},
		{"request target with a space", "GET /api/v1/health/live x HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n", []int{400}},
		{"bare line feed line endings with a denied Host", "GET " + path + " HTTP/1.1\nHost: evil.example\nConnection: close\n\n", []int{403, 400}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := rawExchange(t, addr, tc.request)
			ok := false
			for _, w := range tc.want {
				ok = ok || code == w
			}
			if !ok {
				t.Errorf("status = %d, want one of %v (body %q)", code, tc.want, body)
			}
			if code != 200 && strings.Contains(body, `"status":"ok"`) {
				t.Errorf("a refused request still received the health answer: %q", body)
			}
			if strings.Contains(body, "evil.example") {
				t.Errorf("the denied host is echoed in the response: %q", body)
			}
		})
	}
}

// Whatever a client sends, a denied or malformed request leaves the server
// serving normal requests afterwards.
func TestServerKeepsServingAfterMalformedRequests(t *testing.T) {
	s := start(t, nil)
	addr := hostPort(s)
	for _, junk := range []string{
		"\x00\x01\x02\r\n\r\n",
		"GET\r\n\r\n",
		strings.Repeat("A", 20000) + "\r\n\r\n",
		"GET /" + strings.Repeat("a", 20000) + " HTTP/1.1\r\nHost: " + addr + "\r\n\r\n",
	} {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = conn.Write([]byte(junk))
		_, _ = bufio.NewReader(conn).ReadString('\n')
		conn.Close()
	}
	if code, body := get(t, s.url+"/api/v1/health/live"); code != 200 || body != `{"status":"ok"}` {
		t.Errorf("after junk: %d %s", code, body)
	}
}
