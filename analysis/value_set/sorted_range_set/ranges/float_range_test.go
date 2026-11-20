package ranges

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestFloatNeighbors(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := rangeTestMust(types.NewFloatType(width, foundation.NonNull))
		tiny, maximum, aboveOne := math.SmallestNonzeroFloat64, math.MaxFloat64, math.Float64frombits(0x3ff0000000000001)
		if width == types.Float32 {
			tiny = math.SmallestNonzeroFloat32
			maximum = math.MaxFloat32
			aboveOne = float64(math.Float32frombits(0x3f800001))
		}
		for _, tc := range []struct {
			n       float64
			up      bool
			want    float64
			present bool
		}{
			{0, true, tiny, true},
			{math.Copysign(0, -1), true, tiny, true},
			{0, false, -tiny, true},
			{math.Copysign(0, -1), false, -tiny, true},
			{-tiny, true, 0, true},
			{tiny, false, 0, true},
			{1, true, aboveOne, true},
			{aboveOne, false, 1, true},
			{math.Inf(1), false, maximum, true},
			{math.Inf(-1), true, -maximum, true},
			{maximum, true, math.Inf(1), true},
			{-maximum, false, math.Inf(-1), true},
			{math.Inf(1), true, 0, false},
			{math.Inf(-1), false, 0, false},
		} {
			got, present := floatNeighbor(typ, tc.n, tc.up)
			if present != tc.present || (present && math.Float64bits(got) != math.Float64bits(tc.want)) {
				t.Fatalf("Float%d neighbor %g up=%v got=%g/%v want=%g/%v", width, tc.n, tc.up, got, present, tc.want, tc.present)
			}
		}
		if !floatAdjacent(typ, 1, aboveOne) || floatAdjacent(typ, aboveOne, 1) || floatAdjacent(typ, 0, 0) || floatAdjacent(typ, math.Inf(1), math.Inf(1)) {
			t.Fatal("wrong float adjacency")
		}
	}
}

func TestFloatRangeComparisons(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := rangeTestMust(types.NewFloatType(width, foundation.NonNull))
		v := func(n float64) valueinterfaces.Value { return rangeTestMust(testbedvalues.NewFloat(typ, n)) }
		sequence := []float64{math.Inf(-1), -1, -math.SmallestNonzeroFloat32, 0, math.SmallestNonzeroFloat32, 1, math.Inf(1)}
		for i, a := range sequence {
			for j, b := range sequence {
				order := rangeTestMust(compareRangeValues(v(a), v(b)))
				want := 0
				if i < j {
					want = -1
				}
				if i > j {
					want = 1
				}
				if order != want {
					t.Fatal("non-NaN numeric ordering")
				}
			}
		}
		for _, pair := range [][2]valueinterfaces.Value{{v(math.NaN()), v(0)}, {v(0), v(math.NaN())}, {v(math.NaN()), v(math.NaN())}} {
			if _, err := compareRangeValues(pair[0], pair[1]); !errors.Is(err, value_set.ErrInvalidInput) {
				t.Fatal("NaN comparison must fail", err)
			}
		}
		lower := rangeTestMust(NewRangeEndpoint(v(math.Copysign(0, -1)), true))
		upper := rangeTestMust(NewRangeEndpoint(v(0), false))
		if rangeTestMust(CompareLowerEndpoints(lower, upper)) != -1 || rangeTestMust(CompareUpperEndpoints(lower, upper)) != 1 {
			t.Fatal("zero sign or endpoint inclusion ordering")
		}
	}
}
