package touchstone_test

import (
	"reflect"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	fixture "github.com/lie-yan/predicate-optimization/testbed/touchstone"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func touchstoneMust[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type treeCounts struct{ nodes, inCandidates, arrayCandidates, scalarContains int }

func inspectTouchstone(t *testing.T, f *fixture.Touchstone, root expressions.Expression) (treeCounts, [][]string, [][]int64) {
	t.Helper()
	var count treeCounts
	var stringsLists [][]string
	var integerLists [][]int64
	var visit func(expressions.Expression)
	visit = func(node expressions.Expression) {
		count.nodes++
		switch n := node.(type) {
		case *expressions.SpecialFormExpression:
			args := n.Arguments()
			count.inCandidates += len(args) - 1
			var text []string
			var ints []int64
			for _, arg := range args[1:] {
				literal := arg.(*expressions.LiteralExpression)
				if literal.Type().Nullability() != foundation.NonNull {
					t.Fatal("nullable candidate")
				}
				switch v := literal.Value().(type) {
				case *testbedvalues.VarCharValue:
					text = append(text, v.VarChar())
				case *testbedvalues.IntegerValue:
					ints = append(ints, v.Integer())
				default:
					t.Fatal("unexpected membership value")
				}
			}
			if text != nil {
				stringsLists = append(stringsLists, text)
			}
			if ints != nil {
				integerLists = append(integerLists, ints)
			}
		case *expressions.CallExpression:
			metadata := touchstoneMust(f.Context.Metadata(n.Handle()))
			if name, ok := metadata.BuiltInName().Get(); ok {
				if name == functions.BuiltInArrayContainsAny {
					count.arrayCandidates += len(n.Arguments()[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue).Elements())
				}
				if name == functions.BuiltInArrayContains {
					count.scalarContains++
				}
			}
		}
		for _, child := range node.Children() {
			visit(child)
		}
	}
	visit(root)
	return count, stringsLists, integerLists
}

func TestTouchstoneSchemasAndValidation(t *testing.T) {
	for _, schema := range [][2]foundation.Nullability{
		{foundation.NonNull, foundation.MayNull},
		{foundation.NonNull, foundation.NonNull},
		{foundation.MayNull, foundation.MayNull},
	} {
		f := touchstoneMust(fixture.NewTouchstone(schema[0], schema[1]))
		c := f.Columns
		if c.ArticleType.Type().Nullability() != schema[0] {
			t.Fatal("article nullability lost")
		}
		for _, ref := range []*expressions.ReferenceExpression{c.IsReview, c.BrandID, c.TagIDs, c.CategoryIDs, c.MallIDs} {
			if ref.Type().Nullability() != schema[1] {
				t.Fatal("field nullability lost")
			}
		}
		for _, ref := range []*expressions.ReferenceExpression{c.TagIDs, c.CategoryIDs, c.MallIDs} {
			element := ref.Type().(*types.ArrayType).Element()
			if element.Nullability() != foundation.NonNull || !element.EqualsTo(touchstoneMust(types.NewIntType(types.Int64, foundation.NonNull))) {
				t.Fatal("wrong array element type")
			}
		}
		for _, root := range []expressions.Expression{f.Input, f.Expected} {
			if err := utils.ValidateForOptimization(root); err != nil {
				t.Fatal(err)
			}
		}
		if f.Input.EqualsTo(f.Expected) {
			t.Fatal("input is not independently preserved")
		}
		// Shared reference is used in both roots and multiple guards, without parents.
		if f.Input.Children()[0].Children()[0] != c.ArticleType || f.Expected.Children()[0].Children()[0] != c.ArticleType {
			t.Fatal("reference sharing lost")
		}
	}
	if _, err := fixture.NewTouchstone(foundation.Nullability(255), foundation.NonNull); err == nil {
		t.Fatal("invalid schema admitted")
	}
	if _, err := fixture.NewTouchstone(foundation.NonNull, foundation.Nullability(255)); err == nil {
		t.Fatal("invalid other-field nullability admitted")
	}
}

func TestTouchstoneExactMembershipListsAndCounts(t *testing.T) {
	f := touchstoneMust(fixture.NewPrimaryTouchstone())
	input, text, ints := inspectTouchstone(t, f, f.Input)
	expected, outText, outInts := inspectTouchstone(t, f, f.Expected)
	if input != (treeCounts{111, 46, 4, 7}) || expected != (treeCounts{39, 19, 7, 2}) {
		t.Fatalf("counts: input=%+v expected=%+v", input, expected)
	}
	articles := []string{"tsa_01", "tsa_02", "tsa_03"}
	long := []string{"tsa_01", "tsa_05", "tsa_03", "tsa_04", "tsa_06", "tsa_02", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15"}
	short := []string{"tsa_03", "tsa_02", "tsa_01", "tsa_04"}
	if !reflect.DeepEqual(text, [][]string{articles, long, short, short, short}) || !reflect.DeepEqual(outText, [][]string{articles}) {
		t.Fatal("article candidate lists changed")
	}
	wantInts := [][]int64{{8795001, 8795002, 8795003, 8795004, 8795005}, {8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011}}
	if !reflect.DeepEqual(ints, wantInts) || !reflect.DeepEqual(outInts, wantInts) {
		t.Fatal("review/brand candidates changed")
	}
}

func TestTouchstoneGuardAndArrayStructure(t *testing.T) {
	f := touchstoneMust(fixture.NewPrimaryTouchstone())
	input := f.Input.Children()
	if len(input) != 7 || len(input[6].Children()) != 7 || len(f.Expected.Children()) != 6 {
		t.Fatal("conjunction grouping changed")
	}
	impossible := input[1].Children()[0].Children()
	equal := impossible[1].(*expressions.CallExpression)
	if equal.Arguments()[0] != f.Columns.ArticleType || equal.Arguments()[1].(*expressions.LiteralExpression).Value().(*testbedvalues.VarCharValue).VarChar() != "tsa_04" {
		t.Fatal("impossible article guard changed")
	}
	op, _ := touchstoneMust(f.Context.Metadata(equal.Handle())).Operator().Get()
	if op != foundation.Equal {
		t.Fatal("wrong guard comparison")
	}
	var arrays [][]int64
	var scalarTags []int64
	var walk func(expressions.Expression)
	walk = func(node expressions.Expression) {
		if call, ok := node.(*expressions.CallExpression); ok {
			name, _ := touchstoneMust(f.Context.Metadata(call.Handle())).BuiltInName().Get()
			if name == functions.BuiltInArrayContainsAny {
				var items []int64
				for _, v := range call.Arguments()[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue).Elements() {
					items = append(items, v.(*testbedvalues.IntegerValue).Integer())
				}
				arrays = append(arrays, items)
			}
			if name == functions.BuiltInArrayContains && call.Arguments()[0] == f.Columns.TagIDs {
				scalarTags = append(scalarTags, call.Arguments()[1].(*expressions.LiteralExpression).Value().(*testbedvalues.IntegerValue).Integer())
			}
		}
		for _, child := range node.Children() {
			walk(child)
		}
	}
	walk(f.Input)
	tags := []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}
	if !reflect.DeepEqual(arrays, [][]int64{{8791001, 8791002}, {8792001}, {8793001}}) || !reflect.DeepEqual(scalarTags, tags) {
		t.Fatal("input array constraints changed")
	}
	arrays, scalarTags = nil, nil
	walk(f.Expected)
	if !reflect.DeepEqual(arrays, [][]int64{tags}) || len(scalarTags) != 0 {
		t.Fatal("merged output tag list changed")
	}
	for _, index := range []int{2, 4} {
		call := f.Expected.Children()[index].Children()[0].(*expressions.CallExpression)
		name, _ := touchstoneMust(f.Context.Metadata(call.Handle())).BuiltInName().Get()
		if name != functions.BuiltInArrayContains {
			t.Fatal("singleton normalization missing")
		}
	}
}
