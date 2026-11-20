package sorted_range_set_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Reuse the independent membership models of the union/intersection fixtures.
func checkRangeAlgebra(t *testing.T, a, b *sets.SortedRangeSet, probes []valueinterfaces.Value, inA, inB []bool, completeUniverse bool) {
	t.Helper()
	notA, notB := testutil.Must(a.Complement()), testutil.Must(b.Complement())
	difference, reverse := testutil.Must(a.Difference(b)), testutil.Must(b.Difference(a))
	candidates := []value_set.ValueSet{notA, notB, difference, reverse}
	counts := make([]int, len(candidates))
	disjoint := true
	for i, v := range probes {
		expected := []bool{!inA[i], !inB[i], inA[i] && !inB[i], inB[i] && !inA[i]}
		for j, result := range candidates {
			if testutil.Must(result.Contains(v)) != expected[j] {
				t.Fatalf("algebra result %d probe %d differs", j, i)
			}
			if expected[j] {
				counts[j]++
			}
		}
		if inA[i] && inB[i] {
			disjoint = false
		}
	}
	for i, candidate := range candidates {
		result := candidate.(*sets.SortedRangeSet)
		if completeUniverse && (testutil.Must(result.IsEmpty()) != (counts[i] == 0) || testutil.Must(result.IsUniversal()) != (counts[i] == len(probes))) {
			t.Fatal("algebra universe query")
		}
		output := result.Ranges()
		normalized := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, result.Type(), output, result.IncludesNaN()))
		if !slices.Equal(output, normalized.Ranges()) {
			t.Fatal("algebra output not normalized")
		}
		if len(output) > 0 {
			output[0] = nil
			if result.Ranges()[0] == nil {
				t.Fatal("algebra slice ownership")
			}
		}
	}
	if completeUniverse {
		if testutil.Must(a.IsSubsetOf(b)) != (counts[2] == 0) || testutil.Must(b.IsSubsetOf(a)) != (counts[3] == 0) {
			t.Fatal("subset differs from enumerated model")
		}
		if testutil.Must(a.IsDisjointFrom(b)) != disjoint || testutil.Must(b.IsDisjointFrom(a)) != disjoint {
			t.Fatal("disjointness differs from enumerated model")
		}
		if testutil.Must(a.EqualsTo(b)) != (counts[2] == 0 && counts[3] == 0) || testutil.Must(b.EqualsTo(a)) != (counts[2] == 0 && counts[3] == 0) {
			t.Fatal("equality differs from enumerated model")
		}
	}
	if !testutil.Must(testutil.Must(notA.Complement()).EqualsTo(a)) {
		t.Fatal("complement involution")
	}
	if !testutil.Must(testutil.Must(a.Union(notA)).IsUniversal()) || !testutil.Must(testutil.Must(a.Intersect(notA)).IsEmpty()) {
		t.Fatal("complement partition")
	}
	if !testutil.Must(testutil.Must(testutil.Must(a.Union(b)).Complement()).EqualsTo(testutil.Must(notA.Intersect(notB)))) {
		t.Fatal("union De Morgan")
	}
	if !testutil.Must(testutil.Must(testutil.Must(a.Intersect(b)).Complement()).EqualsTo(testutil.Must(notA.Union(notB)))) {
		t.Fatal("intersection De Morgan")
	}
	if !testutil.Must(testutil.Must(difference.Union(testutil.Must(a.Intersect(b)))).EqualsTo(a)) {
		t.Fatal("difference partition")
	}
	if !testutil.Must(difference.IsDisjointFrom(b)) || !testutil.Must(difference.IsSubsetOf(a)) {
		t.Fatal("difference relation")
	}
}

func TestRangeEqualityWithDifferentEndpointSpellings(t *testing.T) {
	text := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		float := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		next := math.Nextafter(1, 2)
		if width == types.Float32 {
			next = float64(math.Nextafter32(1, 2))
		}
		for _, tc := range []struct {
			typ        types.Type
			start, end ranges.RangeEndpoint
		}{
			{text, testutil.TextEndpoint(text, "a", true), testutil.TextEndpoint(text, "a\x00", false)},
			{float, testutil.FloatEndpoint(float, 1, true), testutil.FloatEndpoint(float, next, false)},
		} {
			span := testutil.RequiredRange(t, tc.typ, tc.start, tc.end)
			singleton := testutil.RequiredRange(t, tc.typ, tc.start, tc.start)
			a := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, tc.typ, []*ranges.Range{span}, false))
			b := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, tc.typ, []*ranges.Range{singleton}, false))
			if !testutil.Must(a.EqualsTo(b)) || !testutil.Must(b.EqualsTo(a)) || !testutil.Must(a.IsSubsetOf(b)) || !testutil.Must(b.IsSubsetOf(a)) {
				t.Fatal("equivalent cuts treated as unequal")
			}
			if !testutil.Must(testutil.Must(a.Complement()).EqualsTo(testutil.Must(b.Complement()))) {
				t.Fatal("equivalent complements treated as unequal")
			}
			if !testutil.Must(testutil.Must(a.Difference(b)).IsEmpty()) || !testutil.Must(testutil.Must(b.Difference(a)).IsEmpty()) {
				t.Fatal("equivalent difference nonempty")
			}
		}
		nan := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, float, nil, true))
		empty := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, float, nil, false))
		all := testutil.Must(empty.Complement())
		ordered := testutil.Must(nan.Complement())
		if testutil.Must(nan.EqualsTo(empty)) || testutil.Must(all.EqualsTo(ordered)) || testutil.Must(nan.IsSubsetOf(ordered)) || !testutil.Must(nan.IsSubsetOf(all)) || !testutil.Must(nan.IsDisjointFrom(ordered)) {
			t.Fatal("relation ignored NaN")
		}
		if !testutil.Must(testutil.Must(all.Difference(ordered)).EqualsTo(nan)) || !testutil.Must(testutil.Must(ordered.Difference(all)).IsEmpty()) {
			t.Fatal("NaN difference")
		}
	}
}

