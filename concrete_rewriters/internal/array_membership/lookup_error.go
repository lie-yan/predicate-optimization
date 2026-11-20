package array_membership

import (
	"errors"

	"github.com/lie-yan/predicate-optimization/functions"
)

// IsUnsupportedTargetLookup reports a benign miss at explicit replacement lookup.
// Stronger failure categories take precedence through wrapping and joining.
// Metadata and construction errors must propagate regardless of this result.
func IsUnsupportedTargetLookup(err error) bool {
	for _, failure := range []error{
		functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation,
		functions.ErrEvaluationFailure, functions.ErrEvaluationUnavailable, functions.ErrEvaluationDisallowed,
	} {
		if errors.Is(err, failure) {
			return false
		}
	}
	return errors.Is(err, functions.ErrUnsupportedLookup)
}
