package functions_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestFloatComparisons(t *testing.T) {
	provider, err := testbed.NewStandardContext()
	if err != nil {
		t.Fatal(err)
	}
	operators := []foundation.OperatorType{foundation.Equal, foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ, err := types.NewFloatType(width, foundation.MayNull)
		if err != nil {
			t.Fatal(err)
		}
		names := []shared.BuiltInFunctionName{shared.BuiltInFloat32Equal, shared.BuiltInFloat32NotEqual, shared.BuiltInFloat32LessThan, shared.BuiltInFloat32LessThanOrEqual, shared.BuiltInFloat32GreaterThan, shared.BuiltInFloat32GreaterThanOrEqual}
		if width == types.Float64 {
			names = []shared.BuiltInFunctionName{shared.BuiltInFloat64Equal, shared.BuiltInFloat64NotEqual, shared.BuiltInFloat64LessThan, shared.BuiltInFloat64LessThanOrEqual, shared.BuiltInFloat64GreaterThan, shared.BuiltInFloat64GreaterThanOrEqual}
		}
		for _, tc := range []struct {
			a, b float64
			want [6]bool
		}{
			{math.NaN(), math.NaN(), [6]bool{false, true, false, false, false, false}},
			{math.NaN(), 1, [6]bool{false, true, false, false, false, false}},
			{1, math.NaN(), [6]bool{false, true, false, false, false, false}},
			{math.Copysign(0, -1), 0, [6]bool{true, false, false, true, false, true}},
			{math.Inf(-1), math.Inf(1), [6]bool{false, true, true, true, false, false}},
			{math.Inf(1), math.Inf(1), [6]bool{true, false, false, true, false, true}},
			{math.SmallestNonzeroFloat32, 0, [6]bool{false, true, false, false, true, true}},
			{1, 2, [6]bool{false, true, true, true, false, false}},
		} {
			a, err := testbedvalues.NewFloat(typ, tc.a)
			if err != nil {
				t.Fatal(err)
			}
			b, err := testbedvalues.NewFloat(typ, tc.b)
			if err != nil {
				t.Fatal(err)
			}
			for i, op := range operators {
				handle, err := provider.ComparisonFunction(op, typ, typ)
				if err != nil {
					t.Fatal(err)
				}
				builtin, err := provider.LookupBuiltInFunction(names[i], []types.Type{typ, typ})
				if err != nil || !handle.EqualsTo(builtin) {
					t.Fatal("builtin mismatch", err)
				}
				if got := evaluateStandard(t, provider, handle, a, b).(*testbedvalues.BoolValue).Bool(); got != tc.want[i] {
					t.Fatalf("%v %v %v: got %v", tc.a, op, tc.b, got)
				}
				null, err := testbedvalues.NewNull(typ)
				if err != nil {
					t.Fatal(err)
				}
				if !evaluateStandard(t, provider, handle, null, b).IsNull() {
					t.Fatal("null not propagated")
				}
			}
		}
	}
}
