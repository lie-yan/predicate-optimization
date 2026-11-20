package types_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestString(t *testing.T) {
	limit := uint64(100)
	bounded, err := types.NewVarCharType(&limit, foundation.NonNull)
	must(t, bounded, err)
	zero := uint64(0)
	empty, err := types.NewVarCharType(&zero, foundation.MayNull)
	must(t, empty, err)
	child := array(t, integer(t, types.Int64, foundation.MayNull), foundation.NonNull)
	record, err := types.NewStructType([]types.StructField{
		{Name: "id", Type: integer(t, types.Int8, foundation.NonNull)},
		{Name: "tags", Type: child},
		{Name: "a\"b", Type: &types.BoolType{}},
	}, foundation.MayNull)
	must(t, record, err)
	cases := []struct {
		typ  types.Type
		want string
	}{
		{&types.BoolType{}, "Bool?"},
		{&types.VarCharType{}, "VarChar?"},
		{bounded, "VarChar(100 bytes)!"},
		{empty, "VarChar(0 bytes)?"},
		{child, "Array<Int64?>!"},
		{array(t, child, foundation.MayNull), "Array<Array<Int64?>!>?"},
		{&types.StructType{}, "Struct<>?"},
		{record, `Struct<"id": Int8!, "tags": Array<Int64?>!, "a\"b": Bool?>?`},
	}
	for _, c := range cases {
		if got := c.typ.String(); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
		if got := fmt.Sprint(c.typ); got != c.want {
			t.Errorf("fmt got %q, want %q", got, c.want)
		}
	}
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		if got := integer(t, width, foundation.NonNull).String(); got != fmt.Sprintf("Int%d!", width) {
			t.Fatal(got)
		}
	}
}

func TestStringNilAssertion(t *testing.T) {
	for _, typ := range []types.Type{(*types.BoolType)(nil), (*types.IntType)(nil), (*types.VarCharType)(nil), (*types.ArrayType)(nil), (*types.StructType)(nil)} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Errorf("missing assertion for %T", typ)
				}
			}()
			_ = typ.String()
		}()
	}
}
