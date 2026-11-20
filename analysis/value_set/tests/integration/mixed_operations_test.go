package integration_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func compareSpecializedMixed(t *testing.T, f *finite.EquatableValueSet, r *sets.SortedRangeSet) {
	t.Helper()
	beforeF, beforeR := f.Values(), r.Ranges()
	converted := testutil.Must(f.ToSortedRangeSet())
	for _, tc := range []struct {
		got, want value_set.ValueSet
		finite    bool
	}{
		{testutil.Must(f.Union(r)), testutil.Must(converted.Union(r)), false},
		{testutil.Must(r.Union(f)), testutil.Must(r.Union(converted)), false},
		{testutil.Must(f.Intersect(r)), testutil.Must(converted.Intersect(r)), f.IsInclusion()},
		{testutil.Must(r.Intersect(f)), testutil.Must(r.Intersect(converted)), f.IsInclusion()},
		{testutil.Must(f.Difference(r)), testutil.Must(converted.Difference(r)), f.IsInclusion()},
		{testutil.Must(r.Difference(f)), testutil.Must(r.Difference(converted)), !f.IsInclusion()},
	} {
		// Compare as ranges so the oracle uses only range/range algorithms.
		var actual *sets.SortedRangeSet
		if tc.finite {
			result, ok := tc.got.(*finite.EquatableValueSet)
			if !ok || !result.IsInclusion() {
				t.Fatalf("expected included finite result, got %T", tc.got)
			}
			actual = testutil.Must(result.ToSortedRangeSet())
		} else {
			var ok bool
			actual, ok = tc.got.(*sets.SortedRangeSet)
			if !ok {
				t.Fatalf("expected range result, got %T", tc.got)
			}
		}
		if !testutil.Must(actual.EqualsTo(tc.want)) {
			t.Fatal("specialized result differs from conversion fallback")
		}
	}
	if !slices.Equal(beforeF, f.Values()) || !slices.Equal(beforeR, r.Ranges()) {
		t.Fatal("mixed operation mutated input")
	}
}

func TestSpecializedMixedAgainstFallback(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	universe := make([]valueinterfaces.Value, 256)
	for i := range universe {
		universe[i] = testutil.Must(testbedvalues.NewInteger(typ, int64(i)-128))
	}
	var regions []*sets.SortedRangeSet
	regions = append(regions, testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, false)))
	for _, bounds := range [][2]int64{{-128, 127}, {-128, 0}, {0, 127}, {-5, 5}, {1, 1}, {-128, -128}, {127, 127}} {
		interval := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, bounds[0], true), testutil.IntEndpoint(typ, bounds[1], true))
		r := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{interval}, false))
		regions = append(regions, r, testutil.Must(r.Complement()).(*sets.SortedRangeSet))
	}
	for _, input := range testutil.EnumerateSets(typ, universe, []int{0, 123, 128, 129, 255}) {
		for _, r := range regions {
			compareSpecializedMixed(t, input.Set.(*finite.EquatableValueSet), r)
		}
	}
}

func TestSpecializedMixedExamplesAndOrder(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	entries := []valueinterfaces.Value{testutil.Must(testbedvalues.NewInteger(typ, 12)), testutil.Must(testbedvalues.NewInteger(typ, 5)), testutil.Must(testbedvalues.NewInteger(typ, 2))}
	f := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, entries))
	interval := testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, testutil.IntEndpoint(typ, 10, false))
	r := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{interval}, false))
	compareSpecializedMixed(t, f, r)
	for _, result := range []value_set.ValueSet{testutil.Must(f.Intersect(r)), testutil.Must(r.Intersect(f))} {
		got := result.(*finite.EquatableValueSet).Values()
		if !slices.Equal(got, []valueinterfaces.Value{entries[2], entries[1]}) {
			t.Fatal("filter did not preserve sorted order and shared Values")
		}
	}
	hole := testutil.Must(testbedvalues.NewInteger(typ, 3))
	excluded := testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{hole}))
	split := testutil.Must(r.Intersect(excluded)).(*sets.SortedRangeSet)
	if len(split.Ranges()) != 2 || testutil.Must(split.Contains(hole)) {
		t.Fatal("excluded point not removed")
	}
	ten := testutil.Must(testbedvalues.NewInteger(typ, 10))
	extended := testutil.Must(r.WithPoints([]valueinterfaces.Value{ten, ten}))
	if len(extended.Ranges()) != 1 || !testutil.Must(extended.Contains(ten)) {
		t.Fatal("boundary insertion did not merge")
	}
	outside := testutil.Must(f.Difference(r)).(*finite.EquatableValueSet)
	if !slices.Equal(outside.Values(), entries[:1]) {
		t.Fatal("finite difference filtering")
	}
}

