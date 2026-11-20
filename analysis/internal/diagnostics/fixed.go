// Package diagnostics supplies immutable, fixed unsupported-analysis errors.
// Only messages without per-occurrence payloads may be shared this way.
package diagnostics

import (
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	opterrors "github.com/lie-yan/predicate-optimization/errors"
)

// A private embedding prevents consumers from replacing the shared header.
type base = opterrors.Base

type fixedError struct {
	base
	message string
}

var _ opterrors.CodedError = (*fixedError)(nil)

func (e *fixedError) Error() string { return e.message }

// Construct records and complete messages once, never during extraction. No
// record retains expressions, providers or mutable per-occurrence state.
func newFixed(code opterrors.Code, detail string) *fixedError {
	return &fixedError{
		base:    opterrors.NewBase(code, value_set.ErrUnsupported, nil),
		message: value_set.ErrUnsupported.Error() + ": " + detail,
	}
}

var (
	comparisonUnsupported = newFixed(opterrors.CodeAnalysisComparisonUnsupported, "scalar comparison semantics not supported")
	logicalUnsupported    = newFixed(opterrors.CodeAnalysisLogicalUnsupported, "logical children are unsupported or refer to different operands")
	constantOnly          = newFixed(opterrors.CodeAnalysisConstantOnly, "constant-only logical expression has no operand domain")
)

// ComparisonUnsupported reports unsupported scalar comparison semantics.
func ComparisonUnsupported() error { return comparisonUnsupported }

// LogicalUnsupported reports unsupported children or incompatible operand domains.
func LogicalUnsupported() error { return logicalUnsupported }

// ConstantOnly reports a logical expression without an operand domain.
func ConstantOnly() error { return constantOnly }
