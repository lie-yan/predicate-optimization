package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"

	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func eligibilityArray(typ *types.ArrayType, numbers ...int64) *expressions.LiteralExpression {
	elements := make([]valueinterfaces.Value, len(numbers))
	for i, number := range numbers {
		elements[i] = must(testbedvalues.NewInteger(typ.Element().(*types.IntType), number))
	}
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewArray(typ, elements))))
}

func TestCanOmitArrayMembershipValues(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	nullable := must(types.NewArrayType(integer, foundation.MayNull))
	one := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integer, 1))))
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullable))))
	membership := &eligibilityContext{FunctionContext: must(testbed.NewStandardContext())}
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, membership)
	for providerIndex := 0; providerIndex < 2; providerIndex++ {
		provider := must(testbed.NewStandardContext())
		context := &eligibilityContext{FunctionContext: provider}
		for _, tc := range []struct {
			name              string
			builtin           functions.BuiltInFunctionName
			target, candidate *expressions.LiteralExpression
			want              foundation.TruthValue
		}{
			{"contains match", functions.BuiltInArrayContains, eligibilityArray(array, 1, 2), one, foundation.True},
			{"contains miss", functions.BuiltInArrayContains, eligibilityArray(array, 2), one, foundation.False},
			{"contains empty", functions.BuiltInArrayContains, eligibilityArray(array), one, foundation.False},
			{"contains nullable", functions.BuiltInArrayContains, eligibilityArray(nullable, 1), one, foundation.True},
			{"contains null", functions.BuiltInArrayContains, null, one, foundation.Null},
			{"any match", functions.BuiltInArrayContainsAny, eligibilityArray(array, 1, 2), eligibilityArray(array, 2, 3), foundation.True},
			{"any miss", functions.BuiltInArrayContainsAny, eligibilityArray(array, 1), eligibilityArray(array, 2), foundation.False},
			{"any empty input", functions.BuiltInArrayContainsAny, eligibilityArray(array), eligibilityArray(array, 1), foundation.False},
			{"any empty candidates", functions.BuiltInArrayContainsAny, eligibilityArray(array, 1), eligibilityArray(array), foundation.False},
			{"any both empty", functions.BuiltInArrayContainsAny, eligibilityArray(array), eligibilityArray(array), foundation.False},
			{"any nullable", functions.BuiltInArrayContainsAny, eligibilityArray(nullable, 1), eligibilityArray(array, 1), foundation.True},
			{"any null", functions.BuiltInArrayContainsAny, null, eligibilityArray(array, 1), foundation.Null},
			{"any null with empty candidates", functions.BuiltInArrayContainsAny, null, eligibilityArray(array), foundation.Null},
		} {
			t.Run(tc.name, func(t *testing.T) {
				handle := must(provider.LookupBuiltInFunction(tc.builtin, []types.Type{tc.target.Type(), tc.candidate.Type()}))
				call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{tc.target, tc.candidate}))
				before := expressions.PrettyPrint(call)
				context.queries = 0
				if ok, err := rule.CanOmit(call); !ok || err != nil {
					t.Fatalf("admission: %v, %v", ok, err)
				}
				if context.queries != 1 || expressions.PrettyPrint(call) != before {
					t.Fatal("unexpected queries or source mutation")
				}
				// Evaluate independently through the real provider; inspection's wrapper forbids Evaluate.
				result := must(provider.Evaluate(handle, call.Signature(), []valueinterfaces.Value{tc.target.Value(), tc.candidate.Value()}))
				truth := foundation.Null
				if !result.IsNull() {
					truth = foundation.False
					if result.(*testbedvalues.BoolValue).Bool() {
						truth = foundation.True
					}
				}
				if truth != tc.want || !result.Type().EqualsTo(call.Type()) || call.Type().Nullability() != tc.target.Type().Nullability() {
					t.Fatal("array contract truth or nullability mismatch")
				}
			})
		}
	}
	if membership.queries != 0 {
		t.Fatal("array admission used the membership context instead of the call context")
	}
}

func TestCanOmitArrayMembershipArguments(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	context := &eligibilityContext{FunctionContext: provider}
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	nullable := must(types.NewArrayType(integer, foundation.MayNull))
	one := membershipCandidate(integer)
	list := eligibilityArray(array, 1)
	opaqueArray := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "array")), array, "array"))
	opaqueInt := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "integer")), integer, "integer"))
	sum := must(expressions.NewCallExpression(context, must(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer})), []expressions.Expression{one, one}))
	for _, tc := range []struct {
		name              string
		builtin           functions.BuiltInFunctionName
		target, candidate expressions.Expression
		want              bool
	}{
		{"array and integer references", functions.BuiltInArrayContains, eligibilityReference(array), eligibilityReference(integer), true},
		{"nullable array reference", functions.BuiltInArrayContainsAny, eligibilityReference(nullable), list, true},
		{"opaque input", functions.BuiltInArrayContains, opaqueArray, one, false},
		{"opaque input with empty candidates", functions.BuiltInArrayContainsAny, opaqueArray, eligibilityArray(array), false},
		{"opaque integer", functions.BuiltInArrayContains, list, opaqueInt, false},
		{"arithmetic candidate", functions.BuiltInArrayContains, list, sum, false},
		{"dynamic candidate array", functions.BuiltInArrayContainsAny, list, eligibilityReference(array), false},
		{"opaque candidate array", functions.BuiltInArrayContainsAny, list, opaqueArray, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handle := must(provider.LookupBuiltInFunction(tc.builtin, []types.Type{tc.target.Type(), tc.candidate.Type()}))
			call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{tc.target, tc.candidate}))
			if ok, err := rule.CanOmit(call); ok != tc.want || err != nil {
				t.Fatalf("admission: %v, %v", ok, err)
			}
		})
	}
	// An unsupported first argument must not hide the second argument's metadata error.
	childContext := &eligibilityContext{FunctionContext: provider}
	child := must(expressions.NewCallExpression(childContext, sum.Handle(), []expressions.Expression{one, one}))
	failure := errors.New("candidate metadata failure")
	childContext.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, failure }
	handle := must(provider.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{array, integer}))
	call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{opaqueArray, child}))
	if ok, err := rule.CanOmit(call); ok || !errors.Is(err, failure) {
		t.Fatalf("child failure hidden: %v, %v", ok, err)
	}
	// Direct call-context failures remain errors as well.
	context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, failure }
	if ok, err := rule.CanOmit(call); ok || !errors.Is(err, failure) {
		t.Fatalf("call failure hidden: %v, %v", ok, err)
	}
}