func TestRangeComplementExtremaAndSplitting(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	points := []int64{math.MinInt64, 0, math.MaxInt64}
	var input []*ranges.Range
	for _, n := range points {
		e := testutil.IntEndpoint(typ, n, true)
		input = append(input, testutil.RequiredRange(t, typ, e, e))
	}
	holes := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, input, false))
	remaining := testutil.Must(holes.Complement()).(*sets.SortedRangeSet)
	if len(remaining.Ranges()) != 2 {
		t.Fatal("wrong split count at Int64 extrema")
	}
	for _, n := range []int64{math.MinInt64, math.MinInt64 + 1, -1, 0, 1, math.MaxInt64 - 1, math.MaxInt64} {
		if testutil.Must(remaining.Contains(testutil.Must(testbedvalues.NewInteger(typ, n)))) == slices.Contains(points, n) {
			t.Fatal("Int64 boundary membership")
		}
	}
	huge := uint64(math.MaxUint64)
	for _, bound := range []*uint64{nil, &huge} {
		typ := testutil.Must(types.NewVarCharType(bound, foundation.NonNull))
		e := testutil.TextEndpoint(typ, "", true)
		singleton := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{testutil.RequiredRange(t, typ, e, e)}, false))
		rest := testutil.Must(singleton.Complement())
		if testutil.Must(rest.Contains(testutil.Must(testbedvalues.NewVarChar(typ, "")))) || !testutil.Must(rest.Contains(testutil.Must(testbedvalues.NewVarChar(typ, "\x00")))) || !testutil.Must(rest.Contains(testutil.Must(testbedvalues.NewVarChar(typ, "\xff")))) {
			t.Fatal("VARCHAR universe-relative complement")
		}
		if !testutil.Must(testutil.Must(rest.Complement()).EqualsTo(singleton)) {
			t.Fatal("huge bound involution")
		}
	}
}

// A provider with an unknown representation: only Type and diagnostic String are safe to query.
type unknownRangeOperand struct {
	value_set.ValueSet
	typ types.Type
}

func (s *unknownRangeOperand) Type() types.Type { return s.typ }

func (*unknownRangeOperand) String() string { return "<unknownRangeOperand>" }

func TestRangeAlgebraUnsupportedOperandsAndPreconditions(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	otherType := testutil.Must(types.NewIntType(types.Int16, foundation.NonNull))
	empty := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, false))
	full := testutil.Must(empty.Complement())
	for _, left := range []value_set.ValueSet{empty, full} {
		for _, tc := range []struct {
			right value_set.ValueSet
			want  error
		}{
			{&unknownRangeOperand{typ: typ}, value_set.ErrUnsupported},
			{&unknownRangeOperand{typ: otherType}, value_set.ErrIncompatibleTypes},
			{testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, otherType, nil, false)), value_set.ErrIncompatibleTypes},
		} {
			for _, op := range []func(value_set.ValueSet) (value_set.ValueSet, error){left.Union, left.Intersect, left.Difference} {
				result, err := op(tc.right)
				if result != nil || !errors.Is(err, tc.want) {
					t.Fatalf("unexpected operation result %v %v", result, err)
				}
			}
			for _, query := range []func(value_set.ValueSet) (bool, error){left.IsSubsetOf, left.IsDisjointFrom, left.EqualsTo} {
				result, err := query(tc.right)
				if result || !errors.Is(err, tc.want) {
					t.Fatalf("unexpected query result %v %v", result, err)
				}
			}
		}
	}
	for _, bad := range []*sets.SortedRangeSet{nil, {}} {
		for _, call := range []func(){
			func() { _, _ = bad.Complement() }, func() { _, _ = bad.Difference(empty) }, func() { _, _ = empty.Difference(bad) },
			func() { _, _ = bad.IsSubsetOf(empty) }, func() { _, _ = empty.IsSubsetOf(bad) },
			func() { _, _ = bad.IsDisjointFrom(empty) }, func() { _, _ = empty.IsDisjointFrom(bad) },
			func() { _, _ = bad.EqualsTo(empty) }, func() { _, _ = empty.EqualsTo(bad) },
		} {
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
	for _, call := range []func(){func() { _, _ = empty.Difference(nil) }, func() { _, _ = empty.EqualsTo(nil) }, func() { _, _ = empty.Union(nil) }, func() { _, _ = empty.Intersect(nil) }, func() { _, _ = empty.IsSubsetOf(nil) }, func() { _, _ = empty.IsDisjointFrom(nil) }} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("missing nil operand assertion")
				}
			}()
			call()
		}()
	}
}
