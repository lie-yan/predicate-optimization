package utils

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// RecognizeScalarComparison recognizes the standard scalar comparison contract:
// compatible supported domains, a non-null Boolean implementation result, and
// PropagateNull for both parameters. Absence means unsupported semantics.
// Metadata and signature must be valid, immutable and non-nil, with signature
// already bound to metadata. This predicate neither validates that binding nor
// queries a provider. Provider semantics are trusted; recognition alone does not
// establish omission eligibility or justify a particular rewrite.
func RecognizeScalarComparison(metadata *functions.FunctionMetadata, signature *functions.FunctionSignature) foundation.Optional[foundation.OperatorType] {
	assertions.NotNilPointer(metadata)
	assertions.NotNilPointer(signature)
	if signature.ArgumentCount() != 2 {
		return foundation.Optional[foundation.OperatorType]{}
	}
	return RecognizeScalarComparisonPair(metadata, signature.ArgumentTypeAt(0), signature.ArgumentTypeAt(1))
}

// RecognizeScalarComparisonPair recognizes the scalar comparison contract for
// two argument types without constructing an effective signature. Arguments must
// already have passed binding against valid, immutable metadata (including result
// nullability adaptation). This does not validate binding or query a provider.
// Recognition alone does not establish omission eligibility or rewrite safety.
func RecognizeScalarComparisonPair(metadata *functions.FunctionMetadata, left, right types.Type) foundation.Optional[foundation.OperatorType] {
	assertions.NotNilPointer(metadata)
	absent := foundation.Optional[foundation.OperatorType]{}
	operator, present := metadata.Operator().Get()
	if !present || !operator.IsComparisonOperator() {
		return absent
	}
	if !isScalarComparisonPair(operator, left, right) {
		return absent
	}
	implementation := metadata.Signature()
	result, boolean := implementation.ResultType().(*types.BoolType)
	if !boolean || result.Nullability() != foundation.NonNull {
		return absent
	}
	for i := 0; i < implementation.ArgumentCount(); i++ {
		if metadata.NullPolicyAt(i) != functions.PropagateNull {
			return absent
		}
	}
	return foundation.Some(operator)
}

// IsScalarComparisonDomain recognizes the argument domain independently of
// metadata, so consumers can check applicability before resolving an invocation.
// Argument Types must be valid, immutable and non-nil. Only outer nullability is
// ignored; integer/float widths and VARCHAR bounds must agree. Boolean supports
// equality and inequality; integer, float and VARCHAR also support ordering.
func IsScalarComparisonDomain(operator foundation.OperatorType, arguments []types.Type) bool {
	if !operator.IsComparisonOperator() || len(arguments) != 2 {
		return false
	}
	return isScalarComparisonPair(operator, arguments[0], arguments[1])
}

// isScalarComparisonPair shares domain recognition after callers have checked
// the operator and arity. It reads the two immutable descriptors without a slice.
func isScalarComparisonPair(operator foundation.OperatorType, left, right types.Type) bool {
	assertions.NotNil(left)
	assertions.NotNil(right)
	if !left.EqualsToIgnoringNullability(right) {
		return false
	}
	switch left.(type) {
	case *types.BoolType, *types.IntType, *types.FloatType, *types.VarCharType:
	default:
		return false
	}
	return operator == foundation.Equal || operator == foundation.NotEqual || left.IsOrderable()
}
