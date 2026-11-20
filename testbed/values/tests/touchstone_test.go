package values_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Explicit fixtures represent the synthetic touchstone literal operands.
// This demonstrates representation, not parsing or predicate simplification.
// Candidate lists are represented as arrays for this test; that does not
// prescribe the future IN expression representation. Int64 is a chosen example
// width, not a field-schema requirement of the touchstone.
func TestTouchstoneLiteralValues(t *testing.T) {
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

	stringLists := []struct {
		name     string
		literals []string
	}{
		{"article_type outer", []string{"tsa_01", "tsa_02", "tsa_03"}},
		{"article_type guard for tag 8791004", []string{"tsa_01", "tsa_05", "tsa_03", "tsa_04", "tsa_06", "tsa_02", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15"}},
		{"article_type guard for tag 8791005", []string{"tsa_03", "tsa_02", "tsa_01", "tsa_04"}},
		{"article_type guard for tag 8791007", []string{"tsa_03", "tsa_02", "tsa_01", "tsa_04"}},
		{"article_type guard for tag 8791008", []string{"tsa_03", "tsa_02", "tsa_01", "tsa_04"}},
	}
	for _, c := range stringLists {
		t.Run(c.name, func(t *testing.T) {
			children := make([]valueinterfaces.Value, len(c.literals))
			for i, literal := range c.literals {
				v, err := testbedvalues.NewVarChar(text, literal)
				if err != nil {
					t.Fatal(err)
				}
				children[i] = v
			}
			list, err := testbedvalues.NewArray(strings, children)
			if err != nil {
				t.Fatal(err)
			}
			if list.IsNull() || list.Type() != strings {
				t.Fatal("invalid collection or type was copied")
			}
			got := list.Elements()
			if len(got) != len(c.literals) {
				t.Fatal("literal count changed")
			}
			for i, child := range got {
				v, ok := child.(*testbedvalues.VarCharValue)
				if !ok || v.VarChar() != c.literals[i] || v.Type() != text {
					t.Fatalf("literal %d lost its payload, order or shared type", i)
				}
			}
		})
	}
	integerLists := []struct {
		name     string
		literals []int64
	}{
		{"tag_ids contains-any", []int64{8791001, 8791002}},
		{"is_review IN", []int64{8795001, 8795002, 8795003, 8795004, 8795005}},
		{"category_ids contains-any", []int64{8792001}},
		{"brand_id IN", []int64{8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011}},
		{"mall_ids contains-any", []int64{8793001}},
	}
	for _, c := range integerLists {
		t.Run(c.name, func(t *testing.T) {
			children := make([]valueinterfaces.Value, len(c.literals))
			for i, literal := range c.literals {
				v, err := testbedvalues.NewInteger(number, literal)
				if err != nil {
					t.Fatal(err)
				}
				children[i] = v
			}
			list, err := testbedvalues.NewArray(integers, children)
			if err != nil {
				t.Fatal(err)
			}
			if list.IsNull() || list.Type() != integers {
				t.Fatal("invalid collection or type was copied")
			}
			got := list.Elements()
			if len(got) != len(c.literals) {
				t.Fatal("literal count changed")
			}
			for i, child := range got {
				v, ok := child.(*testbedvalues.IntegerValue)
				if !ok || v.Integer() != c.literals[i] || v.Type() != number {
					t.Fatalf("literal %d lost its payload, order or shared type", i)
				}
			}
		})
	}
	t.Run("article_type equality tsa_04", func(t *testing.T) {
		v, err := testbedvalues.NewVarChar(text, "tsa_04")
		if err != nil {
			t.Fatal(err)
		}
		if v.VarChar() != "tsa_04" || v.IsNull() || v.Type() != text {
			t.Fatal("equality literal changed")
		}
	})
	t.Run("tag_ids contains scalar arguments", func(t *testing.T) {
		for _, literal := range []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009} {
			v, err := testbedvalues.NewInteger(number, literal)
			if err != nil {
				t.Fatal(err)
			}
			if v.Integer() != literal || v.IsNull() || v.Type() != number {
				t.Fatalf("contains literal %d changed", literal)
			}
		}
	})
}
