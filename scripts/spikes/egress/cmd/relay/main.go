// Command relay is the tiny in-namespace alternative to socat: it listens on a
// loopback TCP address and relays every connection to a Unix socket. It does
// not parse the bytes it relays.
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:3128", "loopback TCP listen address (must be loopback)")
	unixPath := flag.String("unix", "", "Unix socket path to relay to")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		fmt.Fprintln(os.Stderr, "relay: -listen must be a loopback IP:port")
		os.Exit(2)
	}
	if *unixPath == "" {
		fmt.Fprintln(os.Stderr, "relay: -unix required")
		os.Exit(2)
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay:", err)
		os.Exit(1)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			u, err := net.Dial("unix", *unixPath)
			if err != nil {
				return
			}
			defer u.Close()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); io.Copy(u, c); u.(*net.UnixConn).CloseWrite() }()
			go func() { defer wg.Done(); io.Copy(c, u); c.(*net.TCPConn).CloseWrite() }()
			wg.Wait()
		}(c)
	}
}
