package egress

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// DNS types used by the mock.
const (
	TypeA     = 1
	TypeCNAME = 5
	TypeAAAA  = 28
)

// Step is one answer round for a name. A mock name has a sequence of steps;
// the Nth query (per name and type) gets step N, the last step repeats.
type Step struct {
	A     []string
	AAAA  []string
	CNAME string
	NX    bool
}

// DNSServer is a minimal UDP DNS server (A, AAAA, CNAME) on 127.0.0.1.
type DNSServer struct {
	pc     *net.UDPConn
	mu     sync.Mutex
	rules  map[string][]Step
	counts map[string]int // name|qtype -> queries seen
	total  map[string]int // name -> queries seen (any type)
	log    []string
	done   chan struct{}
}

// NewDNSServer listens on 127.0.0.1:0.
func NewDNSServer() (*DNSServer, error) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}
	s := &DNSServer{pc: pc, rules: map[string][]Step{}, counts: map[string]int{}, total: map[string]int{}, done: make(chan struct{})}
	go s.serve()
	return s, nil
}

func (s *DNSServer) Addr() string { return s.pc.LocalAddr().String() }

func (s *DNSServer) Close() { s.pc.Close(); <-s.done }

// Set defines the answer steps for a name.
func (s *DNSServer) Set(name string, steps ...Step) {
	s.mu.Lock()
	s.rules[strings.ToLower(name)] = steps
	s.mu.Unlock()
}

// Reset clears query counters (not the rules).
func (s *DNSServer) Reset() {
	s.mu.Lock()
	s.counts = map[string]int{}
	s.total = map[string]int{}
	s.log = nil
	s.mu.Unlock()
}

// Count returns the number of queries (any type) for the question name.
func (s *DNSServer) Count(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total[strings.ToLower(name)]
}

// CountType returns the number of queries of one type for the question name.
func (s *DNSServer) CountType(name string, qtype int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[fmt.Sprintf("%s|%d", strings.ToLower(name), qtype)]
}

// Log returns a copy of the query log.
func (s *DNSServer) Log() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.log...)
}

// Resolver returns a pure-Go resolver that only talks to this server.
func (s *DNSServer) Resolver() *net.Resolver {
	addr := s.Addr()
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 2 * time.Second}
			return d.DialContext(ctx, "udp", addr) // always the mock, over UDP
		},
	}
}

func (s *DNSServer) serve() {
	defer close(s.done)
	buf := make([]byte, 1500)
	for {
		n, from, err := s.pc.ReadFromUDP(buf)
		if err != nil {
			return
		}
		resp := s.handle(buf[:n])
		if resp != nil {
			s.pc.WriteToUDP(resp, from)
		}
	}
}

func readName(b []byte, off int) (string, int, error) {
	var labels []string
	for {
		if off >= len(b) {
			return "", 0, errors.New("short name")
		}
		l := int(b[off])
		off++
		if l == 0 {
			break
		}
		if l&0xC0 != 0 || off+l > len(b) {
			return "", 0, errors.New("compression/bad label in question")
		}
		labels = append(labels, string(b[off:off+l]))
		off += l
	}
	return strings.ToLower(strings.Join(labels, ".")), off, nil
}

func writeName(name string) []byte {
	var out []byte
	for _, l := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if l == "" {
			continue
		}
		out = append(out, byte(len(l)))
		out = append(out, l...)
	}
	return append(out, 0)
}

func rr(name string, typ uint16, rdata []byte) []byte {
	out := writeName(name)
	var h [10]byte
	binary.BigEndian.PutUint16(h[0:], typ)
	binary.BigEndian.PutUint16(h[2:], 1) // IN
	binary.BigEndian.PutUint32(h[4:], 0) // TTL 0
	binary.BigEndian.PutUint16(h[8:], uint16(len(rdata)))
	out = append(out, h[:]...)
	return append(out, rdata...)
}

func (s *DNSServer) handle(q []byte) []byte {
	if len(q) < 12 || binary.BigEndian.Uint16(q[4:]) != 1 {
		return nil
	}
	name, off, err := readName(q, 12)
	if err != nil || off+4 > len(q) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[off:])
	question := q[12 : off+4]

	s.mu.Lock()
	key := fmt.Sprintf("%s|%d", name, qtype)
	round := s.counts[key]
	s.counts[key]++
	s.total[name]++
	s.log = append(s.log, fmt.Sprintf("q %s type=%d round=%d", name, qtype, round))
	rules := s.rules
	s.mu.Unlock()

	rcode := 0
	var answers []byte
	var n int
	steps, known := rules[name]
	if !known || (len(steps) > 0 && steps[min(round, len(steps)-1)].NX) {
		rcode = 3
	} else {
		cur := name
		for depth := 0; depth < 8; depth++ {
			st := rules[cur]
			if len(st) == 0 {
				break
			}
			step := st[min(round, len(st)-1)]
			if step.CNAME != "" {
				answers = append(answers, rr(cur, TypeCNAME, writeName(step.CNAME))...)
				n++
				cur = strings.ToLower(step.CNAME)
				continue
			}
			var list []string
			switch qtype {
			case TypeA:
				list = step.A
			case TypeAAAA:
				list = step.AAAA
			}
			for _, a := range list {
				ip, err := netip.ParseAddr(a)
				if err != nil {
					continue
				}
				if qtype == TypeA && ip.Is4() {
					b := ip.As4()
					answers = append(answers, rr(cur, TypeA, b[:])...)
					n++
				} else if qtype == TypeAAAA && ip.Is6() {
					b := ip.As16()
					answers = append(answers, rr(cur, TypeAAAA, b[:])...)
					n++
				}
			}
			break
		}
	}
	out := make([]byte, 12)
	copy(out, q[0:2])
	binary.BigEndian.PutUint16(out[2:], 0x8580|uint16(rcode)) // QR AA RD RA
	binary.BigEndian.PutUint16(out[4:], 1)
	binary.BigEndian.PutUint16(out[6:], uint16(n))
	out = append(out, question...)
	return append(out, answers...)
}

// BuildQuery builds a standard query (used by the probe and tests).
func BuildQuery(id uint16, name string, qtype uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[0:], id)
	binary.BigEndian.PutUint16(b[2:], 0x0100) // RD
	binary.BigEndian.PutUint16(b[4:], 1)
	b = append(b, writeName(name)...)
	var t [4]byte
	binary.BigEndian.PutUint16(t[0:], qtype)
	binary.BigEndian.PutUint16(t[2:], 1)
	return append(b, t[:]...)
}

// ParseAnswerCount returns rcode and answer count of a response.
func ParseAnswerCount(resp []byte) (rcode, an int, err error) {
	if len(resp) < 12 {
		return 0, 0, errors.New("short response")
	}
	return int(resp[3] & 0x0F), int(binary.BigEndian.Uint16(resp[6:])), nil
}
