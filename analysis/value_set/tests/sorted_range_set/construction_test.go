package sorted_range_set_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	ranges "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestSortedRangeSetIntegerNormalization(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.MayNull))
	candidates := [][2]int64{{-128, -128}, {-128, -1}, {0, 127}, {1, 2}, {3, 4}, {4, 7}, {10, 10}, {127, 127}, {-128, 127}}
	for _, a := range candidates {
		for _, b := range candidates {
			first := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, a[0], true), testutil.IntEndpoint(typ, a[1], true))
			second := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, b[0], true), testutil.IntEndpoint(typ, b[1], true))
			set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{second, first, second}, false))
			count := 0
			for n := int64(-128); n <= 127; n++ {
				want := (a[0] <= n && n <= a[1]) || (b[0] <= n && n <= b[1])
				if got := testutil.Must(set.Contains(testutil.Must(testbedvalues.NewInteger(typ, n)))); got != want {
					t.Fatalf("%v union %v at %d: got %v", a, b, n, got)
				}
				if want {
					count++
				}
			}
			if testutil.Must(set.IsEmpty()) || testutil.Must(set.IsUniversal()) != (count == 256) {
				t.Fatal("wrong universe query")
			}
			expectedCount := 2
			if a[0] <= b[1]+1 && b[0] <= a[1]+1 {
				expectedCount = 1
			}
			if len(set.Ranges()) != expectedCount {
				t.Fatalf("%v union %v: got %d ranges", a, b, len(set.Ranges()))
			}
			if set.Type().Nullability() != foundation.NonNull || typ.Nullability() != foundation.MayNull {
				t.Fatal("type normalization changed source")
			}
			gotRanges := set.Ranges()
			if len(gotRanges) == 2 {
				upper, _ := gotRanges[0].Upper().Value().Get()
				lower, _ := gotRanges[1].Lower().Value().Get()
				if upper.(*testbedvalues.IntegerValue).Integer() >= lower.(*testbedvalues.IntegerValue).Integer() {
					t.Fatal("ranges not sorted")
				}
			}
		}
	}
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ := testutil.Must(types.NewIntType(width, foundation.NonNull))
		left := testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, testutil.IntEndpoint(typ, 0, false))
		right := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, 0, true), ranges.RangeEndpoint{})
		set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{right, left}, false))
		if !testutil.Must(set.IsUniversal()) || len(set.Ranges()) != 1 {
			t.Fatal("split integer universe not merged")
		}
	}
}

func TestSortedRangeSetVarCharNormalization(t *testing.T) {
	for _, limit := range []uint64{0, 1} {
		typ := testutil.Must(types.NewVarCharType(&limit, foundation.NonNull))
		universe := []string{""}
		cuts := []ranges.RangeEndpoint{{}, testutil.TextEndpoint(typ, "", true), testutil.TextEndpoint(typ, "", false)}
		if limit == 1 {
			for n := 0; n < 256; n++ {
				universe = append(universe, string([]byte{byte(n)}))
			}
			for _, s := range []string{"\x00", "a", "b", "\xff"} {
				cuts = append(cuts, testutil.TextEndpoint(typ, s, false), testutil.TextEndpoint(typ, s, true))
			}
		}
		// Two complementary sides should fill the universe except when both cuts
		// exclude the same point. This also exercises open cuts retained by VARCHAR.
		for _, cut := range cuts {
			for _, other := range cuts {
				var input []*ranges.Range
				for _, pair := range [][2]ranges.RangeEndpoint{{{}, cut}, {other, {}}} {
					if r, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, pair[0], pair[1])).Get(); present {
						input = append(input, r)
					}
				}
				set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, false))
				count := 0
				for _, s := range universe {
					want := testutil.AcceptsText(ranges.RangeEndpoint{}, cut, s) || testutil.AcceptsText(other, ranges.RangeEndpoint{}, s)
					if testutil.Must(set.Contains(testutil.Must(testbedvalues.NewVarChar(typ, s)))) != want {
						t.Fatalf("VARCHAR(%d) membership differs at %q", limit, s)
					}
					if want {
						count++
					}
				}
				if testutil.Must(set.IsUniversal()) != (count == len(universe)) || testutil.Must(set.IsEmpty()) != (count == 0) {
					t.Fatal("wrong VARCHAR universe query")
				}
			}
		}
	}
	one, two, huge := uint64(1), uint64(2), uint64(math.MaxUint64)
	for _, tc := range []struct {
		bound     *uint64
		low, high string
		count     int
	}{
		{nil, "a", "a\x00", 1},
		{nil, "a", "b", 2},
		{&one, "a", "b", 1},
		{&two, "a\xff", "b", 1},
		{&huge, "a", "b", 2},
	} {
		typ := testutil.Must(types.NewVarCharType(tc.bound, foundation.NonNull))
		low, high := testutil.TextEndpoint(typ, tc.low, true), testutil.TextEndpoint(typ, tc.high, true)
		set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{testutil.RequiredRange(t, typ, high, high), testutil.RequiredRange(t, typ, low, low)}, false))
		if len(set.Ranges()) != tc.count {
			t.Fatalf("%q and %q: wrong adjacency", tc.low, tc.high)
		}
	}
}

