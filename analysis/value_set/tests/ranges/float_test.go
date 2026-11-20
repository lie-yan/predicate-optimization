package ranges_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	ranges "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestFloatRangeConsecutiveWindows(t *testing.T) {
	// Explicit bit encodings give independent three-value universes across the
	// subnormal/normal transition, precision boundaries, zero and infinities.
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		var windows [][3]float64
		if width == types.Float32 {
			for _, bits := range [][3]uint32{{0, 1, 2}, {0x007fffff, 0x00800000, 0x00800001}, {0x3f7fffff, 0x3f800000, 0x3f800001}, {0x7f7ffffe, 0x7f7fffff, 0x7f800000}} {
				windows = append(windows, [3]float64{float64(math.Float32frombits(bits[0])), float64(math.Float32frombits(bits[1])), float64(math.Float32frombits(bits[2]))})
			}
		} else {
			for _, bits := range [][3]uint64{{0, 1, 2}, {0x000fffffffffffff, 0x0010000000000000, 0x0010000000000001}, {0x3fefffffffffffff, 0x3ff0000000000000, 0x3ff0000000000001}, {0x7feffffffffffffe, 0x7fefffffffffffff, 0x7ff0000000000000}} {
				windows = append(windows, [3]float64{math.Float64frombits(bits[0]), math.Float64frombits(bits[1]), math.Float64frombits(bits[2])})
			}
		}
		originalCount := len(windows)
		for i := 0; i < originalCount; i++ {
			w := windows[i]
			windows = append(windows, [3]float64{-w[2], -w[1], -w[0]})
		}
		tiny := windows[0][1]
		windows = append(windows, [3]float64{-tiny, 0, tiny})
		for _, window := range windows {
			for _, lo := range window {
				for _, hi := range window {
					for _, li := range []bool{false, true} {
						for _, ui := range []bool{false, true} {
							lower, upper := testutil.FloatEndpoint(typ, lo, li), testutil.FloatEndpoint(typ, hi, ui)
							result, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, upper)).Get()
							count := 0
							for _, n := range window {
								if testutil.AcceptsFloat(lower, upper, n) {
									count++
								}
							}
							if present != (count > 0) {
								t.Fatalf("Float%d range (%g,%g) flags %v/%v: present=%v count=%d", width, lo, hi, li, ui, present, count)
							}
							if !present {
								continue
							}
							probes := append([]float64{math.Inf(-1), math.Inf(1), math.NaN(), math.Copysign(0, -1)}, window[:]...)
							for _, n := range probes {
								if testutil.AcceptsFloat(lower, upper, n) != testutil.AcceptsFloat(result.Lower(), result.Upper(), n) {
									t.Fatal("normalization changed membership")
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestFloatUnboundedAndZeroNormalization(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.MayNull))
		unbounded := ranges.RangeEndpoint{}
		for _, pair := range [][2]ranges.RangeEndpoint{
			{unbounded, unbounded}, {testutil.FloatEndpoint(typ, math.Inf(-1), true), testutil.FloatEndpoint(typ, math.Inf(1), true)},
		} {
			result, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, pair[0], pair[1])).Get()
			if !present || !result.Lower().IsUnbounded() || !result.Upper().IsUnbounded() || result.Type().Nullability() != foundation.NonNull {
				t.Fatal("ordered universe normalization")
			}
		}
		for _, pair := range [][2]ranges.RangeEndpoint{
			{testutil.FloatEndpoint(typ, math.Inf(1), false), unbounded}, {unbounded, testutil.FloatEndpoint(typ, math.Inf(-1), false)},
		} {
			if _, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, pair[0], pair[1])).Get(); present {
				t.Fatal("range beyond infinity")
			}
		}
		for _, n := range []float64{math.Inf(-1), math.Inf(1)} {
			result, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, testutil.FloatEndpoint(typ, n, true), testutil.FloatEndpoint(typ, n, true))).Get()
			if !present || !testutil.AcceptsFloat(result.Lower(), result.Upper(), n) || testutil.AcceptsFloat(result.Lower(), result.Upper(), 0) {
				t.Fatal("infinity singleton lost")
			}
		}
		source := testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1)))
		bound := testutil.Must(ranges.NewRangeEndpoint(source, true))
		result, _ := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, bound, testutil.FloatEndpoint(typ, 0, true))).Get()
		for _, ep := range []ranges.RangeEndpoint{result.Lower(), result.Upper()} {
			value, _ := ep.Value().Get()
			if math.Signbit(value.(*testbedvalues.FloatValue).Float()) || !ep.IsInclusive() {
				t.Fatal("range zero not canonical")
			}
		}
		if !math.Signbit(source.Float()) || source.Type() != typ {
			t.Fatal("source literal changed")
		}
		retained := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, testutil.FloatEndpoint(typ, 0, false), testutil.FloatEndpoint(typ, 1, false)))
		r, present := retained.Get()
		if !present || r.Lower().IsInclusive() || r.Upper().IsInclusive() {
			t.Fatal("ordinary cuts changed")
		}
	}
}

func TestFloatRangeErrors(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		nan := testutil.FloatEndpoint(typ, math.NaN(), true)
		for _, pair := range [][2]ranges.RangeEndpoint{
			{nan, {}}, {{}, nan}, {nan, nan}, {testutil.FloatEndpoint(typ, math.Inf(1), false), nan}, {nan, testutil.FloatEndpoint(typ, math.Inf(-1), false)},
		} {
			result, err := ranges.NewRange(testbedvalues.ValueFactory{}, typ, pair[0], pair[1])
			if !errors.Is(err, value_set.ErrInvalidInput) {
				t.Fatal("NaN endpoint admitted or masked", err)
			}
			if _, present := result.Get(); present {
				t.Fatal("range returned on error")
			}
		}
		other := types.Float32
		if width == types.Float32 {
			other = types.Float64
		}
		otherType := testutil.Must(types.NewFloatType(other, foundation.NonNull))
		if _, err := ranges.NewRange(testbedvalues.ValueFactory{}, typ, testutil.FloatEndpoint(typ, math.Inf(1), false), testutil.FloatEndpoint(otherType, 0, true)); !errors.Is(err, value_set.ErrIncompatibleTypes) {
			t.Fatal("width mismatch hidden", err)
		}
	}
}
