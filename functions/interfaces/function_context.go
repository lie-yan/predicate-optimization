package interfaces

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// FunctionContext is supplied by the caller and paired with its FunctionHandles.
// Metadata, handle meanings and evaluation semantics remain stable throughout
// an optimization session. Construction and executable storage belong to the
// caller. Concurrent use requires an explicit guarantee from the implementation.
//
// Handles, signatures, Types and Values must be valid and non-nil; typed nil
// values are also excluded. Nil inputs violate preconditions and should trigger an
// assertion. Empty argument slices are allowed for zero-argument functions.
// Methods must not mutate or retain caller-owned slices; immutable elements may
// be shared. Successful handle, metadata and Value results are non-nil.
//
// Implementations use the error categories in the functions package, preserving wrapped
// causes. Handle lookup failures are distinct
// from unsupported function requests and unavailable or failed evaluation.
type FunctionContext interface {
	// LookupBuiltInFunction resolves the standard built-in for argument-expression
	// types, including declared outer-nullability adaptation. It neither evaluates
	// arguments nor inserts coercions. The handle exposes implementation types
	// and belongs to this context. Execution availability is independent of lookup.
	LookupBuiltInFunction(name functions.BuiltInFunctionName, argumentTypes []types.Type) (FunctionHandle, error)
	// ArithmeticFunction resolves an arithmetic operator, validating its family
	// and arity. Negate is unary; the other current arithmetic operators are binary.
	ArithmeticFunction(operator foundation.OperatorType, argumentTypes []types.Type) (FunctionHandle, error)
	// ComparisonFunction resolves a binary comparison for the operand types.
	ComparisonFunction(operator foundation.OperatorType, leftType, rightType types.Type) (FunctionHandle, error)
	// IsArithmeticFunction inspects operator metadata. A recognized function
	// outside this family returns false; an unknown/incompatible handle is an error.
	IsArithmeticFunction(handle FunctionHandle) (bool, error)
	// IsComparisonFunction follows the same recognition contract for comparisons.
	IsComparisonFunction(handle FunctionHandle) (bool, error)
	// Metadata returns immutable semantic information consistent with the handle.
	Metadata(handle FunctionHandle) (*functions.FunctionMetadata, error)
	// Evaluate performs checked evaluation of concrete values, not expression
	// children. It checks the effective call signature and exact argument Types, applies
	// evaluation policy and null policies, and returns a valid Value with exactly
	// callSignature.ResultType(). Known-null propagation can succeed without
	// executable behavior. Nullability adaptation preserves nested constraints.
	// Unavailable inputs must remain expressions rather than nil argument Values.
	// Semantic failures and contract violations are errors, never synthetic nulls.
	// Evaluation is synchronous and assumes benign execution behavior.
	Evaluate(handle FunctionHandle, callSignature *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error)
}
