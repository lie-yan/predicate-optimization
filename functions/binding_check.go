package functions

import (
	"errors"

	opterrors "github.com/lie-yan/predicate-optimization/errors"
	"github.com/lie-yan/predicate-optimization/types"
)

// BindingCheck is an immutable binding outcome without a diagnostic allocation.
// The zero value denotes success. Obtain one with ProbeFunctionBinding; use Err
// only when a diagnostic must escape. A retained cause follows its own lifetime
// and concurrency contracts. This record does not implement error.
type BindingCheck struct {
	code          opterrors.Code
	first, second int
	cause         error
}

// ProbeFunctionBinding performs the same checks and result-nullability adaptation
// as CheckFunctionBinding, retaining compact failure details rather than building
// a BindingError. Type adaptation can still allocate or produce an external cause.
func ProbeFunctionBinding(metadata *FunctionMetadata, argumentTypes []types.Type) BindingCheck {
	_, check := probeResultType(metadata, argumentTypes)
	return check
}

// Supported reports whether binding succeeded.
func (c BindingCheck) Supported() bool { return c.code == opterrors.CodeUnknown }

// Is matches the category or original cause without materializing a diagnostic.
// Every current binding rejection has the unsupported-lookup category, including
// adaptation failures whose cause can also carry stronger categories.
func (c BindingCheck) Is(target error) bool {
	return !c.Supported() && target != nil && (errors.Is(ErrUnsupportedLookup, target) || errors.Is(c.cause, target))
}

// Err materializes the same diagnostic as CheckFunctionBinding, or nil on success.
// Results are not cached; repeated calls can allocate separate error records.
func (c BindingCheck) Err() error {
	switch c.code {
	case opterrors.CodeUnknown:
		return nil
	case opterrors.CodeBindingArityMismatch:
		return newBindingArityError(c.first, c.second)
	case opterrors.CodeBindingArgumentTypeMismatch, opterrors.CodeBindingNullPolicyUnspecified:
		return newBindingArgumentError(c.code, c.first)
	default:
		return newBindingError(c.code, c.cause)
	}
}
