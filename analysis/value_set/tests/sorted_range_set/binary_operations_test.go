package sorted_range_set_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The expected membership arrays come from finite models or raw input cuts,
// independently of the resulting sets' boundary and binary-search algorithms.
func checkRangeBinaryOperations(t *testing.T, a, b *sets.SortedRangeSet, probes []valueinterfaces.Value, inA, inB []bool, completeUniverse bool) {
	t.Helper()
	beforeA, beforeB := a.Ranges(), b.Ranges()
	union, intersection := testutil.Must(a.Union(b)).(*sets.SortedRangeSet), testutil.Must(a.Intersect(b)).(*sets.SortedRangeSet)
	reverseUnion, reverseIntersection := testutil.Must(b.Union(a)).(*sets.SortedRangeSet), testutil.Must(b.Intersect(a)).(*sets.SortedRangeSet)
	for _, tc := range []struct {
		result *sets.SortedRangeSet
		union  bool
	}{
		{union, true}, {reverseUnion, true}, {intersection, false}, {reverseIntersection, false},
	} {
		count := 0
		for i, probe := range probes {
			want := inA[i] && inB[i]
			if tc.union {
				want = inA[i] || inB[i]
			}
			if testutil.Must(tc.result.Contains(probe)) != want {
				t.Fatalf("union=%v probe %d: membership differs", tc.union, i)
			}
			if want {
				count++
			}
		}
		if completeUniverse && (testutil.Must(tc.result.IsEmpty()) != (count == 0) || testutil.Must(tc.result.IsUniversal()) != (count == len(probes))) {
			t.Fatal("incorrect finite universe query")
		}
		expectedNaN := a.IncludesNaN() && b.IncludesNaN()
		if tc.union {
			expectedNaN = a.IncludesNaN() || b.IncludesNaN()
		}
		if tc.result.IncludesNaN() != expectedNaN {
			t.Fatal("incorrect NaN flag")
		}
		if !tc.result.Type().EqualsTo(a.Type()) {
			t.Fatal("result domain changed")
		}
		// Re-normalization must neither merge components nor change their order.
		output := tc.result.Ranges()
		normalized := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, tc.result.Type(), output, expectedNaN))
		if !slices.Equal(output, normalized.Ranges()) {
			t.Fatal("result ranges not normalized")
		}
		if len(output) > 0 {
			output[0] = nil
		}
		if len(tc.result.Ranges()) > 0 && tc.result.Ranges()[0] == nil {
			t.Fatal("result slice escaped")
		}
	}
	checkRangeAlgebra(t, a, b, probes, inA, inB, completeUniverse)
	if !slices.Equal(beforeA, a.Ranges()) || !slices.Equal(beforeB, b.Ranges()) {
		t.Fatal("operation modified source ranges")
	}
}

func TestRangeBinaryOperationsInt8Model(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	points := []int64{-128, -2, -1, 0, 127}
	var inputs []*sets.SortedRangeSet
	var membership [][]bool
	probes := make([]valueinterfaces.Value, 256)
	for i := range probes {
		probes[i] = testutil.Must(testbedvalues.NewInteger(typ, int64(i)-128))
	}
	for mask := 0; mask < 1<<len(points); mask++ {
		var input []*ranges.Range
		expected := make([]bool, len(probes))
		for i, n := range points {
			if mask&(1<<i) == 0 {
				continue
			}
			e := testutil.IntEndpoint(typ, n, true)
			input = append(input, testutil.RequiredRange(t, typ, e, e))
			expected[n+128] = true
		}
		inputs = append(inputs, testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, false)))
		membership = append(membership, expected)
	}
	// Add large intervals, holes and the full universe alongside exhaustive subsets.
	for _, spans := range [][][2]int64{
		{{-128, 127}}, {{-128, -1}}, {{0, 127}}, {{-128, -2}, {0, 127}}, {{-3, 3}},
	} {
		var input []*ranges.Range
		expected := make([]bool, len(probes))
		for _, span := range spans {
			input = append(input, testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, span[0], true), testutil.IntEndpoint(typ, span[1], true)))
			for n := span[0]; n <= span[1]; n++ {
				expected[n+128] = true
			}
		}
		inputs = append(inputs, testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, false)))
		membership = append(membership, expected)
	}
	for i, a := range inputs {
		for j, b := range inputs {
			checkRangeBinaryOperations(t, a, b, probes, membership[i], membership[j], true)
		}
	}
}

type rawRange [2]ranges.RangeEndpoint

