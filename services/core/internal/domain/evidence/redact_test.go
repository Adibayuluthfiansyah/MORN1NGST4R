package evidence_test

import (
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
)

// The secrets below are built at run time, so that no source line looks like a
// real credential to a scanner.
var (
	awsKey    = "AK" + "IA" + strings.Repeat("Q", 16)
	ghToken   = "gh" + "p_" + strings.Repeat("a1", 20)
	jwtValue  = "ey" + "J" + strings.Repeat("a", 12) + "." + "ey" + "J" + strings.Repeat("b", 12) + "." + strings.Repeat("c", 12)
	slackTok  = "xo" + "xb-" + strings.Repeat("1", 12) + "-" + strings.Repeat("a", 12)
	pemBegin  = "-----" + "BEGIN " + "PRIVATE KEY" + "-----"
	pemEnd    = "-----" + "END " + "PRIVATE KEY" + "-----"
	longToken = strings.Repeat("Z9", 14)
)

func TestRedactionRemovesWhatD77Names(t *testing.T) {
	for name, c := range map[string]struct{ in, secret string }{
		"authorization header": {"GET / HTTP/1.1\r\nHost: a\r\nAuthorization: Bearer " + longToken + "\r\n\r\n", longToken},
		"basic authorization":  {"Authorization: Basic dXNlcjpwYXNzd29yZA==\r\n", "dXNlcjpwYXNzd29yZA=="},
		"proxy authorization":  {"Proxy-Authorization: secretvalue1\r\n", "secretvalue1"},
		"cookie header":        {"Cookie: sid=" + longToken + "; theme=dark\r\n", longToken},
		"set cookie header":    {"HTTP/1.1 200 OK\r\nSet-Cookie: session=" + longToken + "; HttpOnly\r\n", longToken},
		"api key header":       {"X-Api-Key: " + longToken + "\r\n", longToken},
		"lower case header":    {"authorization: token " + longToken + "\r\n", longToken},
		"bearer in a sentence": {"the request sent bearer " + longToken + " twice", longToken},
		"form body password":   {"user=a&password=" + longToken + "&x=1", longToken},
		"json body password":   {`{"user":"a","password":"` + longToken + `"}`, longToken},
		"json token":           {`{"access_token": "` + longToken + `", "n": 1}`, longToken},
		"secret in a query":    {"GET /cb?client_secret=" + longToken + "&state=x HTTP/1.1", longToken},
		"credentials in a URL": {"fetching https://admin:" + longToken + "@example.com/path", longToken},
		"aws access key":       {"key " + awsKey + " found", awsKey},
		"github token":         {"token " + ghToken + " found", ghToken},
		"jwt":                  {"value " + jwtValue + " here", jwtValue},
		"slack token":          {"hook " + slackTok + " here", slackTok},
		"private key block":    {"before\n" + pemBegin + "\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n" + pemEnd + "\nafter", "MIIEvQIBADANBgkqhkiG9w0BAQEFAASC"},

		"single quoted password":      {"password='" + longToken + "'", longToken},
		"single quoted python dict":   {"{'password': '" + longToken + "', 'user': 'a'}", longToken},
		"single quoted client secret": {"client_secret = '" + longToken + "'", longToken},
		"json cookie header map":      {`{"cookie":"sid=` + longToken + `","accept":"*/*"}`, longToken},
		"json set-cookie header map":  {`{"set-cookie":"sid=` + longToken + `; Path=/"}`, longToken},
		"json authorization map":      {`{"authorization":"Token ` + longToken + `"}`, longToken},
		"json proxy authorization":    {`{"proxy-authorization":"NTLM ` + longToken + `"}`, longToken},
		"json underscore header":      {`{"set_cookie":"sid=` + longToken + `"}`, longToken},
		"json header, mixed case":     {`{"Authorization": "Digest ` + longToken + `"}`, longToken},
		"quoted cookie value":         {"Cookie: session=\"" + longToken + "\"\r\n", longToken},
		"digest authorization":        {"Authorization: Digest username=\"bob\", response=\"" + longToken + "\"\r\n", longToken},
		"underscore header name":      {"set_cookie: sid=" + longToken + "\r\n", longToken},
		"header in a command":         {`curl -H "Authorization: Bearer ` + longToken + `" https://example.com`, longToken},
	} {
		out, n := evidence.Redact([]byte(c.in))
		if strings.Contains(string(out), c.secret) {
			t.Errorf("%s: the secret survived in %q", name, out)
		}
		if n < 1 {
			t.Errorf("%s: %d redactions", name, n)
		}
	}
}

func TestRedactionKeepsTheRestOfTheEvidence(t *testing.T) {
	in := "GET /index HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test\r\nAccept: */*\r\n\r\nHTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html>hello</html>"
	out, n := evidence.Redact([]byte(in))
	if string(out) != in || n != 0 {
		t.Fatalf("out = %q, %d redactions", out, n)
	}
}

func TestRedactionOfAJSONHeaderMapKeepsTheOtherKeys(t *testing.T) {
	in := `{"url":"https://a.example/","header":{"cookie":"sid=` + longToken + `","accept":"*/*"},"status":200}`
	out, n := evidence.Redact([]byte(in))
	want := `{"url":"https://a.example/","header":{"cookie":"[REDACTED]","accept":"*/*"},"status":200}`
	if string(out) != want || n != 1 {
		t.Fatalf("out = %q (%d), want %q", out, n, want)
	}
}

func TestRedactionKeepsTheNamesOfWhatItRemoves(t *testing.T) {
	out, _ := evidence.Redact([]byte("Authorization: Bearer " + longToken + "\r\nCookie: a=" + longToken + "\r\n"))
	for _, want := range []string{"Authorization:", "Cookie:", "[REDACTED]"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("%q lacks %q", out, want)
		}
	}
	if strings.Contains(string(out), longToken) {
		t.Errorf("%q", out)
	}
}

func TestRedactingTwiceChangesNothing(t *testing.T) {
	once, _ := evidence.Redact([]byte("password=" + longToken + "\r\nAuthorization: Bearer " + longToken + "\r\n" + pemBegin + "\nabc\n" + pemEnd))
	twice, n := evidence.Redact(once)
	if string(twice) != string(once) || n != 0 {
		t.Fatalf("second pass changed the text: %q -> %q (%d)", once, twice, n)
	}
}

// A line that is very long must not make the redactor slow or blow it up.
func TestRedactionOfHostileInputIsBounded(t *testing.T) {
	in := strings.Repeat("password=", 20000) + strings.Repeat("a", 1<<20) + strings.Repeat("Authorization:", 20000)
	out, _ := evidence.Redact([]byte(in))
	if len(out) > 4*len(in) {
		t.Fatalf("output of %d bytes for %d", len(out), len(in))
	}
}

func TestBinaryContentIsLeftAloneExceptForTheSecretsInIt(t *testing.T) {
	in := append([]byte{0x00, 0xff, 0xfe, 0x01}, []byte(" password="+longToken+" ")...)
	out, n := evidence.Redact(in)
	if n != 1 || strings.Contains(string(out), longToken) || out[0] != 0x00 || out[1] != 0xff {
		t.Fatalf("out = %q (%d)", out, n)
	}
}
