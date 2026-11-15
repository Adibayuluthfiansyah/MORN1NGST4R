// Command spike runs one experiment (e1..e6) of the egress feasibility spike.
// Each experiment prints observations and one "VERDICT <Q> <verdict> | note" line.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"spike/egress"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike e1|e2|e3|e4|e5|e6")
		os.Exit(2)
	}
	fmt.Printf("# go=%s os=%s/%s time=%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, time.Now().Format(time.RFC3339))
	switch os.Args[1] {
	case "e1":
		e1()
	case "e2":
		e2()
	case "e3":
		e3()
	case "e4":
		e4()
	case "e5":
		e5()
	case "e6":
		e6()
	default:
		fmt.Fprintln(os.Stderr, "unknown experiment")
		os.Exit(2)
	}
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 40*time.Second)
}

func must[T any](v T, err error) T {
	if err != nil {
		fmt.Println("FATAL:", err)
		os.Exit(3)
	}
	return v
}

func hopLines(c *egress.Checker, r *egress.Result) {
	for _, h := range r.Hops {
		c.Obs("%s", h)
	}
}
