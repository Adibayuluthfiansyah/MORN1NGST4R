package evidence_test

import (
	"errors"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
)

func TestInvalidFieldNamesTheFieldOfARefusal(t *testing.T) {
	in := valid()
	in.Kind = "screenshot"
	_, err := evidence.New(in)
	if f, ok := evidence.InvalidField(err); !ok || f != "kind" {
		t.Errorf("InvalidField = %q, %v", f, ok)
	}
	if _, err = evidence.NewCheck("x", item(t), evidence.Observation{}, evidence.TriggerRead, evidence.CheckActor{Kind: evidence.ActorSystem}); true {
		if f, ok := evidence.InvalidField(err); !ok || f != "id" {
			t.Errorf("InvalidField of a check = %q, %v", f, ok)
		}
	}
	if f, ok := evidence.InvalidField(errors.New("other")); ok || f != "" {
		t.Errorf("InvalidField of another error = %q, %v", f, ok)
	}
	if _, ok := evidence.InvalidField(nil); ok {
		t.Error("InvalidField(nil) = true")
	}
}
