package values_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

var compositeCountSink int

func TestIndexedArrayOwnershipAndBounds(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.MayNull))
	typ := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	one := fixtureValue(testbedvalues.NewInteger(integer, 1))
	null := fixtureValue(testbedvalues.NewNull(integer))
	input := []valueinterfaces.Value{one, null, one}
	var array valueinterfaces.ArrayValue = fixtureValue(testbedvalues.NewArray(typ, input))
	input[0] = null
	copied := array.Elements()
	copied[2] = null
	if array.ElementCount() != 3 || array.ElementAt(0) != one || array.ElementAt(1) != null || array.ElementAt(2) != one {
		t.Fatal("indexed access lost ownership, order, duplicates or null children")
	}
	adapted := fixtureValue(array.WithNullability(foundation.MayNull)).(valueinterfaces.ArrayValue)
	if adapted.ElementCount() != 3 || adapted.ElementAt(1) != null || array.Type().Nullability() != foundation.NonNull {
		t.Fatal("nullability adaptation changed children or source")
	}
	for _, index := range []int{-1, 3} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("index %d did not panic", index)
				}
			}()
			array.ElementAt(index)
		}()
	}
	empty := fixtureValue(testbedvalues.NewArray(typ, nil))
	if empty.ElementCount() != 0 {
		t.Fatal("empty array has children")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		compositeCountSink = array.ElementCount()
		interfaceValueSink = array.ElementAt(1)
	}); allocations != 0 {
		t.Fatalf("indexed access allocated: %v", allocations)
	}
}
