// Command hostside runs the host-side half of the namespace experiments (E7/E8):
// mock DNS, local fixtures, an abstract Unix socket, an admin endpoint and the
// policy proxy listening on a path-based Unix socket. Everything is loopback or
// a socket under the scratch directory.
package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"spike/egress"
)

func must[T any](v T, err error) T {
	if err != nil {
		fmt.Println("FATAL:", err)
		os.Exit(3)
	}
	return v
}

func main() {
	dir := os.Getenv("MS_SPIKE_TMP")
	if dir == "" {
		fmt.Println("FATAL: MS_SPIKE_TMP must be set to /tmp/ms-spike-egress-*")
		os.Exit(2)
	}
	dir = must(egress.ScratchDir())

	dns := must(egress.NewDNSServer())
	defer dns.Close()
	dns.Set("allowed.test", egress.Step{A: []string{"127.0.0.1"}})
	dns.Set("deny.test", egress.Step{A: []string{"10.0.0.5"}})
	dns.Set("mixed.test", egress.Step{A: []string{"127.0.0.1", "10.0.0.5"}})

	pki := must(egress.NewPKI([]string{"allowed.test"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}))
	must(egress.WriteScratch(dir, "ca.pem", pki.CAPEM))
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{pki.Leaf}}

	allowed := must(egress.StartFixture("ALLOWED", "tcp", "127.0.0.1:0", nil, false))
	pa := allowed.Port()
	allowed6 := must(egress.StartFixture("ALLOWED-v6", "tcp", fmt.Sprintf("[::1]:%d", pa), nil, false))
	tlsF := must(egress.StartFixture("TLS-ALLOWED", "tcp", "127.0.0.1:0", tlsCfg, false))
	forb := must(egress.StartFixture("FORBIDDEN", "tcp", "127.0.0.1:0", nil, false))
	pt, pf := tlsF.Port(), forb.Port()

	abs := fmt.Sprintf("@ms-spike-egress-abs-%d", os.Getpid())
	aln := must(net.Listen("unix", abs))
	absHits := 0
	go func() {
		for {
			c, err := aln.Accept()
			if err != nil {
				return
			}
			absHits++
			c.Write([]byte("ABSTRACT-SOCKET-OK\n"))
			c.Close()
		}
	}()

	policy := egress.NewPolicy(true, pa, pt) // forbidden fixture, admin, DNS port, abstract socket: not allowed
	prx := &egress.Proxy{Policy: policy, Resolver: dns.Resolver()}
	sock := filepath.Join(dir, "proxy.sock")
	os.Remove(sock)
	uln := must(net.Listen("unix", sock))
	os.Chmod(sock, 0o600)
	prx.Serve(uln)

	mux := http.NewServeMux()
	mux.HandleFunc("/hits", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "allowed=%d allowed_v6=%d tls=%d forbidden=%d abstract=%d\n", allowed.Conns(), allowed6.Conns(), tlsF.Conns(), forb.Conns(), absHits)
	})
	mux.HandleFunc("/proxylog", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, strings.Join(prx.Log(), "\n"))
	})
	adm := must(net.Listen("tcp", "127.0.0.1:0"))
	go http.Serve(adm, mux)
	padm := adm.Addr().(*net.TCPAddr).Port
	dnsPort := dns.Addr()[strings.LastIndex(dns.Addr(), ":")+1:]

	env := fmt.Sprintf("PA=%d\nPT=%d\nPF=%d\nPADM=%d\nDNSADDR=%s\nDNSPORT=%s\nABSNAME=%s\nSOCK=%s\nCA=%s\nHOSTSIDE_PID=%d\n",
		pa, pt, pf, padm, dns.Addr(), dnsPort, strings.TrimPrefix(abs, "@"), sock, filepath.Join(dir, "ca.pem"), os.Getpid())
	must(egress.WriteScratch(dir, "hostside.env", []byte(env)))
	must(egress.WriteScratch(dir, "hostside.ready", []byte("ready\n")))
	fmt.Printf("hostside ready: %s", strings.ReplaceAll(env, "\n", " "))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-sig:
	case <-time.After(5 * time.Minute): // hard stop
	}
	uln.Close()
	aln.Close()
	os.Remove(sock)
	fmt.Println("hostside exiting")
}
