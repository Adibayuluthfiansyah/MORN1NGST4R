package operation_test

import (
	"errors"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// D-59 and FR-APPROVAL-004: the class comes from a closed list of categories.
func TestClassOfFollowsTheStrictMapping(t *testing.T) {
	want := map[operation.Category]operation.RiskClass{
		operation.CategoryPassiveRecon:        operation.ClassPassive,
		operation.CategoryHTTPProbe:           operation.ClassLowImpact,
		operation.CategorySafeReplay:          operation.ClassLowImpact,
		operation.CategoryStateChangingReplay: operation.ClassIntrusive,
		operation.CategoryTemplateCheck:       operation.ClassIntrusive,
	}
	for cat, class := range want {
		if got := operation.ClassOf(cat); got != class {
			t.Errorf("ClassOf(%v) = %v, want %v", cat, got, class)
		}
	}
}

// An unknown operation has no class and is denied (FR-APPROVAL-004 AC4).
func TestClassOfAnUnknownCategoryIsUnknown(t *testing.T) {
	for _, c := range []operation.Category{operation.CategoryUnknown, operation.Category(-1), operation.Category(6), operation.Category(1000)} {
		if got := operation.ClassOf(c); got != operation.ClassUnknown {
			t.Errorf("ClassOf(%d) = %v, want ClassUnknown", c, got)
		}
	}
	var zero operation.RiskClass
	if zero != operation.ClassUnknown {
		t.Error("the zero RiskClass must be ClassUnknown")
	}
}

func TestRiskClassNamesAndActivity(t *testing.T) {
	cases := []struct {
		class  operation.RiskClass
		name   string
		active bool
	}{
		{operation.ClassPassive, "passive", false},
		{operation.ClassLowImpact, "low_impact", true},
		{operation.ClassIntrusive, "intrusive", true},
	}
	for _, tc := range cases {
		if tc.class.String() != tc.name || tc.class.IsActive() != tc.active {
			t.Errorf("%v: String = %q, IsActive = %v", tc.class, tc.class.String(), tc.class.IsActive())
		}
		got, err := operation.ParseRiskClass(tc.name)
		if err != nil || got != tc.class {
			t.Errorf("ParseRiskClass(%q) = %v, %v", tc.name, got, err)
		}
	}
	// The unknown class is not active here. A caller must still deny it, and
	// operation.New never produces it.
	if operation.ClassUnknown.IsActive() || operation.ClassUnknown.String() != "unknown" {
		t.Error("ClassUnknown must be named unknown and must not be treated as active")
	}
	for _, in := range []string{"", "Passive", "low-impact", "lowimpact", "ai", "high", "intrusive "} {
		if got, err := operation.ParseRiskClass(in); !errors.Is(err, operation.ErrInvalid) || got != operation.ClassUnknown {
			t.Errorf("ParseRiskClass(%q) = %v, %v, want ErrInvalid", in, got, err)
		}
	}
}

func TestCategoryNames(t *testing.T) {
	for cat, name := range map[operation.Category]string{
		operation.CategoryPassiveRecon:        "passive_recon",
		operation.CategoryHTTPProbe:           "http_probe",
		operation.CategorySafeReplay:          "safe_replay",
		operation.CategoryStateChangingReplay: "state_changing_replay",
		operation.CategoryTemplateCheck:       "template_check",
		operation.CategoryUnknown:             "unknown",
	} {
		if cat.String() != name {
			t.Errorf("Category(%d).String() = %q, want %q", cat, cat.String(), name)
		}
	}
}
