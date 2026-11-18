package router

import (
	"io"
	"strings"
)

func nopCloser(r *strings.Reader) io.ReadCloser { return io.NopCloser(r) }
