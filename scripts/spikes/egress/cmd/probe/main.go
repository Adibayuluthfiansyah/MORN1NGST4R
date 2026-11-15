// Command probe is the in-namespace test tool for E7/E8. Every subcommand
// prints one line "PROBE <OK|FAIL> <name> :: <detail>" per check and exits 0.
// OK means the operation succeeded, FAIL means it did not. Targets are always
// loopback addresses, sockets under the scratch dir, or the RFC 5737
// documentation address 192.0.2.1 (to confirm "no route" only).
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"spike/egress"
)

func out(name string, ok bool, f string, a ...any) {
	s := "FAIL"
	if ok {
		s = "OK"
	}
	fmt.Printf("PROBE %s %s :: %s\n", s, name, fmt.Sprintf(f, a...))
}

func unescape(s string) string {
	r := strings.NewReplacer(`\r`, "\r", `\n`, "\n", `\x00`, "\x00")
	return r.Replace(s)
}

func firstLine(b []byte) string {
	s := string(b)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: probe tcp|udp|dnsudp|dns|abstract|unix|rawsend|rawsock|caps ARGS")
		return
	}
	cmd, arg := os.Args[1], os.Args[2]
	name := cmd + " " + strings.Join(os.Args[2:], " ")
	if len(name) > 90 {
		name = name[:90]
	}
	t0 := time.Now()
	el := func() string { return fmt.Sprintf("elapsed=%dms", time.Since(t0).Milliseconds()) }
	switch cmd {
	case "tcp":
		c, err := net.DialTimeout("tcp", arg, 3*time.Second)
		if err != nil {
			out(name, false, "%v %s", err, el())
			return
		}
		out(name, true, "connected to %s %s", c.RemoteAddr(), el())
		c.Close()
	case "udp":
		c, err := net.Dial("udp", arg)
		if err != nil {
			out(name, false, "dial: %v %s", err, el())
			return
		}
		defer c.Close()
		if _, err := c.Write([]byte("ping")); err != nil {
			out(name, false, "write: %v %s", err, el())
			return
		}
		c.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		b := make([]byte, 512)
		n, err := c.Read(b)
		if err != nil {
			out(name, false, "read: %v %s", err, el())
			return
		}
		out(name, true, "got %d bytes %s", n, el())
	case "dnsudp":
		if len(os.Args) < 4 {
			return
		}
		c, err := net.Dial("udp", arg)
		if err != nil {
			out(name, false, "dial: %v", err)
			return
		}
		defer c.Close()
		c.Write(egress.BuildQuery(0x4242, os.Args[3], egress.TypeA))
		c.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		b := make([]byte, 512)
		n, err := c.Read(b)
		if err != nil {
			out(name, false, "read: %v %s", err, el())
			return
		}
		rc, an, _ := egress.ParseAnswerCount(b[:n])
		out(name, true, "DNS reply rcode=%d answers=%d %s", rc, an, el())
	case "dns":
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		r := &net.Resolver{PreferGo: true} // reads /etc/resolv.conf like a tool would
		addrs, err := r.LookupHost(ctx, arg)
		if err != nil {
			out(name, false, "%v %s", err, el())
			return
		}
		out(name, true, "resolved %v", addrs)
	case "abstract":
		c, err := net.DialTimeout("unix", "@"+arg, 2*time.Second)
		if err != nil {
			out(name, false, "%v", err)
			return
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(time.Second))
		b := make([]byte, 100)
		n, _ := c.Read(b)
		out(name, true, "connected, banner=%q", firstLine(b[:n]))
	case "unix", "rawsend":
		data := ""
		if len(os.Args) > 3 {
			data = unescape(os.Args[3])
		}
		network := "unix"
		if cmd == "rawsend" {
			network = "tcp"
		}
		c, err := net.DialTimeout(network, arg, 3*time.Second)
		if err != nil {
			out(name, false, "%v %s", err, el())
			return
		}
		defer c.Close()
		c.Write([]byte(data))
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		b := make([]byte, 300)
		n, err := c.Read(b)
		if n == 0 {
			out(name, true, "connected, sent %d bytes, no reply (%v)", len(data), err)
			return
		}
		out(name, true, "connected, first reply line=%q", firstLine(b[:n]))
	case "rawsock":
		fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
		if err != nil {
			out("rawsock AF_INET/SOCK_RAW/ICMP", false, "socket(): %v", err)
		} else {
			// ICMP echo to 192.0.2.1 (documentation address): shows there is no route.
			pkt := []byte{8, 0, 0xf7, 0xff, 0, 1, 0, 1}
			sa := &syscall.SockaddrInet4{Addr: [4]byte{192, 0, 2, 1}}
			err2 := syscall.Sendto(fd, pkt, 0, sa)
			out("rawsock sendto 192.0.2.1 (ICMP)", err2 == nil, "raw socket created; sendto: %v", err2)
			syscall.Close(fd)
		}
		pfd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(3)))
		if err != nil {
			out("rawsock AF_PACKET", false, "socket(): %v", err)
		} else {
			out("rawsock AF_PACKET", true, "packet socket created (interfaces in this netns: lo only)")
			syscall.Close(pfd)
		}
	case "caps":
		b, _ := os.ReadFile("/proc/self/status")
		var keep []string
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "Cap") || strings.HasPrefix(l, "Uid:") || strings.HasPrefix(l, "NoNewPrivs") {
				keep = append(keep, strings.Join(strings.Fields(l), " "))
			}
		}
		out("caps", true, "%s", strings.Join(keep, "; "))
	default:
		fmt.Println("unknown subcommand")
	}
}

func htons(v uint16) uint16 {

	return v<<8 | v>>8
}
