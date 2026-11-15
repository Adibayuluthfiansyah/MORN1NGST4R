package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"spike/egress"
)

// ---- E3: Happy Eyeballs and net.Dialer.Control

type attempt struct {
	at      time.Duration
	address string
	denied  bool
	via     string
}

type ctl struct {
	mu    sync.Mutex
	start time.Time
	log   []attempt
	deny  func(netip.AddrPort) bool
}

func newCtl(deny func(netip.AddrPort) bool) *ctl { return &ctl{start: time.Now(), deny: deny} }

func (t *ctl) record(via, network, address string) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return err
	}
	denied := t.deny != nil && t.deny(ap)
	t.mu.Lock()
	t.log = append(t.log, attempt{time.Since(t.start), network + " " + address, denied, via})
	t.mu.Unlock()
	if denied {
		return fmt.Errorf("policy: %s denied", address)
	}
	return nil
}

func (t *ctl) Control(network, address string, c syscall.RawConn) error {
	return t.record("Control", network, address)
}
func (t *ctl) ControlContext(ctx context.Context, network, address string, c syscall.RawConn) error {
	return t.record("ControlContext", network, address)
}

func (t *ctl) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var s []string
	for _, a := range t.log {
		d := "ALLOW"
		if a.denied {
			d = "DENY"
		}
		s = append(s, fmt.Sprintf("%s(%s @%dms %s)", a.via, a.address, a.at.Milliseconds(), d))
	}
	if len(s) == 0 {
		return "(no Control call)"
	}
	return strings.Join(s, " ; ")
}

func (t *ctl) n() int { t.mu.Lock(); defer t.mu.Unlock(); return len(t.log) }

type fam struct {
	name string // "v4" or "v6"
	ip   string
}

var v4, v6 = fam{"v4", "127.0.0.1"}, fam{"v6", "::1"}

type env struct {
	port    int
	closers []func()
	acc     map[string]*atomic.Int64
	notes   []string
}

func (e *env) close() {
	for _, f := range e.closers {
		f()
	}
}

func pickPort() int {
	l := must(net.Listen("tcp4", "127.0.0.1:0"))
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

// setup opens listeners per family: "ok" (accepts), "refuse" (nothing
// listens), "hang" (listen backlog 0 with a full accept queue: SYNs are dropped).
func setup(m4, m6 string) *env {
	e := &env{port: pickPort(), acc: map[string]*atomic.Int64{"v4": {}, "v6": {}}}
	for _, f := range []struct {
		fm   fam
		mode string
	}{{v4, m4}, {v6, m6}} {
		addr := net.JoinHostPort(f.fm.ip, fmt.Sprint(e.port))
		switch f.mode {
		case "ok":
			network := "tcp4"
			if f.fm.name == "v6" {
				network = "tcp6"
			}
			l := must(net.Listen(network, addr))
			cnt := e.acc[f.fm.name]
			go func() {
				for {
					c, err := l.Accept()
					if err != nil {
						return
					}
					cnt.Add(1)
					c.Close()
				}
			}()
			e.closers = append(e.closers, func() { l.Close() })
		case "hang":
			closer, note := hangListener(f.fm, e.port)
			e.notes = append(e.notes, f.fm.name+": "+note)
			e.closers = append(e.closers, closer)
		}
	}
	return e
}

func hangListener(f fam, port int) (func(), string) {
	family := syscall.AF_INET
	if f.name == "v6" {
		family = syscall.AF_INET6
	}
	fd := must(syscall.Socket(family, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0))
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	ip := net.ParseIP(f.ip)
	if family == syscall.AF_INET {
		sa := &syscall.SockaddrInet4{Port: port}
		copy(sa.Addr[:], ip.To4())
		if err := syscall.Bind(fd, sa); err != nil {
			panic(err)
		}
	} else {
		sa := &syscall.SockaddrInet6{Port: port}
		copy(sa.Addr[:], ip.To16())
		if err := syscall.Bind(fd, sa); err != nil {
			panic(err)
		}
	}
	if err := syscall.Listen(fd, 0); err != nil {
		panic(err)
	}
	var held []net.Conn
	filled := 0
	for i := 0; i < 4; i++ {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(f.ip, fmt.Sprint(port)), 300*time.Millisecond)
		if err != nil {
			break
		}
		held = append(held, c)
		filled++
	}
	probe, perr := net.DialTimeout("tcp", net.JoinHostPort(f.ip, fmt.Sprint(port)), 300*time.Millisecond)
	note := fmt.Sprintf("hang listener on %s:%d backlog=0, %d filler conns; extra connect -> %v", f.ip, port, filled, perr)
	if probe != nil {
		probe.Close()
		note += " (NOT hanging!)"
	}
	return func() {
		for _, c := range held {
			c.Close()
		}
		syscall.Close(fd)
	}, note
}

