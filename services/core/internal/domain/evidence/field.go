package evidence

import "errors"

// InvalidField returns the field that a refusal of this package names, so that
// a transport can report it without parsing the text of the error.
func InvalidField(err error) (string, bool) {
	var e *invalidError
	if errors.As(err, &e) {
		return e.field, true
	}
	return "", false
}
