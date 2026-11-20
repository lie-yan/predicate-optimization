// Package array_membership shares semantic membership-family recognition and
// target-lookup error classification between concrete array rewriters.
package array_membership

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

// RecognizeMetadata recognizes the Array<E> membership family without
// prescribing E or its equality semantics. The provider supplies matching semantics
// for both built-ins; recognition checks their Types, identity and null policies.
// Metadata must be valid and non-nil. This does not inspect a stored call, bind
// arguments, query a provider or establish runtime execution eligibility.
func RecognizeMetadata(metadata *functions.FunctionMetadata, name functions.BuiltInFunctionName) bool {
	if name != functions.BuiltInArrayContainsAny && name != functions.BuiltInArrayContains {
		return false
	}
	actual, present := metadata.BuiltInName().Get()
	if !present || actual != name {
		return false
	}
	if _, present := metadata.Operator().Get(); present {
		return false
	}
	signature := metadata.Signature()
	result, ok := signature.ResultType().(*types.BoolType)
	if !ok || result.Nullability() != foundation.NonNull {
		return false
	}
	parameters := signature.ArgumentTypes()
	if len(parameters) != 2 {
		return false
	}
	array, ok := parameters[0].(*types.ArrayType)
	if !ok || array.Nullability() != foundation.NonNull || array.Element().Nullability() != foundation.NonNull {
		return false
	}
	policies := metadata.NullPolicies()
	if policies[0] != functions.PropagateNull || policies[1] != functions.NullPolicyUnspecified {
		return false
	}
	if name == functions.BuiltInArrayContains {
		return parameters[1].EqualsTo(array.Element())
	}
	return parameters[1].EqualsTo(array)
}
