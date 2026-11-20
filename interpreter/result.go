package interpreter

import (
	"errors"
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ErrInvalidResult identifies an exact result-Type mismatch.
var ErrInvalidResult = errors.New("invalid evaluation result")

// Result is either a known Value or a residual Expression, sharing its immutable
// payload. Its zero value is invalid. Constructors require the original
// expression's effective Type so neither branch silently changes nullability.
type Result struct {
	known    valueinterfaces.Value
	residual expressions.Expression
}

// NewKnownResult constructs a result with exactly expected Type. Nil inputs
// assert; existing Types/Values must be valid. Type mismatches return ErrInvalidResult.
func NewKnownResult(expected types.Type, value valueinterfaces.Value) (Result, error) {
	assertions.NotNil(expected)
	assertions.NotNil(value)
	if !expected.EqualsTo(value.Type()) {
		return Result{}, fmt.Errorf("%w: known value type differs from %s", ErrInvalidResult, expected)
	}
	return Result{known: value}, nil
}

// NewResidualResult requires a valid immutable expression. Nil inputs assert;
// the expected Type must be valid. An exact Type mismatch returns ErrInvalidResult.
// Import validation belongs at the boundary, not every residual construction.
func NewResidualResult(expected types.Type, expression expressions.Expression) (Result, error) {
	assertions.NotNil(expected)
	assertions.NotNil(expression)
	if !expected.EqualsTo(expression.Type()) {
		return Result{}, fmt.Errorf("%w: residual expression type mismatch", ErrInvalidResult)
	}
	return Result{residual: expression}, nil
}

// IsValid reports whether a result was constructed with one of the alternatives.
func (r Result) IsValid() bool                            { return (r.known != nil) != (r.residual != nil) }
func (r Result) Known() (valueinterfaces.Value, bool)     { return r.known, r.known != nil }
func (r Result) Residual() (expressions.Expression, bool) { return r.residual, r.residual != nil }

// Type returns the effective Type; the zero/invalid result violates the precondition.
func (r Result) Type() types.Type {
	if !r.IsValid() {
		panic("interpreter: expected constructed Result")
	}
	if r.known != nil {
		return r.known.Type()
	}
	return r.residual.Type()
}
