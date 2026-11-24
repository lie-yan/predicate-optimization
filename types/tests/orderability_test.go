package types_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestOrderability(t *testing.T) {
	boolean, err := types.NewBoolType(foundation.NonNull)
	must(t, boolean, err)
	record, err := types.NewStructType([]types.StructField{{Name: "id", Type: integer(t, types.Int64, foundation.NonNull)}}, foundation.NonNull)
	must(t, record, err)
	cases := []struct {
		typ  types.Type
		want bool
	}{
		{boolean, false},
		{array(t, integer(t, types.Int64, foundation.NonNull), foundation.NonNull), false},
		{array(t, array(t, boolean, foundation.MayNull), foundation.NonNull), false},
		{record, false},
	}
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		cases = append(cases, struct {
			typ  types.Type
			want bool
		}{integer(t, width, foundation.NonNull), true})
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		floating, err := types.NewFloatType(width, foundation.NonNull)
		cases = append(cases, struct {
			typ  types.Type
			want bool
		}{must(t, floating, err), true})
	}
	zero, bound := uint64(0), uint64(100)
	for _, limit := range []*uint64{nil, &zero, &bound} {
		text, err := types.NewVarCharType(limit, foundation.NonNull)
		cases = append(cases, struct {
			typ  types.Type
			want bool
		}{must(t, text, err), true})
	}
	for _, tc := range cases {
		t.Run(tc.typ.String(), func(t *testing.T) {
			for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				adapted, err := tc.typ.WithNullability(n)
				must(t, adapted, err)
				if got := adapted.IsOrderable(); got != tc.want {
					t.Fatalf("%s: IsOrderable=%v, want %v", adapted, got, tc.want)
				}
				restored, err := adapted.WithNullability(foundation.NonNull)
				must(t, restored, err)
				if !restored.EqualsTo(tc.typ) || restored.IsOrderable() != tc.want || tc.typ.Nullability() != foundation.NonNull {
					t.Fatal("nullability adaptation changed type identity or ordering capability")
				}
			}
		})
	}
}
