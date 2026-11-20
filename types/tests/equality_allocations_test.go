package types_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestTypeEqualityWithoutMemoAllocations(t *testing.T) {
	left := integer(t, types.Int64, foundation.NonNull)
	right := integer(t, types.Int64, foundation.NonNull)
	nullable, err := right.WithNullability(foundation.MayNull)
	must(t, nullable, err)
	// Separately allocated shared subgraphs exercise recursive structural equality,
	// while repeated child occurrences retain the same full-nullability semantics.
	makeShared := func(child types.Type) types.Type {
		wrapped := array(t, child, foundation.NonNull)
		record, err := types.NewStructType([]types.StructField{{Name: "a", Type: wrapped}, {Name: "b", Type: wrapped}}, foundation.NonNull)
		return must(t, record, err)
	}
	a, b := makeShared(left), makeShared(right)
	different := makeShared(nullable)
	if a.EqualsTo(different) || a.EqualsToIgnoringNullability(different) {
		t.Fatal("nested nullability was ignored")
	}
	for _, compare := range []func() bool{
		func() bool { return left.EqualsTo(left) }, func() bool { return left.EqualsTo(right) },
		func() bool { return left.EqualsToIgnoringNullability(nullable) },
		func() bool { return a.EqualsTo(b) }, func() bool { return a.EqualsToIgnoringNullability(b) },
	} {
		if !compare() {
			t.Fatal("equal descriptors differ")
		}
		if allocations := testing.AllocsPerRun(100, func() {
			if !compare() {
				panic("equality changed")
			}
		}); allocations != 0 {
			t.Fatalf("Type equality allocated %.0f objects", allocations)
		}
	}
}
