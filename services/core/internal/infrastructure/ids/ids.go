// Package ids creates identifiers for records. The application generates them
// (DB-04), as UUIDv7: the first 48 bits are the Unix time in milliseconds, so
// ids sort by creation time and index well.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// New returns a version 7 UUID in lower-case canonical form. It panics when the
// system random source fails, which is unrecoverable.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("ids: no random source: " + err.Error())
	}
	ms := uint64(time.Now().UnixMilli())
	b[0], b[1], b[2], b[3], b[4], b[5] = byte(ms>>40), byte(ms>>32), byte(ms>>24), byte(ms>>16), byte(ms>>8), byte(ms)
	b[6] = b[6]&0x0f | 0x70
	b[8] = b[8]&0x3f | 0x80
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:], b[10:])
	return string(out[:])
}