func TestSortedRangeSetFloatNormalization(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		next, tiny, max := math.Nextafter(1, 2), math.SmallestNonzeroFloat64, math.MaxFloat64
		if width == types.Float32 {
			next = float64(math.Nextafter32(1, 2))
			tiny = float64(math.SmallestNonzeroFloat32)
			max = float64(math.MaxFloat32)
		}
		for _, tc := range []struct {
			a, b  float64
			count int
		}{
			{1, next, 1},
			{0, math.Copysign(0, -1), 1},
			{-tiny, 0, 1},
			{0, tiny, 1},
			{-tiny, tiny, 2},
			{max, math.Inf(1), 1},
			{math.Inf(-1), -max, 1},
			{1, 2, 2},
		} {
			a, b := testutil.FloatEndpoint(typ, tc.a, true), testutil.FloatEndpoint(typ, tc.b, true)
			set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{testutil.RequiredRange(t, typ, b, b), testutil.RequiredRange(t, typ, a, a)}, false))
			if len(set.Ranges()) != tc.count {
				t.Fatalf("Float%d %g and %g: wrong adjacency", width, tc.a, tc.b)
			}
			for _, n := range []float64{math.Inf(-1), -max, -tiny, math.Copysign(0, -1), 0, tiny, 1, next, 2, max, math.Inf(1), math.NaN()} {
				if testutil.Must(set.Contains(testutil.Must(testbedvalues.NewFloat(typ, n)))) != (n == tc.a || n == tc.b) {
					t.Fatalf("Float%d %g and %g: membership at %g", width, tc.a, tc.b, n)
				}
			}
		}
		for _, li := range []bool{false, true} {
			for _, ri := range []bool{false, true} {
				left := testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, testutil.FloatEndpoint(typ, 1, li))
				right := testutil.RequiredRange(t, typ, testutil.FloatEndpoint(typ, 1, ri), ranges.RangeEndpoint{})
				set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{right, left}, true))
				if testutil.Must(set.Contains(testutil.Must(testbedvalues.NewFloat(typ, 1)))) != (li || ri) || testutil.Must(set.IsUniversal()) != (li || ri) {
					t.Fatal("float touching endpoints")
				}
				wantCount := 2
				if li || ri {
					wantCount = 1
				}
				if len(set.Ranges()) != wantCount {
					t.Fatal("float touching endpoints not normalized")
				}
			}
		}
		ordered := testutil.RequiredRange(t, typ, testutil.FloatEndpoint(typ, math.Inf(-1), true), testutil.FloatEndpoint(typ, math.Inf(1), true))
		for _, input := range [][]*ranges.Range{nil, {ordered}} {
			for _, nan := range []bool{false, true} {
				set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, nan))
				if set.IncludesNaN() != nan || testutil.Must(set.Contains(testutil.Must(testbedvalues.NewFloat(typ, math.NaN())))) != nan {
					t.Fatal("NaN flag lost")
				}
				if testutil.Must(set.IsEmpty()) != (len(input) == 0 && !nan) || testutil.Must(set.IsUniversal()) != (len(input) == 1 && nan) {
					t.Fatal("NaN universe query")
				}
				nullable := testutil.Must(testutil.Must(testbedvalues.NewFloat(typ, 0)).WithNullability(foundation.MayNull))
				if testutil.Must(set.Contains(nullable)) != (len(input) == 1) {
					t.Fatal("nullable concrete float membership")
				}
			}
		}
	}
}