func TestMixedPointBoundariesAndFloats(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		tiny, next := math.SmallestNonzeroFloat64, math.Nextafter(1, 2)
		if width == types.Float32 {
			tiny = float64(math.SmallestNonzeroFloat32)
			next = float64(math.Nextafter32(1, 2))
		}
		var points []valueinterfaces.Value
		for _, n := range []float64{math.NaN(), math.Inf(1), next, 1, tiny, 0, math.Copysign(0, -1), -tiny, math.Inf(-1), math.NaN()} {
			points = append(points, testutil.Must(testbedvalues.NewFloat(typ, n)))
		}
		for _, spec := range [][2]ranges.RangeEndpoint{
			{{}, {}},
			{testutil.FloatEndpoint(typ, 0, false), testutil.FloatEndpoint(typ, 1, false)},
			{testutil.FloatEndpoint(typ, 0, true), testutil.FloatEndpoint(typ, 0, true)},
			{testutil.FloatEndpoint(typ, 1, true), testutil.FloatEndpoint(typ, next, false)},
		} {
			interval := testutil.RequiredRange(t, typ, spec[0], spec[1])
			for _, nan := range []bool{false, true} {
				r := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{interval}, nan))
				compareSpecializedMixed(t, testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, points)), r)
				compareSpecializedMixed(t, testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, points)), r)
				comparePointOperations(t, r, points)
			}
		}
	}
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ := testutil.Must(types.NewIntType(width, foundation.NonNull))
		max := int64(math.MaxInt64)
		if width != types.Int64 {
			max = int64(1)<<(width-1) - 1
		}
		min := -max - 1
		var points []valueinterfaces.Value
		for _, n := range []int64{max, 0, min, 1, -1, min, max} {
			points = append(points, testutil.Must(testbedvalues.NewInteger(typ, n)))
		}
		r := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})}, false))
		comparePointOperations(t, r, points)
		comparePointOperations(t, testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, false)), points)
	}
	zero, one, huge := uint64(0), uint64(1), uint64(math.MaxUint64)
	for _, bound := range []*uint64{&zero, &one, &huge, nil} {
		typ := testutil.Must(types.NewVarCharType(bound, foundation.NonNull))
		texts := []string{""}
		if bound == nil || *bound > 0 {
			texts = append(texts, "\xff", "a", "\x00", "a")
		}
		var points []valueinterfaces.Value
		for _, s := range texts {
			points = append(points, testutil.Must(testbedvalues.NewVarChar(typ, s)))
		}
		r := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})}, false))
		comparePointOperations(t, r, points)
		comparePointOperations(t, testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, false)), points)
		compareSpecializedMixed(t, testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, points)), r)
	}
}

func comparePointOperations(t *testing.T, r *sets.SortedRangeSet, points []valueinterfaces.Value) {
	t.Helper()
	before, entries := r.Ranges(), slices.Clone(points)
	f := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, r.Type(), points))
	converted := testutil.Must(f.ToSortedRangeSet())
	added, removed := testutil.Must(r.WithPoints(points)), testutil.Must(r.WithoutPoints(points))
	if !testutil.Must(added.EqualsTo(testutil.Must(r.Union(converted)))) || !testutil.Must(removed.EqualsTo(testutil.Must(r.Difference(converted)))) {
		t.Fatal("point operation differs from range fallback")
	}
	for _, result := range []*sets.SortedRangeSet{added, removed} {
		out := result.Ranges()
		normalized := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, result.Type(), out, result.IncludesNaN()))
		if !slices.Equal(out, normalized.Ranges()) {
			t.Fatal("point result not normalized")
		}
		if len(out) > 0 {
			out[0] = nil
			if result.Ranges()[0] == nil {
				t.Fatal("result storage escaped")
			}
		}
	}
	if !slices.Equal(points, entries) || !slices.Equal(before, r.Ranges()) {
		t.Fatal("point operation mutated source")
	}
}

func TestPointValidation(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	all := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})}, false))
	empty := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, false))
	good := testutil.Must(testbedvalues.NewInteger(typ, 1))
	nullable := testutil.Must(good.WithNullability(foundation.MayNull))
	comparePointOperations(t, all, []valueinterfaces.Value{nullable})
	for _, r := range []*sets.SortedRangeSet{all, empty} {
		comparePointOperations(t, r, nil)
		for _, tc := range []struct {
			bad  valueinterfaces.Value
			want error
		}{
			{testutil.Must(testbedvalues.NewNull(nullable.Type())), value_set.ErrInvalidInput},
			{testutil.Must(testbedvalues.NewInteger(testutil.Must(types.NewIntType(types.Int16, foundation.NonNull)), 1)), value_set.ErrIncompatibleTypes},
			{testutil.Must(testbedvalues.NewFloat(testutil.Must(types.NewFloatType(types.Float32, foundation.NonNull)), math.NaN())), value_set.ErrIncompatibleTypes},
		} {
			for _, op := range []func([]valueinterfaces.Value) (*sets.SortedRangeSet, error){r.WithPoints, r.WithoutPoints} {
				result, err := op([]valueinterfaces.Value{good, tc.bad})
				if result != nil || !errors.Is(err, tc.want) {
					t.Fatal("shortcut masked invalid point", err)
				}
			}
		}
	}
	for _, call := range []func(){
		func() { _, _ = all.WithPoints([]valueinterfaces.Value{nil}) }, func() { _, _ = all.WithoutPoints([]valueinterfaces.Value{(*testbedvalues.IntegerValue)(nil)}) },
		func() { var r *sets.SortedRangeSet; _, _ = r.WithPoints(nil) }, func() { _, _ = (&sets.SortedRangeSet{}).WithoutPoints(nil) },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("missing assertion")
				}
			}()
			call()
		}()
	}
}