func e3() {
	c := egress.NewChecker("E3")
	cx, cancel := ctx()
	defer cancel()
	c.Obs("Go version: %s", goVersion())
	dns := must(egress.NewDNSServer())
	defer dns.Close()
	res := dns.Resolver()
	dns.Set("dual.test", egress.Step{A: []string{"127.0.0.1"}, AAAA: []string{"::1"}})

	ips := must(res.LookupNetIP(cx, "ip", "dual.test"))
	c.Obs("resolver order for dual.test: %v (Go uses the family of the FIRST address as 'primary')", ips)
	P, O := v6, v4
	if ips[0].Is4() {
		P, O = v4, v6
	}
	c.Obs("primary=%s fallback=%s", P.name, O.name)
	m := func(primary, other string) (string, string) {
		if P.name == "v4" {
			return primary, other
		}
		return other, primary
	}
	isFam := func(f fam) func(netip.AddrPort) bool {
		return func(ap netip.AddrPort) bool { return ap.Addr().Is4() == (f.name == "v4") }
	}

	run := func(label string, e *env, d *net.Dialer, tc *ctl, timeout time.Duration) (string, time.Duration, error) {
		cc, cn := context.WithTimeout(cx, timeout)
		defer cn()
		t0 := time.Now()
		tc.start = t0
		conn, err := d.DialContext(cc, "tcp", fmt.Sprintf("dual.test:%d", e.port))
		el := time.Since(t0)
		remote := "-"
		if conn != nil {
			remote = conn.RemoteAddr().String()
			conn.Close()
		}
		time.Sleep(50 * time.Millisecond) // let accept counters settle
		c.Obs("%s: elapsed=%dms remote=%s err=%v", label, el.Milliseconds(), remote, err)
		c.Obs("%s: Control log: %s", label, tc)
		return remote, el, err
	}
	mk := func(tc *ctl, fd time.Duration, timeout time.Duration, useCtx bool) *net.Dialer {
		d := &net.Dialer{Resolver: res, FallbackDelay: fd, Timeout: timeout}
		if useCtx {
			d.ControlContext = tc.ControlContext
		} else {
			d.Control = tc.Control
		}
		return d
	}
	has := func(remote string, f fam) bool {
		return strings.HasPrefix(remote, f.ip) || strings.HasPrefix(remote, "["+f.ip+"]")
	}

	// S1: both listening, default FallbackDelay
	e := setup("ok", "ok")
	tc := newCtl(nil)
	r, _, _ := run("S1 both ok, default FallbackDelay", e, mk(tc, 0, 5*time.Second, true), tc, 6*time.Second)
	c.Check("S1-single-attempt-when-primary-works", tc.n() == 1 && has(r, P), "Control calls=%d remote=%s", tc.n(), r)
	e.close()

	// S2: primary refuses
	m4, m6 := m("refuse", "ok")
	e = setup(m4, m6)
	tc = newCtl(nil)
	r, el, _ := run("S2 primary refuses", e, mk(tc, 0, 5*time.Second, true), tc, 6*time.Second)
	c.Check("S2-control-called-for-each-attempt", tc.n() == 2 && has(r, O), "Control calls=%d remote=%s elapsed=%dms (fallback started immediately on refusal, not after FallbackDelay)", tc.n(), r, el.Milliseconds())
	e.close()

	// S3: primary hangs; FallbackDelay variants
	for _, fd := range []time.Duration{0, 50 * time.Millisecond, 1000 * time.Millisecond} {
		m4, m6 = m("hang", "ok")
		e = setup(m4, m6)
		for _, n := range e.notes {
			c.Obs("setup: %s", n)
		}
		tc = newCtl(nil)
		lbl := fmt.Sprintf("S3 primary hangs, FallbackDelay=%v", fd)
		if fd == 0 {
			lbl += " (default = 300ms)"
		}
		r, el, _ = run(lbl, e, mk(tc, fd, 5*time.Second, true), tc, 6*time.Second)
		want := fd
		if want == 0 {
			want = 300 * time.Millisecond
		}
		okTiming := el >= want-20*time.Millisecond && el < want+400*time.Millisecond
		c.Check(fmt.Sprintf("S3-parallel-fallback-fd=%v", fd), tc.n() == 2 && has(r, O) && okTiming,
			"Control calls=%d (both parallel attempts passed through Control) remote=%s elapsed=%dms", tc.n(), r, el.Milliseconds())
		e.close()
	}

	// S3c: Happy Eyeballs disabled (FallbackDelay<0 => serial dial with split deadline)
	m4, m6 = m("hang", "ok")
	e = setup(m4, m6)
	tc = newCtl(nil)
	r, el, err := run("S3c primary hangs, FallbackDelay=-1 (disabled), Timeout=1s", e, mk(tc, -1, time.Second, true), tc, 6*time.Second)
	c.Check("S3c-serial-short-timeout-starves-second-address", err != nil && tc.n() == 1 && e.acc[O.name].Load() == 0,
		"remote=%s err=%v Control calls=%d: the first address used the whole 1s budget (Go splits remaining time only when >= 2s), the second was never attempted, so nothing connected without a Control call", r, err, tc.n())
	e.close()
	m4, m6 = m("hang", "ok")
	e = setup(m4, m6)
	tc = newCtl(nil)
	r, el, err = run("S3d primary hangs, FallbackDelay=-1 (disabled), Timeout=6s", e, mk(tc, -1, 6*time.Second, true), tc, 8*time.Second)
	c.Check("S3d-serial-dial-also-passes-control", err == nil && tc.n() == 2 && has(r, O) && el > 2*time.Second,
		"Control calls=%d remote=%s elapsed=%dms (first address got half of the remaining time before the serial fallback)", tc.n(), r, el.Milliseconds())
	e.close()

	// S4: Control denies the primary family, both listening
	e = setup("ok", "ok")
	tc = newCtl(isFam(P))
	r, _, err = run("S4 Control DENIES primary family", e, mk(tc, 0, 5*time.Second, true), tc, 6*time.Second)
	c.Check("S4-deny-in-control-then-dialer-falls-back-and-checks-fallback", tc.n() == 2 && has(r, O) && err == nil,
		"a denied attempt is only a failed attempt: dialer moved on to the other address, and Control ran for it too (calls=%d)", tc.n())
	e.close()

	// S5: Control denies both
	e = setup("ok", "ok")
	tc = newCtl(func(netip.AddrPort) bool { return true })
	r, _, err = run("S5 Control DENIES everything", e, mk(tc, 0, 5*time.Second, true), tc, 6*time.Second)
	c.Check("S5-deny-all-no-connection", err != nil && e.acc["v4"].Load()+e.acc["v6"].Load() == 0 && tc.n() == 2,
		"err=%v listener accepts=%d Control calls=%d", err, e.acc["v4"].Load()+e.acc["v6"].Load(), tc.n())
	e.close()

	// S6: both Control and ControlContext set
	e = setup("ok", "ok")
	tc = newCtl(nil)
	d := mk(tc, 0, 5*time.Second, false)
	d.ControlContext = tc.ControlContext
	run("S6 Control and ControlContext both set", e, d, tc, 6*time.Second)
	via := ""
	if len(tc.log) > 0 {
		via = tc.log[0].via
	}
	c.Check("S6-controlcontext-takes-precedence", tc.n() == 1 && via == "ControlContext", "invoked: %s (Control is ignored when ControlContext is set)", tc)
	e.close()

	// S7: pre-resolved single IP literal, no Happy Eyeballs
	e = setup("ok", "ok")
	tc = newCtl(nil)
	d = &net.Dialer{Timeout: 5 * time.Second, ControlContext: tc.ControlContext}
	conn, derr := d.DialContext(cx, "tcp", net.JoinHostPort(P.ip, fmt.Sprint(e.port)))
	if conn != nil {
		conn.Close()
	}
	c.Obs("S7 dial literal %s: err=%v Control log: %s", P.ip, derr, tc)
	c.Check("S7-literal-ip-one-attempt", derr == nil && tc.n() == 1, "calls=%d", tc.n())
	tc = newCtl(isFam(P))
	d = &net.Dialer{Timeout: 5 * time.Second, ControlContext: tc.ControlContext}
	_, derr = d.DialContext(cx, "tcp", net.JoinHostPort(P.ip, fmt.Sprint(e.port)))
	c.Check("S7-literal-ip-denied-no-fallback", derr != nil && tc.n() == 1, "err=%v calls=%d (nothing to fall back to)", derr, tc.n())
	e.close()

	// S8: the first-party pinned dial: validate all, dial validated IPs one at a time
	e = setup("ok", "ok")
	lab := egress.NewPolicy(true, e.port)
	t := lab.Evaluate(cx, res, "dual.test", e.port)
	st := &egress.DialStats{}
	conn, derr = lab.PinnedDial(t, st)(cx, "tcp", fmt.Sprintf("dual.test:%d", e.port))
	remote := "-"
	if conn != nil {
		remote = conn.RemoteAddr().String()
		conn.Close()
	}
	c.Obs("S8 PinnedDial (resolve-once, sequential, Control re-check): remote=%s err=%v events=%v", remote, derr, st.Snapshot())
	c.Check("S8-pinned-dial-sufficient", derr == nil && len(st.Snapshot()) == 2, "one attempt, Control saw the real remote address")
	e.close()
	m4, m6 = m("hang", "ok")
	e = setup(m4, m6)
	lab = egress.NewPolicy(true, e.port)
	t = lab.Evaluate(cx, res, "dual.test", e.port)
	st = &egress.DialStats{}
	t0 := time.Now()
	conn, derr = lab.PinnedDial(t, st)(cx, "tcp", fmt.Sprintf("dual.test:%d", e.port))
	if conn != nil {
		remote = conn.RemoteAddr().String()
		conn.Close()
	}
	c.Obs("S8b PinnedDial with a hanging primary: remote=%s err=%v elapsed=%dms events=%v", remote, derr, time.Since(t0).Milliseconds(), st.Snapshot())
	c.Check("S8b-pinned-dial-cost-of-no-happy-eyeballs", derr == nil && time.Since(t0) > 2*time.Second,
		"serial dial pays the full per-address timeout (3s) when the first validated address hangs; correctness unaffected, latency is the only cost")
	e.close()

	c.Verdict("Control/ControlContext runs for every attempt incl. fallbacks and can deny one; single pinned IP is sufficient")
}
