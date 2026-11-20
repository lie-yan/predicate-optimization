package ranges_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

var _ fmt.Stringer = (*ranges.Range)(nil)

func checkRangeString(t *testing.T, typ types.Type, lower, upper ranges.RangeEndpoint, want string) {
	t.Helper()
	r, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, upper)).Get()
	if !present {
		t.Fatal("fixture unexpectedly empty")
	}
	beforeLower, beforeUpper, beforeType := r.Lower(), r.Upper(), r.Type()
	if got := r.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := fmt.Sprint(r); got != want {
		t.Fatalf("fmt.Sprint got %q, want %q", got, want)
	}
	if r.Lower() != beforeLower || r.Upper() != beforeUpper || r.Type() != beforeType {
		t.Fatal("formatting mutated range")
	}
}

func TestIntegerRangeString(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	for _, tc := range []struct {
		lower, upper ranges.RangeEndpoint
		want         string
	}{
		{testutil.IntEndpoint(typ, 10, true), testutil.IntEndpoint(typ, 299, false), "[10,298]"},
		{testutil.IntEndpoint(typ, 10, false), testutil.IntEndpoint(typ, 299, true), "[11,299]"},
		{testutil.IntEndpoint(typ, 10, true), testutil.IntEndpoint(typ, 10, true), "{10}"},
		{ranges.RangeEndpoint{}, testutil.IntEndpoint(typ, 10, false), "(-∞,9]"},
		{testutil.IntEndpoint(typ, 10, true), ranges.RangeEndpoint{}, "[10,+∞)"},
		{ranges.RangeEndpoint{}, ranges.RangeEndpoint{}, "(-∞,+∞)"},
		{testutil.IntEndpoint(typ, math.MinInt64, true), testutil.IntEndpoint(typ, math.MaxInt64, true), "(-∞,+∞)"},
		{testutil.IntEndpoint(typ, math.MinInt64, true), testutil.IntEndpoint(typ, math.MinInt64, true), "(-∞,-9223372036854775808]"},
	} {
		checkRangeString(t, typ, tc.lower, tc.upper, tc.want)
	}
}

func TestFloatRangeString(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		ep := func(number float64, inclusive bool) ranges.RangeEndpoint {
			return testutil.Must(ranges.NewRangeEndpoint(testutil.Must(testbedvalues.NewFloat(typ, number)), inclusive))
		}
		for _, tc := range []struct {
			lower, upper ranges.RangeEndpoint
			want         string
		}{
			{ep(10, true), ep(299, false), "[10,299)"},
			{ep(10, false), ep(299, true), "(10,299]"},
			{ep(10, false), ep(299, false), "(10,299)"},
			{ep(10, true), ep(299, true), "[10,299]"},
			{ep(10, true), ep(10, true), "{10}"},
			{ep(math.Copysign(0, -1), true), ep(0, true), "{0}"},
			{ranges.RangeEndpoint{}, ep(10, true), "(-∞,10]"},
			{ep(10, false), ranges.RangeEndpoint{}, "(10,+∞)"},
			{ranges.RangeEndpoint{}, ranges.RangeEndpoint{}, "(-∞,+∞)"},
			{ep(math.Inf(-1), false), ep(math.Inf(1), false), "(-Inf,+Inf)"},
			{ep(math.Inf(-1), true), ep(math.Inf(1), true), "(-∞,+∞)"},
			{ranges.RangeEndpoint{}, ep(math.Inf(-1), true), "(-∞,-Inf]"},
			{ep(math.Inf(1), true), ranges.RangeEndpoint{}, "[+Inf,+∞)"},
		} {
			checkRangeString(t, typ, tc.lower, tc.upper, tc.want)
		}
	}
}

func TestVarCharRangeString(t *testing.T) {
	typ := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	ep := func(s string, inclusive bool) ranges.RangeEndpoint {
		return testutil.Must(ranges.NewRangeEndpoint(testutil.Must(testbedvalues.NewVarChar(typ, s)), inclusive))
	}
	for _, tc := range []struct {
		lower, upper ranges.RangeEndpoint
		want         string
	}{
		{ep("a", true), ep("z", false), `["a","z")`},
		{ep("a", false), ep("z", true), `("a","z"]`},
		{ep("a", false), ep("z", false), `("a","z")`},
		{ep("a", true), ep("z", true), `["a","z"]`},
		{ep("a\"\\\n\x00\xff", true), ep("a\"\\\n\x00\xff", true), `{"a\"\\\n\x00\xff"}`},
		{ranges.RangeEndpoint{}, ep("z", true), `(-∞,"z"]`},
		{ep("a", true), ranges.RangeEndpoint{}, `["a",+∞)`},
		{ep("", true), ep("z", true), `(-∞,"z"]`},
	} {
		checkRangeString(t, typ, tc.lower, tc.upper, tc.want)
	}
}

func TestRangeStringPreconditions(t *testing.T) {
	for _, r := range []*ranges.Range{nil, {}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected assertion for unconstructed range")
				}
			}()
			_ = r.String()
		}()
	}
}
