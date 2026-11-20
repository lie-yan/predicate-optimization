package ranges_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	valuefixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func TestRangesWithExternalEndpoints(t *testing.T) {
	integer := testutil.Must(types.NewIntType(types.Int8, foundation.MayNull))
	f32 := testutil.Must(types.NewFloatType(types.Float32, foundation.NonNull))
	f64 := testutil.Must(types.NewFloatType(types.Float64, foundation.NonNull))
	text := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	for _, tc := range []struct {
		name      string
		typ       types.Type
		low, high valueinterfaces.Value
		inclusive bool
		want      string
		wantEmpty bool
	}{
		{"integer shift", integer, testutil.Must(testbedvalues.NewInteger(integer, 1)), testutil.Must(testbedvalues.NewInteger(integer, 3)), false, "{2}", false},
		{"integer singleton", integer, testutil.Must(testbedvalues.NewInteger(integer, 3)), testutil.Must(testbedvalues.NewInteger(integer, 3)), true, "{3}", false},
		{"integer extrema", integer, testutil.Must(testbedvalues.NewInteger(integer, -128)), testutil.Must(testbedvalues.NewInteger(integer, 127)), true, "(-∞,+∞)", false},
		{"varchar bytes", text, testutil.Must(testbedvalues.NewVarChar(text, "\x00")), testutil.Must(testbedvalues.NewVarChar(text, "\xff")), true, "[\"\\x00\",\"\\xff\"]", false},
		{"float32 adjacent open", f32, testutil.Must(testbedvalues.NewFloat(f32, 1)), testutil.Must(testbedvalues.NewFloat(f32, float64(math.Nextafter32(1, 2)))), false, "", true},
		{"float infinity", f64, testutil.Must(testbedvalues.NewFloat(f64, math.Inf(-1))), testutil.Must(testbedvalues.NewFloat(f64, math.Inf(1))), true, "(-∞,+∞)", false},
		{"float signed zero", f64, testutil.Must(testbedvalues.NewFloat(f64, math.Copysign(0, -1))), testutil.Must(testbedvalues.NewFloat(f64, 0)), true, "{0}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Exercise either side or both using external, non-comparable testbedvalues.
			for mask := 1; mask <= 3; mask++ {
				low, high := tc.low, tc.high
				if mask&1 != 0 {
					low = valuefixtures.WrapScalar(low)
				}
				if mask&2 != 0 {
					high = valuefixtures.WrapScalar(high)
				}
				lower := testutil.Must(ranges.NewRangeEndpoint(low, tc.inclusive))
				upper := testutil.Must(ranges.NewRangeEndpoint(high, tc.inclusive))
				result := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, tc.typ, lower, upper))
				interval, present := result.Get()
				if present == tc.wantEmpty {
					t.Fatal("incorrect emptiness")
				}
				if present && interval.String() != tc.want {
					t.Fatalf("got %s, want %s", interval.String(), tc.want)
				}
			}
		})
	}
}

func TestExternalEndpointComparisonAndRejection(t *testing.T) {
	typ := testutil.Must(types.NewFloatType(types.Float64, foundation.NonNull))
	negativeZero := valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1))))
	zero := testutil.Must(testbedvalues.NewFloat(typ, 0))
	lower := testutil.Must(ranges.NewRangeEndpoint(negativeZero, true))
	exclusive := testutil.Must(ranges.NewRangeEndpoint(zero, false))
	if testutil.Must(ranges.CompareLowerEndpoints(lower, exclusive)) != -1 || testutil.Must(ranges.CompareUpperEndpoints(lower, exclusive)) != 1 {
		t.Fatal("equal-payload endpoint inclusion ordering changed")
	}
	positive := testutil.Must(ranges.NewRangeEndpoint(zero, true))
	if testutil.Must(ranges.CompareLowerEndpoints(lower, positive)) != 0 || testutil.Must(ranges.CompareLowerEndpoints(positive, lower)) != 0 {
		t.Fatal("mixed signed zeros do not compare equally")
	}
	testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, positive))
	if !math.Signbit(negativeZero.Float()) {
		t.Fatal("normalization mutated external payload")
	}
	nan := testutil.Must(ranges.NewRangeEndpoint(valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.NaN()))), true))
	if _, err := ranges.NewRange(testbedvalues.ValueFactory{}, typ, nan, positive); !errors.Is(err, value_set.ErrInvalidInput) {
		t.Fatal("external NaN endpoint accepted")
	}
	integer := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	other := testutil.Must(ranges.NewRangeEndpoint(valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewInteger(integer, 0))), true))
	if _, err := ranges.CompareLowerEndpoints(lower, other); !errors.Is(err, value_set.ErrIncompatibleTypes) {
		t.Fatal("incompatible endpoint types accepted")
	}
	boolean := testutil.Must(types.NewBoolType(foundation.NonNull))
	unsupported := testutil.Must(ranges.NewRangeEndpoint(valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewBool(boolean, false))), true))
	if _, err := ranges.NewRange(testbedvalues.ValueFactory{}, boolean, unsupported, unsupported); !errors.Is(err, value_set.ErrUnsupported) {
		t.Fatal("unsupported domain accepted")
	}
}
