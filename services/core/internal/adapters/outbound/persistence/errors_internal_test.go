package persistence

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestUnavailableCoversTheWaysAConnectionBreaks(t *testing.T) {
	for name, err := range map[string]error{
		"connect":         &pgconn.ConnectError{},
		"eof":             io.EOF,
		"unexpected eof":  fmt.Errorf("read: %w", io.ErrUnexpectedEOF),
		"closed":          net.ErrClosed,
		"network":         &net.OpError{Op: "read", Err: errors.New("connection reset by peer")},
		"admin shutdown":  &pgconn.PgError{Code: "57P01"},
		"too many":        &pgconn.PgError{Code: "53300"},
		"connection down": &pgconn.PgError{Code: "08006"},
	} {
		if !unavailable(err) {
			t.Errorf("%s was not treated as unavailable", name)
		}
	}
	for name, err := range map[string]error{
		"unique":    &pgconn.PgError{Code: "23505"},
		"syntax":    &pgconn.PgError{Code: "42601"},
		"plain":     errors.New("something else"),
		"not found": nil,
	} {
		if unavailable(err) {
			t.Errorf("%s was treated as unavailable", name)
		}
	}
}
