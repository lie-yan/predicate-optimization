package functions_test

import (
	"errors"
	"math"
	"testing"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func evaluateStandard(t *testing.T, provider *testbed.Context, handle functioninterfaces.FunctionHandle, args ...valueinterfaces.Value) valueinterfaces.Value {
	t.Helper()
	metadata, err := provider.Metadata(handle)
	if err != nil {
		t.Fatal(err)
	}
	signature := evalSignature(t, metadata, args...)
	result, err := provider.Evaluate(handle, signature, args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Type().EqualsTo(signature.ResultType()) {
		t.Fatal("result violates effective type")
	}
	return result
}

func TestStandardNot(t *testing.T) {
	provider, err := testbed.NewStandardContext()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		typ, err := types.NewBoolType(n)
		if err != nil {
			t.Fatal(err)
		}
		handle, err := provider.LookupBuiltInFunction(shared.BuiltInNot, []types.Type{typ})
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range []bool{false, true} {
			argument, err := testbedvalues.NewBool(typ, input)
			if err != nil {
				t.Fatal(err)
			}
			result := evaluateStandard(t, provider, handle, argument)
			if result.(*testbedvalues.BoolValue).Bool() == input || result.Type().Nullability() != n {
				t.Fatal("incorrect NOT result")
			}
		}
		if n == foundation.MayNull {
			null, err := testbedvalues.NewNull(typ)
			if err != nil {
				t.Fatal(err)
			}
			if !evaluateStandard(t, provider, handle, null).IsNull() {
				t.Fatal("NOT null must be null")
			}
		}
	}
}

func TestStandardComparisonFixtures(t *testing.T) {
	provider, err := testbed.NewStandardContext()
	if err != nil {
		t.Fatal(err)
	}
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	text, err := types.NewVarCharType(nil, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	stringValue := func(s string) valueinterfaces.Value {
		v, err := testbedvalues.NewVarChar(text, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	type fixture struct {
		left, right valueinterfaces.Value
		order       int
	}
	cases := []fixture{
		{evalInteger(t, integer, -1), evalInteger(t, integer, 0), -1},
		{evalInteger(t, integer, 0), evalInteger(t, integer, 0), 0},
		{evalInteger(t, integer, 1), evalInteger(t, integer, 0), 1},
		{evalInteger(t, integer, math.MinInt64), evalInteger(t, integer, math.MaxInt64), -1},
		{evalInteger(t, integer, math.MaxInt64), evalInteger(t, integer, math.MinInt64), 1},
		{stringValue("tsa_01"), stringValue("tsa_02"), -1},
		{stringValue("tsa_03"), stringValue("tsa_03"), 0},
		{stringValue("tsa_02"), stringValue("tsa_01"), 1},
		{stringValue(""), stringValue("a"), -1},
		{stringValue("a"), stringValue("aa"), -1},
		{stringValue("A"), stringValue("a"), -1},
		{stringValue("é"), stringValue("z"), 1},
	}
	operators := []foundation.OperatorType{foundation.Equal, foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual}
	truth := map[int][]bool{-1: {false, true, true, true, false, false}, 0: {true, false, false, true, false, true}, 1: {false, true, false, false, true, true}}
	for _, tc := range cases {
		for i, operator := range operators {
			for _, nullable := range []bool{false, true} {
				left, right := tc.left, tc.right
				if nullable {
					left, err = left.WithNullability(foundation.MayNull)
					if err != nil {
						t.Fatal(err)
					}
					right, err = right.WithNullability(foundation.MayNull)
					if err != nil {
						t.Fatal(err)
					}
				}
				handle, err := provider.ComparisonFunction(operator, left.Type(), right.Type())
				if err != nil {
					t.Fatal(err)
				}
				result := evaluateStandard(t, provider, handle, left, right)
				if result.(*testbedvalues.BoolValue).Bool() != truth[tc.order][i] {
					t.Fatalf("comparison %d, order %d: wrong result", operator, tc.order)
				}
				if nullable {
					null, err := testbedvalues.NewNull(left.Type())
					if err != nil {
						t.Fatal(err)
					}
					for _, args := range [][]valueinterfaces.Value{{null, right}, {left, null}, {null, null}} {
						if !evaluateStandard(t, provider, handle, args...).IsNull() {
							t.Fatal("comparison with null must return null")
						}
					}
				}
			}
		}
	}
}

func TestStandardUnsupportedAndPairing(t *testing.T) {
	provider, err := testbed.NewStandardContext()
	if err != nil {
		t.Fatal(err)
	}
	other, err := testbed.NewStandardContext()
	if err != nil {
		t.Fatal(err)
	}
	boolean, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := types.NewIntType(types.Int8, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	limit := uint64(20)
	bounded, err := types.NewVarCharType(&limit, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][2]types.Type{{boolean, integer}, {integer, narrow}, {bounded, bounded}} {
		if _, err := provider.ComparisonFunction(foundation.Equal, args[0], args[1]); !errors.Is(err, shared.ErrUnsupportedLookup) {
			t.Fatal("unsupported overload accepted")
		}
	}
	if _, err := provider.ArithmeticFunction(foundation.Add, []types.Type{integer, narrow}); !errors.Is(err, shared.ErrUnsupportedLookup) {
		t.Fatal("mixed-width arithmetic accepted")
	}
	handle, err := provider.LookupBuiltInFunction(shared.BuiltInNot, []types.Type{boolean})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Metadata(handle); !errors.Is(err, shared.ErrIncompatibleHandle) {
		t.Fatal("independent standard contexts share identity")
	}
}
