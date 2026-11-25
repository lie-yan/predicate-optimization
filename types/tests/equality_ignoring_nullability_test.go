package types_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestEqualsToIgnoringNullability(t *testing.T) {
	boolean := &types.BoolType{}
	zero, ten := uint64(0), uint64(10)
	fixtures := []types.Type{boolean}
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		fixtures = append(fixtures, integer(t, width, foundation.MayNull))
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		v, err := types.NewFloatType(width, foundation.MayNull)
		fixtures = append(fixtures, must(t, v, err))
	}
	for _, bound := range []*uint64{nil, &zero, &ten} {
		v, err := types.NewVarCharType(bound, foundation.MayNull)
		fixtures = append(fixtures, must(t, v, err))
	}
	nonNull, err := boolean.WithNullability(foundation.NonNull)
	must(t, nonNull, err)
	fixtures = append(fixtures, array(t, boolean, foundation.MayNull), array(t, nonNull, foundation.MayNull))
	for _, field := range []types.StructField{{Name: "flag", Type: boolean}, {Name: "flag", Type: nonNull}, {Name: "other", Type: boolean}} {
		v, err := types.NewStructType([]types.StructField{field}, foundation.MayNull)
		fixtures = append(fixtures, must(t, v, err))
	}
	for i, a := range fixtures {
		for j, b := range fixtures {
			other, err := b.WithNullability(foundation.NonNull)
			must(t, other, err)
			if a.EqualsToIgnoringNullability(other) != (i == j) || other.EqualsToIgnoringNullability(a) != (i == j) {
				t.Fatalf("unexpected comparison: %s / %s", a, other)
			}
			if a.EqualsTo(other) {
				t.Fatal("ordinary equality ignored outer nullability")
			}
			if a.Nullability() != foundation.MayNull || other.Nullability() != foundation.NonNull {
				t.Fatal("comparison mutated inputs")
			}
		}
	}
}
