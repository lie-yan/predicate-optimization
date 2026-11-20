package functions

import "errors"

// Context implementations wrap these categories with details using %w or an
// equivalent wrapping mechanism. Consumers use errors.Is rather than messages.
// Execution is synchronous; no cancellation or deadline protocol is exposed.
var (
	// ErrUnknownHandle means the context accepts the handle's representation
	// and identity domain but cannot find the referenced definition.
	ErrUnknownHandle = errors.New("unknown function handle")
	// ErrIncompatibleHandle means the handle belongs to an unsupported
	// representation or identity domain.
	ErrIncompatibleHandle = errors.New("incompatible function handle")
	// ErrUnsupportedLookup means a requested name, operator, arity or argument
	// signature cannot be resolved. A rewrite can retain the original call.
	ErrUnsupportedLookup = errors.New("unsupported function lookup")
	// ErrEvaluationUnavailable means the function has no executable behavior
	// available in this context. Metadata lookup may still succeed.
	ErrEvaluationUnavailable = errors.New("function evaluation unavailable")
	// ErrEvaluationDisallowed means evaluation is not admitted by the context's
	// policy or established contract, including unspecified null handling.
	ErrEvaluationDisallowed = errors.New("function evaluation disallowed")
	// ErrEvaluationFailure means a semantic failure on concrete inputs, such
	// as division by zero. It must not be interpreted as a null or false result.
	ErrEvaluationFailure = errors.New("function evaluation failed")
	// ErrContractViolation means an invalid signature, argument or result
	// violates the function contract. It is not an unsupported optimization.
	ErrContractViolation = errors.New("function contract violated")
)
