package operation

import "errors"

// ErrInvalid is returned for input that this package refuses. The text is
// static. It never echoes the input, which can come from a tool manifest, a
// plan item or an AI proposal.
var ErrInvalid = errors.New("invalid operation")

// RiskClass is the approval class of an operation (D-59). The zero value is
// ClassUnknown, which no code path accepts: an operation without a reviewed
// classification is denied.
type RiskClass int

const (
	ClassUnknown RiskClass = iota
	ClassPassive
	ClassLowImpact
	ClassIntrusive
)

// String returns the stable name that is stored and audited.
func (c RiskClass) String() string {
	switch c {
	case ClassPassive:
		return "passive"
	case ClassLowImpact:
		return "low_impact"
	case ClassIntrusive:
		return "intrusive"
	}
	return "unknown"
}

// IsActive reports whether the class sends traffic to the target. Active
// operations need a current validation, approval and established limits.
func (c RiskClass) IsActive() bool { return c == ClassLowImpact || c == ClassIntrusive }

// ParseRiskClass reads a stored class name. Only the three exact names are
// valid.
func ParseRiskClass(s string) (RiskClass, error) {
	switch s {
	case "passive":
		return ClassPassive, nil
	case "low_impact":
		return ClassLowImpact, nil
	case "intrusive":
		return ClassIntrusive, nil
	}
	return ClassUnknown, ErrInvalid
}

// Category is what an operation does, as declared by the reviewed tool
// manifest. It is the only input from which a class is derived, so a class
// that an AI proposal carries has nowhere to go (FR-APPROVAL-004 AC5).
type Category int

const (
	CategoryUnknown Category = iota
	CategoryPassiveRecon
	CategoryHTTPProbe
	CategorySafeReplay
	CategoryStateChangingReplay
	CategoryTemplateCheck
)

// String returns the stable name of the category.
func (c Category) String() string {
	switch c {
	case CategoryPassiveRecon:
		return "passive_recon"
	case CategoryHTTPProbe:
		return "http_probe"
	case CategorySafeReplay:
		return "safe_replay"
	case CategoryStateChangingReplay:
		return "state_changing_replay"
	case CategoryTemplateCheck:
		return "template_check"
	}
	return "unknown"
}

// ClassOf is the strict mapping of D-59. Passive reconnaissance is Passive.
// HTTP probing and safe-method replay are Low-impact. State-changing replay and
// known-vulnerability template checks are Intrusive. Anything else has no class.
func ClassOf(c Category) RiskClass {
	switch c {
	case CategoryPassiveRecon:
		return ClassPassive
	case CategoryHTTPProbe, CategorySafeReplay:
		return ClassLowImpact
	case CategoryStateChangingReplay, CategoryTemplateCheck:
		return ClassIntrusive
	}
	return ClassUnknown
}
