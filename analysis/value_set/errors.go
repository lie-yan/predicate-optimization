package value_set

import "errors"

var (
	// ErrInvalidInput identifies malformed non-nil analysis input.
	ErrInvalidInput = errors.New("scalar analysis: invalid input")
	// ErrIncompatibleTypes identifies different supported non-null value universes.
	ErrIncompatibleTypes = errors.New("scalar analysis: incompatible types")
	// ErrUnsupported identifies valid input whose semantics or exact query is not supported.
	// It is distinct from an exact empty set or a false query result.
	ErrUnsupported = errors.New("scalar analysis: unsupported semantics")
)
