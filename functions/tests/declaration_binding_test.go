package functions_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func boundedText(t *testing.T, bound *uint64, n foundation.Nullability) *types.VarCharType {
	t.Helper()
	text, err := types.NewVarCharType(bound, n)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestDeclarationBindingExactBoundsAndResult(t *testing.T) {
	x := varcharPattern(t, "x", foundation.NonNull)
	repeated := declaration(t, []types.TypePattern{x, x}, x)
	zero, small, large, max := uint64(0), uint64(32), uint64(64), uint64(math.MaxUint64)
	bounds := []*uint64{nil, &zero, &small, &large, &max}
	for i, left := range bounds {
		for j, right := range bounds {
			actual := []types.Type{boundedText(t, left, foundation.MayNull), boundedText(t, right, foundation.NonNull)}
			bound, err := functions.BindDeclarationSignature(repeated, actual)
			if i != j {
				if bound != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
					t.Fatalf("mismatched bounds %d/%d were not a lookup miss: %v", i, j, err)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := boundedText(t, left, foundation.NonNull)
			if !bound.ArgumentTypeAt(0).EqualsTo(expected) || !bound.ArgumentTypeAt(1).EqualsTo(expected) || !bound.ResultType().EqualsTo(expected) {
				t.Fatal("substitution lost the bound or declaration nullability")
			}
			if actual[0].Nullability() != foundation.MayNull {
				t.Fatal("binding changed actual argument nullability")
			}
		}
	}
	// Independent parameters may differ; the result selects the declared parameter.
	y := varcharPattern(t, "y", foundation.NonNull)
	separate := declaration(t, []types.TypePattern{x, y}, y)
	bound, err := functions.BindDeclarationSignature(separate, []types.Type{boundedText(t, &small, foundation.NonNull), boundedText(t, &large, foundation.NonNull)})
	if err != nil {
		t.Fatal(err)
	}
	if !bound.ResultType().EqualsTo(boundedText(t, &large, foundation.NonNull)) {
		t.Fatal("result used the wrong parameter")
	}
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	for _, actual := range [][]types.Type{nil, {integer, integer}, {boundedText(t, &small, foundation.NonNull)}} {
		if result, err := functions.BindDeclarationSignature(repeated, actual); result != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
			t.Fatal("wrong arity/domain was not a lookup miss")
		}
	}
}

func TestDeclarationBindingComposesWithNullPolicies(t *testing.T) {
	size := uint64(32)
	text := boundedText(t, &size, foundation.MayNull)
	boolean, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []functions.NullPolicy{functions.PropagateNull, functions.PassNull, functions.NullPolicyUnspecified} {
		n := foundation.NonNull
		if policy == functions.PassNull {
			n = foundation.MayNull
		}
		pattern := varcharPattern(t, "length", n)
		decl := declaration(t, []types.TypePattern{pattern}, types.NewConcreteTypePattern(boolean))
		implementation, err := functions.BindDeclarationSignature(decl, []types.Type{text})
		if err != nil {
			t.Fatal(err)
		}
		metadata, err := functions.NewFunctionMetadata(implementation, "test", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{policy})
		if err != nil {
			t.Fatal(err)
		}
		call, err := functions.BindFunctionSignature(metadata, []types.Type{text})
		if policy == functions.NullPolicyUnspecified {
			if call != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
				t.Fatal("domain binding must not bypass unspecified null policy")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		expected := foundation.NonNull
		if policy == functions.PropagateNull {
			expected = foundation.MayNull
		}
		if call.ResultType().Nullability() != expected || !call.ArgumentTypeAt(0).EqualsTo(text) {
			t.Fatal("effective signature lost null policy or actual argument Type")
		}
		if implementation.ArgumentTypeAt(0).Nullability() != n || implementation.ResultType() != boolean {
			t.Fatal("call adaptation changed bound implementation metadata")
		}
	}
}

func TestFixedDeclarationBindingRetainsNestedConstraints(t *testing.T) {
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	nullable, err := integer.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	array, err := types.NewArrayType(integer, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := array.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	nested, err := types.NewArrayType(nullable, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := functions.NewFunctionSignature([]types.Type{array}, integer)
	if err != nil {
		t.Fatal(err)
	}
	decl := functions.DeclarationFromSignature(fixed)
	bound, err := functions.BindDeclarationSignature(decl, []types.Type{outer})
	if err != nil {
		t.Fatal(err)
	}
	if bound.ArgumentTypeAt(0) != array || bound.ResultType() != integer {
		t.Fatal("fixed binding should share declaration descriptors")
	}
	if result, err := functions.BindDeclarationSignature(decl, []types.Type{nested}); result != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatal("nested nullability was weakened")
	}
	zero, err := functions.NewFunctionSignature(nil, integer)
	if err != nil {
		t.Fatal(err)
	}
	bound, err = functions.BindDeclarationSignature(functions.DeclarationFromSignature(zero), nil)
	if err != nil {
		t.Fatal(err)
	}
	if bound.ArgumentCount() != 0 || bound.ResultType() != integer {
		t.Fatal("zero-argument fixed binding changed")
	}
}
