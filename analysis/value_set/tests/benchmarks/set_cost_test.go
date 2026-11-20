package benchmarks_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

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

// Global sinks retain results without including enumeration in the measurement.
var (
	costSetResult  value_set.ValueSet
	costBoolResult bool
)

func benchmarkCostSet(b *testing.B, operation func() value_set.ValueSet) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		costSetResult = operation()
	}
}

func benchmarkCostQuery(b *testing.B, operation func() bool) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		costBoolResult = operation()
	}
}

func benchmarkCostRange(typ types.Type, lower, upper ranges.RangeEndpoint) *ranges.Range {
	r, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, upper)).Get()
	if !present {
		panic("benchmark requires nonempty interval")
	}
	return r
}

// Fixtures are outside timed loops. Sizes grow by four so linear and quadratic
// behavior are distinguishable. The odd-multiplier permutation is reproducible
// and visits every index at these power-of-two sizes, avoiding sorted-only inputs.
func BenchmarkSetCosts(b *testing.B) {
	for _, n := range []int{64, 256, 1024} {
		typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
		leftValues, rightValues := make([]valueinterfaces.Value, n), make([]valueinterfaces.Value, n)
		leftRanges, rightRanges := make([]*ranges.Range, n), make([]*ranges.Range, n)
		insert, remove := make([]valueinterfaces.Value, n), make([]valueinterfaces.Value, n)
		for i := 0; i < n; i++ {
			index := (i * 37) % n
			x := int64(index) * 8
			leftValues[i] = testutil.Must(testbedvalues.NewInteger(typ, x))
			rightValues[i] = testutil.Must(testbedvalues.NewInteger(typ, x+6))
			leftRanges[i] = benchmarkCostRange(typ, testutil.IntEndpoint(typ, x, true), testutil.IntEndpoint(typ, x+3, true))
			rightRanges[i] = benchmarkCostRange(typ, testutil.IntEndpoint(typ, x+2, true), testutil.IntEndpoint(typ, x+5, true))
			insert[i] = testutil.Must(testbedvalues.NewInteger(typ, x+4))
			remove[i] = testutil.Must(testbedvalues.NewInteger(typ, x+1))
		}
		a, c := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, leftValues)), testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, rightValues))
		r, s := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, leftRanges, false)), testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, rightRanges, false))
		miss := testutil.Must(testbedvalues.NewInteger(typ, -1))
		for _, tc := range []struct {
			name      string
			operation func() value_set.ValueSet
		}{
			{"FiniteBuild", func() value_set.ValueSet {
				return testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, leftValues))
			}},
			{"FiniteUnionDisjoint", func() value_set.ValueSet { return testutil.Must(a.Union(c)) }},
			{"FiniteIntersectDisjoint", func() value_set.ValueSet { return testutil.Must(a.Intersect(c)) }},
			{"FiniteDifferenceDisjoint", func() value_set.ValueSet { return testutil.Must(a.Difference(c)) }},
			{"FiniteComplement", func() value_set.ValueSet { return testutil.Must(a.Complement()) }},
			{"RangeBuildPermuted", func() value_set.ValueSet {
				return testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, leftRanges, false))
			}},
			{"RangeUnion", func() value_set.ValueSet { return testutil.Must(r.Union(s)) }},
			{"RangeIntersect", func() value_set.ValueSet { return testutil.Must(r.Intersect(s)) }},
			{"RangeDifference", func() value_set.ValueSet { return testutil.Must(r.Difference(s)) }},
			{"RangeComplement", func() value_set.ValueSet { return testutil.Must(r.Complement()) }},
			{"FiniteToRange", func() value_set.ValueSet { return testutil.Must(a.ToSortedRangeSet()) }},
			{"MixedFilter", func() value_set.ValueSet { return testutil.Must(a.Intersect(r)) }},
			{"InsertPoints", func() value_set.ValueSet { return testutil.Must(r.WithPoints(insert)) }},
			{"RemovePoints", func() value_set.ValueSet { return testutil.Must(r.WithoutPoints(remove)) }},
		} {
			b.Run(fmt.Sprintf("%s/n=%d", tc.name, n), func(b *testing.B) { benchmarkCostSet(b, tc.operation) })
		}
		b.Run(fmt.Sprintf("FiniteContainsMiss/n=%d", n), func(b *testing.B) { benchmarkCostQuery(b, func() bool { return testutil.Must(a.Contains(miss)) }) })
		b.Run(fmt.Sprintf("RangeContainsMiss/n=%d", n), func(b *testing.B) { benchmarkCostQuery(b, func() bool { return testutil.Must(r.Contains(miss)) }) })
		b.Run(fmt.Sprintf("RangeEqualsSelf/n=%d", n), func(b *testing.B) { benchmarkCostQuery(b, func() bool { return testutil.Must(r.EqualsTo(r)) }) })
	}
}

// All comparisons share a long prefix, exposing actual payload comparison work.
// Values have equal length so equality cannot reject them using length alone.
func BenchmarkSetPayloadCosts(b *testing.B) {
	typ := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	for _, length := range []int{0, 64, 1024, 16384} {
		prefix := strings.Repeat("x", length)
		entries := make([]valueinterfaces.Value, 128)
		for i := range entries {
			entries[i] = testutil.Must(testbedvalues.NewVarChar(typ, prefix+fmt.Sprintf("%04x", i)))
		}
		f := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, entries))
		r := testutil.Must(f.ToSortedRangeSet())
		probe := testutil.Must(testbedvalues.NewVarChar(typ, prefix+"zzzz"))
		b.Run(fmt.Sprintf("FiniteContainsMiss/prefix=%d", length), func(b *testing.B) { benchmarkCostQuery(b, func() bool { return testutil.Must(f.Contains(probe)) }) })
		b.Run(fmt.Sprintf("RangeContainsMiss/prefix=%d", length), func(b *testing.B) { benchmarkCostQuery(b, func() bool { return testutil.Must(r.Contains(probe)) }) })
	}
}

// The same tiny payloads under very different declared bounds exercise coverage
// early-exit and range-boundary logic without any universe-sized allocation.
func BenchmarkSetDeclaredBoundCosts(b *testing.B) {
	for _, bound := range []uint64{8, math.MaxUint64} {
		typ := testutil.Must(types.NewVarCharType(&bound, foundation.NonNull))
		f := testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(typ, ""))}))
		r := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{benchmarkCostRange(typ, testutil.TextEndpoint(typ, "a", true), testutil.TextEndpoint(typ, "b", true))}, false))
		points := []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(typ, "a\x00"))}
		b.Run(fmt.Sprintf("Coverage/maxBytes=%d", bound), func(b *testing.B) { benchmarkCostQuery(b, func() bool { return testutil.Must(f.IsEmpty()) }) })
		b.Run(fmt.Sprintf("RemovePoint/maxBytes=%d", bound), func(b *testing.B) {
			benchmarkCostSet(b, func() value_set.ValueSet { return testutil.Must(r.WithoutPoints(points)) })
		})
	}
}
