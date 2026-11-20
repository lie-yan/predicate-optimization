package sorted_range_set_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

var _ fmt.Stringer = (*sets.SortedRangeSet)(nil)

func checkRangeSetString(t *testing.T, typ types.Type, input []*ranges.Range, nan bool, want string) {
	t.Helper()
	set := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, nan))
	before, originalType := set.Ranges(), set.Type()
	original := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, before, nan))
	for round := 0; round < 2; round++ {
		if got := set.String(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		if got := fmt.Sprint(set); got != want {
			t.Fatalf("fmt.Sprint got %q, want %q", got, want)
		}
		after := set.Ranges()
		if len(after) != len(before) || set.IncludesNaN() != nan || set.Type() != originalType {
			t.Fatal("formatting changed representation")
		}
		for i, r := range before {
			if after[i] != r {
				t.Fatal("formatting changed range identity or order")
			}
		}
		if !testutil.Must(set.EqualsTo(original)) {
			t.Fatal("formatting changed set identity")
		}
	}
}

func TestIntegerRangeSetString(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	interval := func(a, b int64) *ranges.Range {
		return testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, a, true), testutil.IntEndpoint(typ, b, true))
	}
	first, second, point := interval(10, 20), interval(30, 40), interval(50, 50)
	checkRangeSetString(t, typ, nil, false, "{}")
	checkRangeSetString(t, typ, []*ranges.Range{first}, false, "[10,20]")
	checkRangeSetString(t, typ, []*ranges.Range{point}, false, "{50}")
	checkRangeSetString(t, typ, []*ranges.Range{point, second, first, second}, false, "[10,20] ∪ [30,40] ∪ {50}")
	checkRangeSetString(t, typ, []*ranges.Range{interval(10, 15), interval(16, 20)}, false, "[10,20]")
	checkRangeSetString(t, typ, []*ranges.Range{testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})}, false, "ALL")
}

func TestFloatRangeSetString(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		ep := func(v float64, inclusive bool) ranges.RangeEndpoint {
			return testutil.Must(ranges.NewRangeEndpoint(testutil.Must(testbedvalues.NewFloat(typ, v)), inclusive))
		}
		full := testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})
		first := testutil.RequiredRange(t, typ, ep(10, true), ep(20, false))
		second := testutil.RequiredRange(t, typ, ep(30, false), ep(40, true))
		point := testutil.RequiredRange(t, typ, ep(0, true), ep(0, true))
		finiteOnly := testutil.RequiredRange(t, typ, ep(math.Inf(-1), false), ep(math.Inf(1), false))
		checkRangeSetString(t, typ, nil, false, "{}")
		checkRangeSetString(t, typ, nil, true, "{NaN}")
		checkRangeSetString(t, typ, []*ranges.Range{full}, false, "(-∞,+∞)")
		checkRangeSetString(t, typ, []*ranges.Range{full}, true, "ALL")
		checkRangeSetString(t, typ, []*ranges.Range{point}, true, "{0} ∪ {NaN}")
		checkRangeSetString(t, typ, []*ranges.Range{first}, true, "[10,20) ∪ {NaN}")
		checkRangeSetString(t, typ, []*ranges.Range{second, first}, false, "[10,20) ∪ (30,40]")
		checkRangeSetString(t, typ, []*ranges.Range{second, first}, true, "[10,20) ∪ (30,40] ∪ {NaN}")
		checkRangeSetString(t, typ, []*ranges.Range{finiteOnly}, true, "(-Inf,+Inf) ∪ {NaN}")
	}
}

func TestVarCharRangeSetString(t *testing.T) {
	typ := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	ep := func(v string) ranges.RangeEndpoint {
		return testutil.Must(ranges.NewRangeEndpoint(testutil.Must(testbedvalues.NewVarChar(typ, v)), true))
	}
	first := testutil.RequiredRange(t, typ, ep("a"), ep("b"))
	point := testutil.RequiredRange(t, typ, ep("z\n\xff"), ep("z\n\xff"))
	checkRangeSetString(t, typ, []*ranges.Range{point, first}, false, `["a","b"] ∪ {"z\n\xff"}`)
	checkRangeSetString(t, typ, nil, false, "{}")
	checkRangeSetString(t, typ, []*ranges.Range{testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})}, false, "ALL")
}

func TestRangeSetStringPreconditions(t *testing.T) {
	for _, set := range []*sets.SortedRangeSet{nil, {}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected assertion for unconstructed set")
				}
			}()
			_ = set.String()
		}()
	}
}
