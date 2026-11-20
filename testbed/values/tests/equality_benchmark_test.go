package values_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

var equalityBenchmarkResult bool

func benchmarkValueEquality(b *testing.B, left, right valueinterfaces.Value) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		equalityBenchmarkResult = left.EqualsTo(right)
	}
}

// Shared Type pointers isolate Value comparison from Type graph traversal.
// Values and fixtures are built outside the timed loops.
func BenchmarkValueEquality(b *testing.B) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	for _, n := range []int{64, 256, 1024} {
		left, right := make([]valueinterfaces.Value, n), make([]valueinterfaces.Value, n)
		for i := range left {
			left[i] = fixtureValue(testbedvalues.NewInteger(integer, int64(i)))
			right[i] = fixtureValue(testbedvalues.NewInteger(integer, int64(i)))
		}
		a, c := fixtureValue(testbedvalues.NewArray(array, left)), fixtureValue(testbedvalues.NewArray(array, right))
		b.Run(fmt.Sprintf("FlatArray/n=%d", n), func(b *testing.B) { benchmarkValueEquality(b, a, c) })
		b.Run(fmt.Sprintf("Identity/n=%d", n), func(b *testing.B) { benchmarkValueEquality(b, a, a) })
		record := fixtureValue(types.NewStructType([]types.StructField{{Name: "a", Type: array}, {Name: "b", Type: array}}, foundation.NonNull))
		x, y := fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{a, a})), fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{c, c}))
		b.Run(fmt.Sprintf("StructSharedArray/n=%d", n), func(b *testing.B) { benchmarkValueEquality(b, x, y) })
	}
	for _, depth := range []int{4, 8, 12} {
		var a, c valueinterfaces.Value = fixtureValue(testbedvalues.NewInteger(integer, 1)), fixtureValue(testbedvalues.NewInteger(integer, 1))
		for level := 0; level < depth; level++ {
			typ := fixtureValue(types.NewArrayType(a.Type(), foundation.NonNull))
			a = fixtureValue(testbedvalues.NewArray(typ, []valueinterfaces.Value{a, a}))
			c = fixtureValue(testbedvalues.NewArray(typ, []valueinterfaces.Value{c, c}))
		}
		b.Run(fmt.Sprintf("SharedDAG/depth=%d", depth), func(b *testing.B) { benchmarkValueEquality(b, a, c) })
	}
}
