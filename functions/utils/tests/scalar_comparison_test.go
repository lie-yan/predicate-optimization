package utils_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/types"
)

func comparisonMetadata(arguments []types.Type, result types.Type, operator foundation.Optional[foundation.OperatorType], policies []functions.NullPolicy) *functions.FunctionMetadata {
	return must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(arguments, result)), "int64.equal", foundation.Optional[functions.BuiltInFunctionName]{}, operator, policies))
}

func TestRecognizeScalarComparisonDomainsAndNullability(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	bound := uint64(8)
	domains := []types.Type{boolean, must(types.NewVarCharType(nil, foundation.NonNull)), must(types.NewVarCharType(&bound, foundation.NonNull))}
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		domains = append(domains, must(types.NewIntType(width, foundation.NonNull)))
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		domains = append(domains, must(types.NewFloatType(width, foundation.NonNull)))
	}
	operators := []foundation.OperatorType{foundation.Equal, foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual}
	for _, domain := range domains {
		for _, operator := range operators {
			metadata := comparisonMetadata([]types.Type{domain, domain}, boolean, foundation.Some(operator), []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull})
			for _, leftNull := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				for _, rightNull := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
					arguments := []types.Type{must(domain.WithNullability(leftNull)), must(domain.WithNullability(rightNull))}
					signature := must(functions.BindFunctionSignature(metadata, arguments))
					want := operator == foundation.Equal || operator == foundation.NotEqual || domain.IsOrderable()
					got, present := utils.RecognizeScalarComparison(metadata, signature).Get()
					if present != want || present && got != operator || utils.IsScalarComparisonDomain(operator, arguments) != want {
						t.Fatalf("recognition changed for %s operator=%v nullability=%v/%v", domain, operator, leftNull, rightNull)
					}
					pair, supported := utils.RecognizeScalarComparisonPair(metadata, arguments[0], arguments[1]).Get()
					if supported != want || supported && pair != operator {
						t.Fatal("pair recognition changed")
					}
					wantNull := foundation.NonNull
					if leftNull == foundation.MayNull || rightNull == foundation.MayNull {
						wantNull = foundation.MayNull
					}
					if signature.ResultType().Nullability() != wantNull || metadata.Signature().ResultType() != boolean {
						t.Fatal("recognition mutated the effective or implementation signature")
					}
				}
			}
		}
	}
}

func TestRecognizeScalarComparisonUnsupportedContracts(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	narrow := must(types.NewIntType(types.Int8, foundation.NonNull))
	float32Type := must(types.NewFloatType(types.Float32, foundation.NonNull))
	float64Type := must(types.NewFloatType(types.Float64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	nullable := must(integer.WithNullability(foundation.MayNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	bound := uint64(8)
	bounded := must(types.NewVarCharType(&bound, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	structure := must(types.NewStructType(nil, foundation.NonNull))
	propagate := []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}
	for _, tc := range []struct {
		name      string
		arguments []types.Type
		result    types.Type
		operator  foundation.Optional[foundation.OperatorType]
		policies  []functions.NullPolicy
	}{
		{"diagnostic name only", []types.Type{integer, integer}, boolean, foundation.Optional[foundation.OperatorType]{}, propagate},
		{"arithmetic", []types.Type{integer, integer}, integer, foundation.Some(foundation.Add), propagate},
		{"zero arguments", nil, boolean, foundation.Some(foundation.Equal), nil},
		{"one argument", []types.Type{integer}, boolean, foundation.Some(foundation.Equal), propagate[:1]},
		{"integer widths", []types.Type{integer, narrow}, boolean, foundation.Some(foundation.Equal), propagate},
		{"float widths", []types.Type{float32Type, float64Type}, boolean, foundation.Some(foundation.Equal), propagate},
		{"mixed families", []types.Type{integer, text}, boolean, foundation.Some(foundation.Equal), propagate},
		{"varchar bounds", []types.Type{text, bounded}, boolean, foundation.Some(foundation.Equal), propagate},
		{"arrays", []types.Type{array, array}, boolean, foundation.Some(foundation.Equal), propagate},
		{"structs", []types.Type{structure, structure}, boolean, foundation.Some(foundation.Equal), propagate},
		{"nullable result", []types.Type{integer, integer}, must(boolean.WithNullability(foundation.MayNull)), foundation.Some(foundation.Equal), propagate},
		{"nonboolean result", []types.Type{integer, integer}, integer, foundation.Some(foundation.Equal), propagate},
		{"unspecified left", []types.Type{integer, integer}, boolean, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.NullPolicyUnspecified, functions.PropagateNull}},
		{"unspecified right", []types.Type{integer, integer}, boolean, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}},
		{"pass null", []types.Type{nullable, integer}, boolean, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.PassNull, functions.PropagateNull}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := comparisonMetadata(tc.arguments, tc.result, tc.operator, tc.policies)
			signature := must(functions.BindFunctionSignature(metadata, tc.arguments))
			if len(tc.arguments) == 2 {
				if _, present := utils.RecognizeScalarComparisonPair(metadata, tc.arguments[0], tc.arguments[1]).Get(); present {
					t.Fatal("unsupported pair recognized")
				}
			}
			if _, present := utils.RecognizeScalarComparison(metadata, signature).Get(); present {
				t.Fatal("unsupported but valid contract recognized")
			}
		})
	}
}

func TestScalarDomainRejectsBeforeReadingArguments(t *testing.T) {
	// Unsupported operators and arities must be rejected before dereferencing
	// argument descriptors, including when debug nil assertions are enabled.
	for _, tc := range []struct {
		operator  foundation.OperatorType
		arguments []types.Type
	}{
		{foundation.Add, []types.Type{nil, nil}},
		{foundation.OperatorType(255), []types.Type{nil, nil}},
		{foundation.Equal, nil},
		{foundation.Equal, []types.Type{nil}},
		{foundation.Equal, []types.Type{nil, nil, nil}},
	} {
		if utils.IsScalarComparisonDomain(tc.operator, tc.arguments) {
			t.Fatal("unsupported domain accepted")
		}
	}
}