func TestRangeBinaryOperationsVarCharCuts(t *testing.T) {
	for _, limit := range []uint64{0, 1} {
		typ := testutil.Must(types.NewVarCharType(&limit, foundation.NonNull))
		texts := []string{""}
		if limit == 1 {
			for n := 0; n < 256; n++ {
				texts = append(texts, string([]byte{byte(n)}))
			}
		}
		probes := make([]valueinterfaces.Value, len(texts))
		for i, s := range texts {
			probes[i] = testutil.Must(testbedvalues.NewVarChar(typ, s))
		}
		specifications := [][]rawRange{nil, {{{}, {}}}}
		selected := []string{""}
		if limit == 1 {
			selected = append(selected, "\x00", "a", "b", "\xff")
		}
		for _, s := range selected {
			for _, inclusive := range []bool{false, true} {
				e := testutil.TextEndpoint(typ, s, inclusive)
				specifications = append(specifications, []rawRange{{{}, e}}, []rawRange{{e, {}}}, []rawRange{{{}, testutil.TextEndpoint(typ, s, false)}, {testutil.TextEndpoint(typ, s, false), {}}})
			}
		}
		var inputs []*sets.SortedRangeSet
		var membership [][]bool
		for _, spec := range specifications {
			var input []*ranges.Range
			expected := make([]bool, len(probes))
			for _, span := range spec {
				if r, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, span[0], span[1])).Get(); present {
					input = append(input, r)
				}
				for i, s := range texts {
					expected[i] = expected[i] || testutil.AcceptsText(span[0], span[1], s)
				}
			}
			inputs = append(inputs, testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, false)))
			membership = append(membership, expected)
		}
		for i, a := range inputs {
			for j, b := range inputs {
				checkRangeBinaryOperations(t, a, b, probes, membership[i], membership[j], true)
			}
		}
	}
}

func TestRangeBinaryOperationsFloatCutsAndNaN(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		tiny, max, next := math.SmallestNonzeroFloat64, math.MaxFloat64, math.Nextafter(1, 2)
		if width == types.Float32 {
			tiny = float64(math.SmallestNonzeroFloat32)
			max = float64(math.MaxFloat32)
			next = float64(math.Nextafter32(1, 2))
		}
		numbers := []float64{math.Inf(-1), -max, -1, -tiny, math.Copysign(0, -1), 0, tiny, 1, next, max, math.Inf(1), math.NaN()}
		probes := make([]valueinterfaces.Value, len(numbers))
		for i, n := range numbers {
			probes[i] = testutil.Must(testbedvalues.NewFloat(typ, n))
		}
		specs := [][]rawRange{nil, {{{}, {}}}}
		for _, n := range []float64{math.Inf(-1), -tiny, 0, 1, next, math.Inf(1)} {
			closed, open := testutil.FloatEndpoint(typ, n, true), testutil.FloatEndpoint(typ, n, false)
			specs = append(specs, []rawRange{{closed, closed}}, []rawRange{{{}, open}}, []rawRange{{open, {}}}, []rawRange{{{}, open}, {open, {}}})
		}
		var inputs []*sets.SortedRangeSet
		var membership [][]bool
		for _, spec := range specs {
			for _, nan := range []bool{false, true} {
				var input []*ranges.Range
				expected := make([]bool, len(probes))
				for _, span := range spec {
					if r, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, span[0], span[1])).Get(); present {
						input = append(input, r)
					}
					for i, n := range numbers {
						expected[i] = expected[i] || testutil.AcceptsFloat(span[0], span[1], n)
					}
				}
				expected[len(expected)-1] = nan
				inputs = append(inputs, testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, nan)))
				membership = append(membership, expected)
			}
		}
		for i, a := range inputs {
			for j, b := range inputs {
				checkRangeBinaryOperations(t, a, b, probes, membership[i], membership[j], false)
			}
		}
		ordered := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{testutil.RequiredRange(t, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})}, false))
		nanOnly := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, true))
		if !testutil.Must(testutil.Must(ordered.Union(nanOnly)).IsUniversal()) || !testutil.Must(testutil.Must(ordered.Intersect(nanOnly)).IsEmpty()) {
			t.Fatal("float universe or empty NaN algebra")
		}
	}
}

func TestRangeBinaryOperationCompatibilityAndAssertions(t *testing.T) {
	i8 := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	i16 := testutil.Must(types.NewIntType(types.Int16, foundation.NonNull))
	f32 := testutil.Must(types.NewFloatType(types.Float32, foundation.NonNull))
	f64 := testutil.Must(types.NewFloatType(types.Float64, foundation.NonNull))
	one := uint64(1)
	bounded := testutil.Must(types.NewVarCharType(&one, foundation.NonNull))
	unbounded := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	for _, pair := range [][2]types.Type{{i8, i16}, {f32, f64}, {bounded, unbounded}, {i8, f32}} {
		a, b := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, pair[0], nil, false)), testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, pair[1], nil, false))
		for _, operands := range [][2]*sets.SortedRangeSet{{a, b}, {b, a}} {
			for _, op := range []func(value_set.ValueSet) (value_set.ValueSet, error){operands[0].Union, operands[0].Intersect, operands[0].Difference} {
				result, err := op(operands[1])
				if result != nil || !errors.Is(err, value_set.ErrIncompatibleTypes) {
					t.Fatal("empty shortcut hid incompatible domain", err)
				}
			}
		}
	}
	a := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, i8, nil, false))
	nullable := testutil.Must(i8.WithNullability(foundation.MayNull))
	b := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, nullable, nil, false))
	if !testutil.Must(testutil.Must(a.Union(b)).IsEmpty()) || !testutil.Must(testutil.Must(a.Intersect(b)).IsEmpty()) {
		t.Fatal("outer nullability mismatch")
	}
	for _, bad := range []*sets.SortedRangeSet{nil, {}} {
		for _, call := range []func(){func() { _, _ = a.Union(bad) }, func() { _, _ = a.Intersect(bad) }, func() { _, _ = bad.Union(a) }, func() { _, _ = bad.Intersect(a) }} {
			func() {
				defer func() {
					if recover() == nil {
						t.Error("missing constructed-set assertion")
					}
				}()
				call()
			}()
		}
	}
}
