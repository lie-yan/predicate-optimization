package interpreter

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// EvaluateMembershipEquality compares concrete scalar values through a provider's
// Equal operator. It does not use structural Value.EqualsTo as SQL equality.
// Only Bool, integer, float and VARCHAR domains are admitted; provider lookup decides
// which exact widths/bounds it implements. No coercion is performed.
// NULL equality returns a typed NULL under the provider's propagation contract.
// Admission of NULL/empty/dynamic IN candidates belongs to membership evaluation,
// not this pairwise service. Inputs must be valid and non-nil.
func EvaluateMembershipEquality(provider functioninterfaces.FunctionContext, left, right valueinterfaces.Value) (valueinterfaces.Value, error) {
	assertions.NotNil(provider)
	assertions.NotNil(left)
	assertions.NotNil(right)
	for _, value := range []valueinterfaces.Value{left, right} {
		switch value.Type().(type) {
		case *types.BoolType, *types.IntType, *types.FloatType, *types.VarCharType:
		default:
			return nil, fmt.Errorf("%w: membership equality domain", functions.ErrUnsupportedLookup)
		}
	}
	handle, err := provider.ComparisonFunction(foundation.Equal, left.Type(), right.Type())
	if err != nil {
		return nil, err
	}
	metadata, err := provider.Metadata(handle)
	if err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, fmt.Errorf("%w: nil equality metadata", functions.ErrContractViolation)
	}
	signature, err := functions.BindFunctionSignature(metadata, []types.Type{left.Type(), right.Type()})
	if err != nil {
		return nil, err
	}
	nullability := foundation.NonNull
	if left.Type().Nullability() == foundation.MayNull || right.Type().Nullability() == foundation.MayNull {
		nullability = foundation.MayNull
	}
	boolean, err := types.NewBoolType(nullability)
	if err != nil {
		return nil, err
	}
	if !signature.ResultType().EqualsTo(boolean) {
		return nil, fmt.Errorf("%w: equality must return %s", functions.ErrContractViolation, boolean)
	}
	result, err := provider.Evaluate(handle, signature, []valueinterfaces.Value{left, right})
	if err != nil {
		return nil, err
	}
	assertions.NotNil(result)
	if !result.Type().EqualsTo(boolean) {
		return nil, fmt.Errorf("%w: invalid equality result", functions.ErrContractViolation)
	}
	return result, nil
}