func TestCanOmitArrayMembershipUnsupportedContracts(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	nullableInt := must(types.NewIntType(types.Int64, foundation.MayNull))
	narrow := must(types.NewIntType(types.Int32, foundation.NonNull))
	float := must(types.NewFloatType(types.Float64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	nullableArray := must(types.NewArrayType(integer, foundation.MayNull))
	narrowArray := must(types.NewArrayType(narrow, foundation.NonNull))
	nullableElements := must(types.NewArrayType(nullableInt, foundation.NonNull))
	nestedArray := must(types.NewArrayType(array, foundation.NonNull))
	list := eligibilityArray(array, 1)
	nullableList := eligibilityArray(nullableArray, 1)
	nullList := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullableArray))))
	for _, tc := range []struct {
		name      string
		builtin   functions.BuiltInFunctionName
		params    []types.Type
		result    types.Type
		policies  []functions.NullPolicy
		op        foundation.Optional[foundation.OperatorType]
		candidate expressions.Expression
	}{
		{"diagnostic name only", "", []types.Type{array, integer}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"wrong arity", functions.BuiltInArrayContains, []types.Type{array}, boolean, []functions.NullPolicy{functions.PropagateNull}, foundation.Optional[foundation.OperatorType]{}, nil},
		{"wrong result", functions.BuiltInArrayContains, []types.Type{array, integer}, integer, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"nullable result", functions.BuiltInArrayContains, []types.Type{array, integer}, must(boolean.WithNullability(foundation.MayNull)), nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"nonarray target", functions.BuiltInArrayContains, []types.Type{integer, integer}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"wrong element width", functions.BuiltInArrayContains, []types.Type{narrowArray, narrow}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"nullable elements", functions.BuiltInArrayContains, []types.Type{nullableElements, integer}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"nested array", functions.BuiltInArrayContains, []types.Type{nestedArray, array}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, list},
		{"candidate width", functions.BuiltInArrayContains, []types.Type{array, narrow}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"float candidate", functions.BuiltInArrayContains, []types.Type{array, float}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"candidate array width", functions.BuiltInArrayContainsAny, []types.Type{array, narrowArray}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, eligibilityArray(narrowArray, 1)},
		{"nullable candidate type", functions.BuiltInArrayContains, []types.Type{array, nullableInt}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.PassNull}, foundation.Optional[foundation.OperatorType]{}, nil},
		{"nullable candidate array", functions.BuiltInArrayContainsAny, []types.Type{array, nullableArray}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.PassNull}, foundation.Optional[foundation.OperatorType]{}, nullableList},
		{"null candidate array", functions.BuiltInArrayContainsAny, []types.Type{array, nullableArray}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.PassNull}, foundation.Optional[foundation.OperatorType]{}, nullList},
		{"nullable implementation array", functions.BuiltInArrayContains, []types.Type{nullableArray, integer}, boolean, nil, foundation.Optional[foundation.OperatorType]{}, nil},
		{"missing array propagation", functions.BuiltInArrayContains, []types.Type{array, integer}, boolean, []functions.NullPolicy{functions.NullPolicyUnspecified, functions.NullPolicyUnspecified}, foundation.Optional[foundation.OperatorType]{}, nil},
		{"candidate propagation", functions.BuiltInArrayContains, []types.Type{array, integer}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}, foundation.Optional[foundation.OperatorType]{}, nil},
		{"conflicting operator", functions.BuiltInArrayContainsAny, []types.Type{array, array}, boolean, nil, foundation.Some(foundation.Equal), list},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policies := tc.policies
			if policies == nil {
				policies = []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
			}
			var builtin foundation.Optional[functions.BuiltInFunctionName]
			if tc.builtin != "" {
				builtin = foundation.Some(tc.builtin)
			}
			signature := must(functions.NewFunctionSignature(tc.params, tc.result))
			metadata := must(functions.NewFunctionMetadata(signature, "array_contains", builtin, tc.op, policies))
			args := make([]expressions.Expression, len(tc.params))
			for i, typ := range tc.params {
				args[i] = eligibilityReference(typ)
			}
			if tc.candidate != nil {
				args[1] = tc.candidate
			}
			_, call := eligibilityCall(metadata, args)
			if ok, err := rule.CanOmit(call); ok || err != nil {
				t.Fatalf("unsupported invocation: %v, %v", ok, err)
			}
		})
	}
}
