package functions_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func bindingInt(t *testing.T, width types.IntWidth, n foundation.Nullability) types.Type {
	t.Helper()
	typ, err := types.NewIntType(width, n)
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func bindingMetadata(t *testing.T, args []types.Type, result types.Type, policies []functions.NullPolicy) *functions.FunctionMetadata {
	t.Helper()
	signature, err := functions.NewFunctionSignature(args, result)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := functions.NewFunctionMetadata(signature, "binding fixture", noBuiltIn, noOperator, policies)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func TestBindFunctionSignatureNullPolicies(t *testing.T) {
	nonNull := bindingInt(t, types.Int64, foundation.NonNull)
	nullable := bindingInt(t, types.Int64, foundation.MayNull)
	for _, tc := range []struct {
		name     string
		params   []types.Type
		args     []types.Type
		policies []functions.NullPolicy
		result   types.Type
		want     types.Type
	}{
		{"propagate non-null", []types.Type{nonNull}, []types.Type{nonNull}, []functions.NullPolicy{functions.PropagateNull}, nonNull, nonNull},
		{"propagate nullable", []types.Type{nonNull}, []types.Type{nullable}, []functions.NullPolicy{functions.PropagateNull}, nonNull, nullable},
		{"declared nullable result", []types.Type{nonNull}, []types.Type{nonNull}, []functions.NullPolicy{functions.PropagateNull}, nullable, nullable},
		{"pass nullable", []types.Type{nullable}, []types.Type{nullable}, []functions.NullPolicy{functions.PassNull}, nonNull, nonNull},
		{"pass non-null", []types.Type{nullable}, []types.Type{nonNull}, []functions.NullPolicy{functions.PassNull}, nonNull, nonNull},
		{"mixed passing null", []types.Type{nonNull, nullable}, []types.Type{nonNull, nullable}, []functions.NullPolicy{functions.PropagateNull, functions.PassNull}, nonNull, nonNull},
		{"mixed propagating null", []types.Type{nonNull, nullable}, []types.Type{nullable, nullable}, []functions.NullPolicy{functions.PropagateNull, functions.PassNull}, nonNull, nullable},
		{"unspecified non-null", []types.Type{nonNull}, []types.Type{nonNull}, []functions.NullPolicy{functions.NullPolicyUnspecified}, nonNull, nonNull},
		{"unspecified nullable parameter non-null input", []types.Type{nullable}, []types.Type{nonNull}, []functions.NullPolicy{functions.NullPolicyUnspecified}, nonNull, nonNull},
		{"zero arity", nil, nil, nil, nonNull, nonNull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := bindingMetadata(t, tc.params, tc.result, tc.policies)
			checkErr := functions.CheckFunctionBinding(metadata, tc.args)
			signature, err := functions.BindFunctionSignature(metadata, tc.args)
			if (checkErr == nil) != (err == nil) || (err != nil && checkErr.Error() != err.Error()) {
				t.Fatalf("check and bind disagree: %v / %v", checkErr, err)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !signature.ResultType().EqualsTo(tc.want) {
				t.Fatalf("got %s, want %s", signature.ResultType(), tc.want)
			}
			exact := tc.result.EqualsTo(tc.want)
			for i, argument := range tc.args {
				exact = exact && argument.EqualsTo(tc.params[i])
			}
			if (signature == metadata.Signature()) != exact {
				t.Fatal("only exact bindings should reuse the implementation signature")
			}
			for i, argument := range signature.ArgumentTypes() {
				if argument != tc.args[i] {
					t.Fatal("effective argument descriptors should be retained")
				}
			}
			if metadata.Signature().ResultType() != tc.result {
				t.Fatal("binding mutated the implementation signature")
			}
			if err := functions.ValidateCallSignature(metadata, signature); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExactBindingSharesSignatureWithoutBorrowingInputSlice(t *testing.T) {
	parameter := bindingInt(t, types.Int64, foundation.NonNull)
	argument := bindingInt(t, types.Int64, foundation.NonNull)
	metadata := bindingMetadata(t, []types.Type{parameter}, parameter, []functions.NullPolicy{functions.PropagateNull})
	args := []types.Type{argument}
	signature, err := functions.BindFunctionSignature(metadata, args)
	if err != nil || signature != metadata.Signature() {
		t.Fatalf("equal independently constructed Types should reuse the signature: %v", err)
	}
	args[0] = bindingInt(t, types.Int8, foundation.NonNull)
	returned := signature.ArgumentTypes()
	returned[0] = args[0]
	if signature.ArgumentTypeAt(0) != parameter {
		t.Fatal("caller mutation changed shared implementation metadata")
	}
}

func TestBindingRejectsUnsupportedInputs(t *testing.T) {
	nonNull := bindingInt(t, types.Int64, foundation.NonNull)
	nullable := bindingInt(t, types.Int64, foundation.MayNull)
	for _, tc := range []struct {
		name   string
		param  types.Type
		args   []types.Type
		policy functions.NullPolicy
	}{
		{"missing argument", nonNull, nil, functions.PropagateNull},
		{"extra argument", nonNull, []types.Type{nonNull, nonNull}, functions.PropagateNull},
		{"integer width", nonNull, []types.Type{bindingInt(t, types.Int8, foundation.NonNull)}, functions.PropagateNull},
		{"unspecified nullable argument", nonNull, []types.Type{nullable}, functions.NullPolicyUnspecified},
		{"unspecified already nullable parameter", nullable, []types.Type{nullable}, functions.NullPolicyUnspecified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := bindingMetadata(t, []types.Type{tc.param}, nonNull, []functions.NullPolicy{tc.policy})
			checkErr := functions.CheckFunctionBinding(metadata, tc.args)
			signature, err := functions.BindFunctionSignature(metadata, tc.args)
			if (checkErr == nil) != (err == nil) || (err != nil && checkErr.Error() != err.Error()) {
				t.Fatalf("check and bind disagree: %v / %v", checkErr, err)
			}
			if signature != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
				t.Fatalf("expected unsupported binding, got %v", err)
			}
		})
	}
}

func TestBindingPreservesTypeConstraints(t *testing.T) {
	integer := bindingInt(t, types.Int64, foundation.NonNull)
	nullableInteger := bindingInt(t, types.Int64, foundation.MayNull)
	array, err := types.NewArrayType(integer, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	nullableArray, err := array.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	differentElements, err := types.NewArrayType(nullableInteger, foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	structType, err := types.NewStructType([]types.StructField{{Name: "value", Type: integer}}, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	differentField, err := types.NewStructType([]types.StructField{{Name: "value", Type: nullableInteger}}, foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	limit := uint64(10)
	bounded, err := types.NewVarCharType(&limit, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	unbounded, err := types.NewVarCharType(nil, foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		param    types.Type
		argument types.Type
	}{
		{"array elements", array, differentElements},
		{"struct field", structType, differentField},
		{"varchar bound", bounded, unbounded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := bindingMetadata(t, []types.Type{tc.param}, integer, []functions.NullPolicy{functions.PropagateNull})
			if _, err := functions.BindFunctionSignature(metadata, []types.Type{tc.argument}); !errors.Is(err, functions.ErrUnsupportedLookup) {
				t.Fatalf("nested or type-specific constraint changed: %v", err)
			}
		})
	}
	metadata := bindingMetadata(t, []types.Type{array}, array, []functions.NullPolicy{functions.PropagateNull})
	bound, err := functions.BindFunctionSignature(metadata, []types.Type{nullableArray})
	if err != nil || !bound.ResultType().EqualsTo(nullableArray) {
		t.Fatalf("outer nullability adaptation failed: %v", err)
	}
	if array.Nullability() != foundation.NonNull || array.Element() != integer {
		t.Fatal("source composite was changed")
	}
}

func TestValidateCallSignatureRejectsWrongResult(t *testing.T) {
	nonNull := bindingInt(t, types.Int64, foundation.NonNull)
	nullable := bindingInt(t, types.Int64, foundation.MayNull)
	metadata := bindingMetadata(t, []types.Type{nonNull}, nonNull, []functions.NullPolicy{functions.PropagateNull})
	for _, tc := range []struct {
		argument types.Type
		result   types.Type
	}{
		{nullable, nonNull}, // Lost nullable result.
		{nonNull, nullable}, // Invented nullable result.
		{nonNull, bindingInt(t, types.Int8, foundation.NonNull)},
	} {
		signature, err := functions.NewFunctionSignature([]types.Type{tc.argument}, tc.result)
		if err != nil {
			t.Fatal(err)
		}
		if err := functions.ValidateCallSignature(metadata, signature); !errors.Is(err, functions.ErrContractViolation) {
			t.Fatalf("arbitrary result accepted: %v", err)
		}
	}
}
