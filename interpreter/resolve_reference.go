// Package interpreter defines input resolution and expression evaluation contracts.
package interpreter

import (
	"errors"
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

var (
	// ErrInvalidBinding indicates a present Value with a mismatched Type.
	ErrInvalidBinding = errors.New("invalid input binding")
	// ErrUnavailableInput identifies a required input that cannot be resolved in
	// complete evaluation. Partial evaluation instead retains its expression.
	ErrUnavailableInput = errors.New("unavailable input")
)

// ResolveReference checks the binding's exact declared Type.
// It never coerces or narrows nullability. Inputs are valid and non-nil; typed
// nils also assert. Provider errors preserve their causes. Absence remains an
// optional absence; complete/partial interpreters choose how to handle it.
func ResolveReference(environment interfaces.Environment, reference *expressions.ReferenceExpression) (foundation.Optional[valueinterfaces.Value], error) {
	assertions.NotNil(environment)
	assertions.NotNilPointer(reference)
	var absent foundation.Optional[valueinterfaces.Value]
	result, err := environment.Lookup(reference.ID())
	if err != nil {
		return absent, err
	}
	value, present := result.Get()
	if !present {
		return absent, nil
	}
	assertions.NotNil(value)
	if !value.Type().EqualsTo(reference.Type()) {
		return absent, fmt.Errorf("%w: expected %s, got %s", ErrInvalidBinding, reference.Type(), value.Type())
	}
	return result, nil
}
