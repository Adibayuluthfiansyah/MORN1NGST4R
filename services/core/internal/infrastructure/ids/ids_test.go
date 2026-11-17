package ids_test

import (
	"regexp"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
)

var canonical = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewIsAVersion7UUIDInLowerCaseCanonicalForm(t *testing.T) {
	for i := 0; i < 100; i++ {
		if id := ids.New(); !canonical.MatchString(id) {
			t.Fatalf("%q is not a lower-case version 7 UUID", id)
		}
	}
}

func TestNewIsUniqueAndTimeOrdered(t *testing.T) {
	seen := map[string]bool{}
	prev := ""
	for i := 0; i < 5000; i++ {
		id := ids.New()
		if seen[id] {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = true
		// The first 48 bits are the millisecond clock, so ids never sort before an earlier one.
		if prev != "" && id[:13] < prev[:13] {
			t.Fatalf("%s sorts before %s", id, prev)
		}
		prev = id
	}
}
