package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestSignatureIndexedAccess(t *testing.T) {
	integer := bindingMust(types.NewIntType(types.Int64, foundation.MayNull))
	array := bindingMust(types.NewArrayType(integer, foundation.NonNull))
	result := bindingMust(types.NewBoolType(foundation.NonNull))
	for _, args := range [][]types.Type{nil, {}, {integer}, {array, integer, array}} {
		original := append([]types.Type(nil), args...)
		signature := bindingMust(functions.NewFunctionSignature(args, result))
		for i := range args {
			args[i] = result
		}
		returned := signature.ArgumentTypes()
		if signature.ArgumentCount() != len(original) {
			t.Fatal("count differs from constructed arity")
		}
		for i, want := range original {
			if signature.ArgumentTypeAt(i) != want || returned[i] != want {
				t.Fatal("descriptor order, identity or constructor ownership changed")
			}
			returned[i] = result
			if signature.ArgumentTypeAt(i) != want || signature.ArgumentTypes()[i] != want {
				t.Fatal("returned slice aliases signature storage")
			}
		}
		if allocations := testing.AllocsPerRun(100, func() {
			if signature.ArgumentCount() != len(original) {
				panic("unstable count")
			}
			for i, want := range original {
				if signature.ArgumentTypeAt(i) != want {
					panic("unstable argument")
				}
			}
		}); allocations != 0 {
			t.Fatalf("indexed access allocated %g times", allocations)
		}
		for _, index := range []int{-1, len(original), len(original) + 1} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("index %d did not panic for arity %d", index, len(original))
					}
				}()
				signature.ArgumentTypeAt(index)
			}()
		}
	}
}
