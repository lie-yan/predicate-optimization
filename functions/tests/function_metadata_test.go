package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

var (
	noBuiltIn  foundation.Optional[functions.BuiltInFunctionName]
	noOperator foundation.Optional[foundation.OperatorType]
)

func metadataSignature(t *testing.T) *functions.FunctionSignature {
	t.Helper()
	nonNull, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	nullable, err := nonNull.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	result, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := functions.NewFunctionSignature([]types.Type{nonNull, nullable}, result)
	if err != nil {
		t.Fatal(err)
	}
	return signature
}

func TestFunctionMetadataOwnership(t *testing.T) {
	signature := metadataSignature(t)
	policies := []functions.NullPolicy{functions.PropagateNull, functions.PassNull}
	metadata, err := functions.NewFunctionMetadata(signature, "custom", noBuiltIn, noOperator, policies)
	if err != nil {
		t.Fatal(err)
	}
	policies[0] = functions.PassNull
	got := metadata.NullPolicies()
	if got[0] != functions.PropagateNull || got[1] != functions.PassNull {
		t.Fatal("metadata must copy input policies")
	}
	got[1] = functions.NullPolicyUnspecified
	if metadata.NullPolicies()[1] != functions.PassNull {
		t.Fatal("returned policies must be copied")
	}
	if metadata.Signature() != signature || metadata.DisplayName() != "custom" {
		t.Fatal("signature must be shared and display text preserved")
	}
	if name, present := metadata.BuiltInName().Get(); present || name != "" {
		t.Fatal("absent built-in classification became established")
	}
	if operator, present := metadata.Operator().Get(); present || operator != 0 {
		t.Fatal("absent operator classification became established")
	}
}

func TestFunctionMetadataClassification(t *testing.T) {
	signature := metadataSignature(t)
	policies := []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}
	metadata, err := functions.NewFunctionMetadata(signature, "comparison", noBuiltIn, foundation.Some(foundation.Equal), policies)
	if err != nil {
		t.Fatal(err)
	}
	if operator, present := metadata.Operator().Get(); !present || operator != foundation.Equal {
		t.Fatal("operator classification lost")
	}
	boolean := &types.BoolType{}
	notSignature, err := functions.NewFunctionSignature([]types.Type{boolean}, boolean)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err = functions.NewFunctionMetadata(notSignature, "", foundation.Some(functions.BuiltInNot), noOperator, []functions.NullPolicy{functions.PropagateNull})
	if err != nil {
		t.Fatal(err)
	}
	if name, present := metadata.BuiltInName().Get(); !present || name != functions.BuiltInNot {
		t.Fatal("built-in classification lost")
	}
}

func TestFunctionMetadataRejectsInconsistentInputs(t *testing.T) {
	signature := metadataSignature(t)
	validPolicies := []functions.NullPolicy{functions.PropagateNull, functions.PassNull}
	for _, tc := range []struct {
		name      string
		signature *functions.FunctionSignature
		builtIn   foundation.Optional[functions.BuiltInFunctionName]
		operator  foundation.Optional[foundation.OperatorType]
		policies  []functions.NullPolicy
	}{
		{"nil signature", nil, noBuiltIn, noOperator, nil},
		{"zero signature", &functions.FunctionSignature{}, noBuiltIn, noOperator, nil},
		{"unknown operator", signature, noBuiltIn, foundation.Some(foundation.OperatorType(255)), validPolicies},
		{"present empty built-in", signature, foundation.Some(functions.BuiltInFunctionName("")), noOperator, validPolicies},
		{"present zero operator", signature, noBuiltIn, foundation.Some(foundation.OperatorType(0)), validPolicies},
		{"missing policies", signature, noBuiltIn, noOperator, nil},
		{"extra policies", signature, noBuiltIn, noOperator, []functions.NullPolicy{0, 0, 0}},
		{"invalid policy", signature, noBuiltIn, noOperator, []functions.NullPolicy{255, 0}},
		{"PassNull non-null parameter", signature, noBuiltIn, noOperator, []functions.NullPolicy{functions.PassNull, functions.PassNull}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata, err := functions.NewFunctionMetadata(tc.signature, "", tc.builtIn, tc.operator, tc.policies)
			if err == nil || metadata != nil {
				t.Fatal("inconsistent metadata accepted")
			}
		})
	}
}

func TestFunctionMetadataUnspecifiedAndZeroArity(t *testing.T) {
	signature := metadataSignature(t)
	if _, err := functions.NewFunctionMetadata(signature, "", noBuiltIn, noOperator, []functions.NullPolicy{0, 0}); err != nil {
		t.Fatal(err)
	}
	zeroArity, err := functions.NewFunctionSignature(nil, signature.ResultType())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := functions.NewFunctionMetadata(zeroArity, "constant", noBuiltIn, noOperator, nil)
	if err != nil || metadata == nil {
		t.Fatalf("zero-arity metadata rejected: %v", err)
	}
}

func TestFunctionMetadataNamedOperator(t *testing.T) {
	integer, err := types.NewIntType(types.Int8, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := functions.NewFunctionSignature([]types.Type{integer, integer}, integer)
	if err != nil {
		t.Fatal(err)
	}
	name := functions.BuiltInFunctionName("int8.add")
	metadata, err := functions.NewFunctionMetadata(signature, "addition", foundation.Some(name), foundation.Some(foundation.Add), []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := metadata.BuiltInName().Get(); !ok || got != name {
		t.Fatal("built-in identity lost")
	}
	if got, ok := metadata.Operator().Get(); !ok || got != foundation.Add {
		t.Fatal("operator identity lost")
	}
}
