package integration_test

import (
	"fmt"
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

// Every ValueSet is usable as a fmt.Stringer without a concrete type assertion.
var _ fmt.Stringer = (value_set.ValueSet)(nil)

func TestValueSetStringInterface(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	point := testutil.Must(testbedvalues.NewInteger(typ, 10))
	interval := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, 10, true), testutil.IntEndpoint(typ, 299, false))
	for _, tc := range []struct {
		set  value_set.ValueSet
		want string
	}{
		{testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil)), "{}"},
		{testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{point})), "{10}"},
		{testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{point})), "ALL \\ {10}"},
		{testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{interval}, false)), "[10,298]"},
	} {
		var printable fmt.Stringer = tc.set
		if tc.set.String() != tc.want || printable.String() != tc.want || fmt.Sprint(tc.set) != tc.want {
			t.Fatalf("interface formatting differs: %s", tc.set)
		}
	}
	boolean := testutil.Must(types.NewBoolType(foundation.NonNull))
	var included value_set.ValueSet = testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, boolean, []valueinterfaces.Value{testutil.Must(testbedvalues.NewBool(boolean, false)), testutil.Must(testbedvalues.NewBool(boolean, true))}))
	var universal value_set.ValueSet = testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, boolean, nil))
	if !testutil.Must(included.EqualsTo(universal)) || included.String() == universal.String() {
		t.Fatal("diagnostic representation must not stand in for set equality")
	}
}

func Example_valueSetFormatting() {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	var set value_set.ValueSet = testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewInteger(typ, 3)), testutil.Must(testbedvalues.NewInteger(typ, 1))}))
	fmt.Println(set.String())
	floating := testutil.Must(types.NewFloatType(types.Float64, foundation.NonNull))
	lower := testutil.Must(ranges.NewRangeEndpoint(testutil.Must(testbedvalues.NewFloat(floating, 10)), true))
	upper := testutil.Must(ranges.NewRangeEndpoint(testutil.Must(testbedvalues.NewFloat(floating, 299)), false))
	interval, _ := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, floating, lower, upper)).Get()
	set = testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, floating, []*ranges.Range{interval}, true))
	fmt.Println(set)
	// Output:
	// ALL \ {1, 3}
	// [10,299) ∪ {NaN}
}
