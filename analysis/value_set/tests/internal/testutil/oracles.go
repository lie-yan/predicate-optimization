package testutil

import (
	"math"
	"strings"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// AcceptsText checks VARCHAR interval membership independently of range operations.
func AcceptsText(lower, upper ranges.RangeEndpoint, text string) bool {
	if value, present := lower.Value().Get(); present {
		order := strings.Compare(text, value.(*testbedvalues.VarCharValue).VarChar())
		if order < 0 || (order == 0 && !lower.IsInclusive()) {
			return false
		}
	}
	if value, present := upper.Value().Get(); present {
		order := strings.Compare(text, value.(*testbedvalues.VarCharValue).VarChar())
		if order > 0 || (order == 0 && !upper.IsInclusive()) {
			return false
		}
	}
	return true
}

// AcceptsFloat checks native float interval membership independently of range operations.
func AcceptsFloat(lower, upper ranges.RangeEndpoint, n float64) bool {
	if math.IsNaN(n) {
		return false
	}
	if v, has := lower.Value().Get(); has {
		a := v.(*testbedvalues.FloatValue).Float()
		if n < a || (n == a && !lower.IsInclusive()) {
			return false
		}
	}
	if v, has := upper.Value().Get(); has {
		b := v.(*testbedvalues.FloatValue).Float()
		if n > b || (n == b && !upper.IsInclusive()) {
			return false
		}
	}
	return true
}

// EnumeratedSet pairs a set with independently derived membership over a test universe.
type EnumeratedSet struct {
	Set     value_set.ValueSet
	Members []bool
}

// Enumerate every subset of chosen entries and both polarities; independently
// derive membership across the entire supplied universe using bit masks.
func EnumerateSets(typ types.Type, universe []valueinterfaces.Value, chosen []int) []EnumeratedSet {
	var result []EnumeratedSet
	for mask := 0; mask < 1<<len(chosen); mask++ {
		entries := []valueinterfaces.Value{}
		marked := make([]bool, len(universe))
		for bit, index := range chosen {
			if mask&(1<<bit) != 0 {
				entries = append(entries, universe[index])
				marked[index] = true
			}
		}
		for _, isInclusion := range []bool{true, false} {
			var set value_set.ValueSet
			if isInclusion {
				set = Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, entries))
			} else {
				set = Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, entries))
			}
			membership := make([]bool, len(universe))
			for i, present := range marked {
				membership[i] = present == isInclusion
			}
			result = append(result, EnumeratedSet{set, membership})
		}
	}
	return result
}

// Inspect the public enumeration independently of the storage comparator.
func AssertOrderedFiniteStorage(t *testing.T, set value_set.ValueSet) {
	t.Helper()
	if !set.Type().IsOrderable() {
		return
	}
	finiteSet, ok := set.(*finite.EquatableValueSet)
	if !ok {
		return
	}
	entries := finiteSet.Values()
	for i := 1; i < len(entries); i++ {
		ascending := false
		switch a := entries[i-1].(type) {
		case *testbedvalues.IntegerValue:
			ascending = a.Integer() < entries[i].(*testbedvalues.IntegerValue).Integer()
		case *testbedvalues.VarCharValue:
			ascending = a.VarChar() < entries[i].(*testbedvalues.VarCharValue).VarChar()
		case *testbedvalues.FloatValue:
			x, y := a.Float(), entries[i].(*testbedvalues.FloatValue).Float()
			ascending = !math.IsNaN(x) && (math.IsNaN(y) || x < y)
		}
		if !ascending {
			t.Fatalf("entries %d and %d are not strictly ordered", i-1, i)
		}
	}
}

// VerifyBinaryAlgebra checks set operations against independently enumerated membership.
func VerifyBinaryAlgebra(t *testing.T, universe []valueinterfaces.Value, sets []EnumeratedSet) {
	t.Helper()
	for i, a := range sets {
		for j, b := range sets {
			union := Must(a.Set.Union(b.Set))
			intersection := Must(a.Set.Intersect(b.Set))
			difference := Must(a.Set.Difference(b.Set))
			for _, result := range []value_set.ValueSet{union, intersection, difference} {
				AssertOrderedFiniteStorage(t, result)
			}
			subset, disjoint, equal := true, true, true
			for k, value := range universe {
				av, bv := a.Members[k], b.Members[k]
				if Must(union.Contains(value)) != (av || bv) || Must(intersection.Contains(value)) != (av && bv) || Must(difference.Contains(value)) != (av && !bv) {
					t.Fatalf("wrong algebra for sets %d,%d at value %d", i, j, k)
				}
				subset = subset && (!av || bv)
				disjoint = disjoint && (!av || !bv)
				equal = equal && (av == bv)
			}
			if Must(a.Set.IsSubsetOf(b.Set)) != subset || Must(a.Set.IsDisjointFrom(b.Set)) != disjoint || Must(a.Set.EqualsTo(b.Set)) != equal {
				t.Fatalf("wrong relation for sets %d,%d", i, j)
			}
			if !Must(union.EqualsTo(Must(b.Set.Union(a.Set)))) || !Must(intersection.EqualsTo(Must(b.Set.Intersect(a.Set)))) {
				t.Fatal("commutativity")
			}
			if !Must(Must(union.Complement()).EqualsTo(Must(Must(a.Set.Complement()).Intersect(Must(b.Set.Complement()))))) {
				t.Fatal("De Morgan")
			}
			if !Must(Must(intersection.Complement()).EqualsTo(Must(Must(a.Set.Complement()).Union(Must(b.Set.Complement()))))) {
				t.Fatal("De Morgan")
			}
			if !Must(difference.EqualsTo(Must(a.Set.Intersect(Must(b.Set.Complement()))))) {
				t.Fatal("difference identity")
			}
		}
	}
}
