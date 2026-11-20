package values_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestValueEqualityWithoutMemoAllocations(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	record := fixtureValue(types.NewStructType([]types.StructField{{Name: "a", Type: array}, {Name: "b", Type: array}}, foundation.NonNull))
	makeArray := func(n int64) *testbedvalues.ArrayValue {
		return fixtureValue(testbedvalues.NewArray(array, []valueinterfaces.Value{fixtureValue(testbedvalues.NewInteger(integer, n))}))
	}
	a, b, c := makeArray(1), makeArray(1), makeArray(2)
	left := fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{a, a}))
	right := fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{b, b}))
	different := fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{b, c}))
	if left.EqualsTo(different) || different.EqualsTo(left) {
		t.Fatal("later child mismatch missed")
	}
	for _, pair := range [][2]valueinterfaces.Value{{a, a}, {a, b}, {left, left}, {left, right}} {
		if !pair[0].EqualsTo(pair[1]) {
			t.Fatal("equal Values differ")
		}
		if allocations := testing.AllocsPerRun(100, func() {
			if !pair[0].EqualsTo(pair[1]) {
				panic("equality changed")
			}
		}); allocations != 0 {
			t.Fatalf("Value equality allocated %.0f objects", allocations)
		}
	}
}
