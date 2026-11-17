package actor_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
)

func TestZeroActorIsNotHuman(t *testing.T) {
	var a actor.Actor
	if a.IsHuman() || !a.IsZero() || a.Kind() != actor.KindInvalid || a.ID() != "" {
		t.Errorf("the zero Actor = kind %v id %q human %v zero %v, want an unusable value", a.Kind(), a.ID(), a.IsHuman(), a.IsZero())
	}
}

func TestNewRefusesBadInput(t *testing.T) {
	bad := []struct {
		name string
		kind actor.Kind
		id   string
	}{
		{"invalid kind", actor.KindInvalid, "u1"},
		{"kind out of range", actor.Kind(99), "u1"},
		{"negative kind", actor.Kind(-1), "u1"},
		{"empty id", actor.KindHuman, ""},
		{"space in id", actor.KindHuman, "u 1"},
		{"leading space", actor.KindHuman, " u1"},
		{"newline", actor.KindHuman, "u1\n"},
		{"NUL", actor.KindHuman, "u\x001"},
		{"slash", actor.KindHuman, "u/1"},
		{"at sign", actor.KindHuman, "u@1"},
		{"non-ASCII", actor.KindHuman, "usér"},
		{"leading dot", actor.KindHuman, ".u1"},
		{"leading dash", actor.KindHuman, "-u1"},
		{"65 bytes", actor.KindHuman, strings.Repeat("a", 65)},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			a, err := actor.New(tc.kind, tc.id)
			if !errors.Is(err, actor.ErrInvalid) {
				t.Errorf("New(%v, %q) = %+v, %v, want ErrInvalid", tc.kind, tc.id, a, err)
			}
			if !a.IsZero() || a.IsHuman() {
				t.Errorf("New(%v, %q) returned a usable value with an error", tc.kind, tc.id)
			}
		})
	}
}

func TestNewAcceptsGoodInput(t *testing.T) {
	for _, id := range []string{
		"0190a3a2-7c1e-7cc3-9f3c-5d2a8f1e4b6a", "worker-1", "u_1.x:y", "A", strings.Repeat("a", 64),
	} {
		a, err := actor.New(actor.KindHuman, id)
		if err != nil || a.ID() != id || a.IsZero() || !a.IsHuman() {
			t.Errorf("New(human, %q) = %+v, %v", id, a, err)
		}
	}
}

func TestOnlyAHumanIsHuman(t *testing.T) {
	for kind, want := range map[actor.Kind]bool{actor.KindHuman: true, actor.KindSystem: false, actor.KindAI: false} {
		a, err := actor.New(kind, "x1")
		if err != nil {
			t.Fatal(err)
		}
		if a.IsHuman() != want || a.Kind() != kind {
			t.Errorf("kind %v: IsHuman = %v, Kind = %v", kind, a.IsHuman(), a.Kind())
		}
	}
}

func TestParseKind(t *testing.T) {
	for in, want := range map[string]actor.Kind{"human": actor.KindHuman, "system": actor.KindSystem, "ai": actor.KindAI} {
		k, err := actor.ParseKind(in)
		if err != nil || k != want || k.String() != in {
			t.Errorf("ParseKind(%q) = %v, %v", in, k, err)
		}
	}
	// There are three kinds. A worker is a system actor and the MVP has no API tokens (D-60).
	for _, in := range []string{"Human", "HUMAN", "worker", "api_token", "token", "", "admin", " human", "human "} {
		if k, err := actor.ParseKind(in); !errors.Is(err, actor.ErrInvalid) || k != actor.KindInvalid {
			t.Errorf("ParseKind(%q) = %v, %v, want ErrInvalid", in, k, err)
		}
	}
	if actor.KindInvalid.String() != "invalid" {
		t.Errorf("KindInvalid.String() = %q", actor.KindInvalid.String())
	}
}
