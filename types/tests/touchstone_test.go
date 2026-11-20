package types_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

// These are representations of the synthetic touchstone literal operands.
// Int64 is an illustrative carrier choice: the touchstone does not fix integer
// widths. Literal nullability is independent of the referenced field's schema.
// Array types here describe literal collections, not the eventual IN node API.
func TestTouchstoneLiteralTypes(t *testing.T) {
	text, err := types.NewVarCharType(nil, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	number, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	strings, err := types.NewArrayType(text, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	integers, err := types.NewArrayType(number, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		typ     types.Type
		want    string
		element types.Type
	}{
		{"article_type equality tsa_04", text, "VarChar!", nil},
		{"article_type outer and guarded IN lists", strings, "Array<VarChar!>!", text},
		{"is_review IN list", integers, "Array<Int64!>!", number},
		{"brand_id IN list", integers, "Array<Int64!>!", number},
		{"tag_ids contains-any list", integers, "Array<Int64!>!", number},
		{"category_ids contains-any singleton", integers, "Array<Int64!>!", number},
		{"mall_ids contains-any singleton", integers, "Array<Int64!>!", number},
		{"tag_ids contains scalar arguments", number, "Int64!", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.typ.String() != c.want {
				t.Fatalf("unexpected type: %s", c.typ.String())
			}
			if c.element != nil {
				a, ok := c.typ.(*types.ArrayType)
				if !ok || a.Element() != c.element {
					t.Fatal("literal collection must share its element descriptor")
				}
			}
		})
	}
	if text.EqualsTo(number) || strings.EqualsTo(integers) {
		t.Fatal("string and integer domains conflated")
	}
}