func TestSortedRangeSetOwnershipAndErrors(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	one := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, 1, true), testutil.IntEndpoint(typ, 1, true))
	all := testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})
	input := []*ranges.Range{one}
	set := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, false))
	input[0] = all
	output := set.Ranges()
	if output[0] != one {
		t.Fatal("immutable range not shared")
	}
	output[0] = all
	if testutil.Must(set.IsUniversal()) || testutil.Must(set.Contains(testutil.Must(testbedvalues.NewInteger(typ, 2)))) {
		t.Fatal("slice ownership escaped")
	}
	empty := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, false))
	if !testutil.Must(empty.IsEmpty()) || testutil.Must(empty.IsUniversal()) || testutil.Must(empty.Contains(testutil.Must(testbedvalues.NewInteger(typ, 0)))) || empty.IncludesNaN() {
		t.Fatal("empty queries")
	}
	otherType := testutil.Must(types.NewIntType(types.Int16, foundation.NonNull))
	other := testutil.RequiredRange(t, otherType, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})
	for _, tc := range []struct {
		typ   types.Type
		input []*ranges.Range
		nan   bool
		want  error
	}{
		{typ, []*ranges.Range{all, other}, false, value_set.ErrIncompatibleTypes},
		{typ, nil, true, value_set.ErrInvalidInput},
		{testutil.Must(types.NewVarCharType(nil, foundation.NonNull)), nil, true, value_set.ErrInvalidInput},

		{testutil.Must(types.NewBoolType(foundation.NonNull)), nil, false, value_set.ErrUnsupported},
	} {
		result, err := sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, tc.typ, tc.input, tc.nan)
		if !errors.Is(err, tc.want) || result != nil {
			t.Fatalf("construction: got %v want %v", err, tc.want)
		}
	}
	for _, tc := range []struct {
		value valueinterfaces.Value
		want  error
	}{
		{testutil.Must(testbedvalues.NewInteger(otherType, 1)), value_set.ErrIncompatibleTypes},
		{testutil.Must(testbedvalues.NewNull(testutil.Must(typ.WithNullability(foundation.MayNull)))), value_set.ErrInvalidInput},
	} {
		if _, err := set.Contains(tc.value); !errors.Is(err, tc.want) {
			t.Fatalf("membership: got %v want %v", err, tc.want)
		}
	}
	for _, call := range []func(){
		func() { _, _ = sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, nil, nil, false) },
		func() {
			_, _ = sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, (*types.IntType)(nil), nil, false)
		},
		func() {
			_, _ = sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{nil}, false)
		},
		func() {
			_, _ = sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{{}}, false)
		},
		func() { _, _ = set.Contains(nil) }, func() { _, _ = set.Contains((*testbedvalues.IntegerValue)(nil)) },
		func() { (&sorted_range_set.SortedRangeSet{}).Type() }, func() { var s *sorted_range_set.SortedRangeSet; s.Ranges() },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("missing precondition assertion")
				}
			}()
			call()
		}()
	}
}
