package functions

import (
	"fmt"

	opterrors "github.com/lie-yan/predicate-optimization/errors"
)

// Keep the embedded header private so callers cannot replace classification.
type bindingErrorBase = opterrors.Base

// BindingError describes a signature-binding failure. Its fields are immutable;
// use errors.As to inspect details and errors.Is to classify its category.
// The zero value has an unknown code. Diagnostics are formatted only by Error.
type BindingError struct {
	bindingErrorBase
	argument int
	actual   int
	expected int
}

var _ opterrors.CodedError = (*BindingError)(nil)

func newBindingError(code opterrors.Code, cause error) *BindingError {
	category := ErrUnsupportedLookup
	if code == opterrors.CodeInvalidCallBinding || code == opterrors.CodeBindingResultTypeMismatch {
		category = ErrContractViolation
	}
	return &BindingError{bindingErrorBase: opterrors.NewBase(code, category, cause)}
}

func newBindingArityError(actual, expected int) *BindingError {
	err := newBindingError(opterrors.CodeBindingArityMismatch, nil)
	err.actual, err.expected = actual, expected
	return err
}

func newBindingArgumentError(code opterrors.Code, index int) *BindingError {
	err := newBindingError(code, nil)
	err.argument = index
	return err
}

// ArgumentIndex returns the zero-based position for type and null-policy failures.
func (e *BindingError) ArgumentIndex() (index int, present bool) {
	switch e.Code() {
	case opterrors.CodeBindingArgumentTypeMismatch, opterrors.CodeBindingNullPolicyUnspecified:
		return e.argument, true
	default:
		return 0, false
	}
}

// ArgumentCounts returns actual and expected arities for an arity failure.
func (e *BindingError) ArgumentCounts() (actual, expected int, present bool) {
	if e.Code() == opterrors.CodeBindingArityMismatch {
		return e.actual, e.expected, true
	}
	return 0, 0, false
}

// Error renders the diagnostic on demand, preserving the original binding text.
// Messages are not cached; rendering a cause follows that cause's Error contract.
func (e *BindingError) Error() string {
	switch e.Code() {
	case opterrors.CodeBindingArityMismatch:
		return fmt.Sprintf("%v: argument count %d does not match arity %d", ErrUnsupportedLookup, e.actual, e.expected)
	case opterrors.CodeBindingArgumentTypeMismatch:
		return fmt.Sprintf("%v: argument %d differs beyond outer nullability", ErrUnsupportedLookup, e.argument)
	case opterrors.CodeBindingNullPolicyUnspecified:
		return fmt.Sprintf("%v: argument %d is nullable with unspecified null policy", ErrUnsupportedLookup, e.argument)
	case opterrors.CodeBindingResultNullability:
		return fmt.Sprintf("%v: result nullability adaptation: %v", ErrUnsupportedLookup, e.Unwrap())
	case opterrors.CodeInvalidCallBinding:
		return fmt.Sprintf("%v: invalid call binding: %v", ErrContractViolation, e.Unwrap())
	case opterrors.CodeBindingResultTypeMismatch:
		return fmt.Sprintf("%v: effective result type does not match binding", ErrContractViolation)
	default:
		return e.bindingErrorBase.Error()
	}
}
