package equatable_value_set_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	equatable "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestFloatSetConstruction(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		nullable := testutil.Must(types.NewFloatType(width, foundation.MayNull))
		negativeZero := testutil.Must(testbedvalues.NewFloat(nullable, math.Copysign(0, -1)))
		input := []valueinterfaces.Value{negativeZero, testutil.Must(testbedvalues.NewFloat(typ, 0)), testutil.Must(testbedvalues.NewFloat(typ, math.NaN())), testutil.Must(testbedvalues.NewFloat(nullable, math.NaN())), testutil.Must(testbedvalues.NewFloat(typ, math.Inf(1))), testutil.Must(testbedvalues.NewFloat(typ, math.Inf(-1))), testutil.Must(testbedvalues.NewFloat(typ, math.SmallestNonzeroFloat32)), testutil.Must(testbedvalues.NewFloat(typ, math.MaxFloat32))}
		set := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, nullable, input))
		got := set.Values()
		if len(got) != 6 || !math.IsInf(got[0].(*testbedvalues.FloatValue).Float(), -1) || math.Signbit(got[1].(*testbedvalues.FloatValue).Float()) || got[1].(*testbedvalues.FloatValue).Float() != 0 || !math.IsNaN(got[5].(*testbedvalues.FloatValue).Float()) {
			t.Fatal("bad set canonicalization")
		}
		for _, value := range got {
			if !value.Type().EqualsTo(typ) {
				t.Fatal("bad normalized type")
			}
		}
		if !math.Signbit(negativeZero.Float()) {
			t.Fatal("input mutated")
		}
		input[0], got[0] = nil, nil
		if set.Values()[0] == nil {
			t.Fatal("slice escaped")
		}
		otherWidth := types.Float32
		if width == types.Float32 {
			otherWidth = types.Float64
		}
		other := testutil.Must(types.NewFloatType(otherWidth, foundation.NonNull))
		if _, err := equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewFloat(other, 1))}); !errors.Is(err, value_set.ErrIncompatibleTypes) {
			t.Fatal(err)
		}
		for _, invalid := range []valueinterfaces.Value{testutil.Must(testbedvalues.NewNull(nullable))} {
			if _, err := equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{invalid}); !errors.Is(err, value_set.ErrInvalidInput) {
				t.Fatal(err)
			}
		}
	}
}
