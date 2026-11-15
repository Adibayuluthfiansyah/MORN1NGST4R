// gotool: tiny static Go "tool" used to show that a Go binary (threads, goroutines, os/exec,
// DNS-less networking) still runs under the seccomp filter. Local only; no network access.
package main

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.Gosched()
		}()
	}
	wg.Wait()
	f, err := os.Create("gotool.out")
	if err == nil {
		f.WriteString("ok\n")
		f.Close()
	}
	_, nerr := net.Dial("tcp", "127.0.0.1:1") // loopback inside the sandbox: expected to be refused
	fmt.Printf("GOTOOL goroutines=ok threads=%d writefile=%v loopback_dial_err=%v\n", runtime.NumCPU(), err == nil, nerr != nil)
}
