package touchstone_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	fixture "github.com/lie-yan/predicate-optimization/testbed/touchstone"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Bind by scoped identity rather than a reference's diagnostic display name.
type rowBinding struct {
	reference *expressions.ReferenceExpression
	value     valueinterfaces.Value
}
type rowEnvironment []rowBinding

func (r rowEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for _, b := range r {
		if b.reference.ID().EqualsTo(id) {
			return foundation.Some(b.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

// A nil array slice is an empty array. SQL NULL is selected explicitly by column.
type inputRow struct {
	article                 string
	review, brand           int64
	tags, categories, malls []int64
	nullColumns             map[string]bool
}

func (r inputRow) environment(f *fixture.Touchstone) rowEnvironment {
	c := f.Columns
	refs := []*expressions.ReferenceExpression{c.ArticleType, c.IsReview, c.BrandID, c.TagIDs, c.CategoryIDs, c.MallIDs}
	names := []string{"article", "review", "brand", "tags", "categories", "malls"}
	var env rowEnvironment
	for index, ref := range refs {
		var value valueinterfaces.Value
		if r.nullColumns[names[index]] {
			value = touchstoneMust(testbedvalues.NewNull(ref.Type()))
		} else {
			switch index {
			case 0:
				value = touchstoneMust(testbedvalues.NewVarChar(ref.Type().(*types.VarCharType), r.article))
			case 1:
				value = touchstoneMust(testbedvalues.NewInteger(ref.Type().(*types.IntType), r.review))
			case 2:
				value = touchstoneMust(testbedvalues.NewInteger(ref.Type().(*types.IntType), r.brand))
			default:
				typ := ref.Type().(*types.ArrayType)
				var elements []valueinterfaces.Value
				for _, n := range [][]int64{r.tags, r.categories, r.malls}[index-3] {
					elements = append(elements, touchstoneMust(testbedvalues.NewInteger(typ.Element().(*types.IntType), n)))
				}
				value = touchstoneMust(testbedvalues.NewArray(typ, elements))
			}
		}
		env = append(env, rowBinding{ref, value})
	}
	return env
}

func assertPair(t *testing.T, f *fixture.Touchstone, row inputRow, inputWant, outputWant foundation.TruthValue) {
	t.Helper()
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
	env := row.environment(f)
	for _, item := range []struct {
		name string
		root expressions.Expression
		want foundation.TruthValue
	}{
		{"input", f.Input, inputWant}, {"expected", f.Expected, outputWant},
	} {
		result, err := evaluator.Evaluate(item.root, env)
		if err != nil {
			t.Fatalf("%s: %v", item.name, err)
		}
		if !result.Type().EqualsTo(item.root.Type()) {
			t.Fatalf("%s: result type changed", item.name)
		}
		got := foundation.Null
		if !result.IsNull() {
			b, ok := result.(*testbedvalues.BoolValue)
			if !ok {
				t.Fatalf("%s: result is not Boolean", item.name)
			}
			got = foundation.False
			if b.Bool() {
				got = foundation.True
			}
		}
		if got != item.want {
			t.Fatalf("%s: truth=%v, want %v", item.name, got, item.want)
		}
	}
}

func TestTouchstoneNonNullAssignments(t *testing.T) {
	// Both schemas accept concrete rows, but yield different effective result Types.
	for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		t.Run(fmt.Sprint(nullability), func(t *testing.T) {
			f := touchstoneMust(fixture.NewTouchstone(foundation.NonNull, nullability))
			articles := []string{"tsa_01", "tsa_02", "tsa_03", "tsa_05", "tsa_04", "tsa_06", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15", "unlisted", ""}
			for index, article := range articles {
				t.Run("article/"+article, func(t *testing.T) {
					want := foundation.False
					if index < 3 {
						want = foundation.True
					}
					assertPair(t, f, inputRow{review: 8795001, article: article}, want, want)
					// These tags occur only in the impossible tsa_04 guard.
					assertPair(t, f, inputRow{review: 8795001, article: article, tags: []int64{8791001, 8791002, 8791001}}, want, want)
				})
			}
			for _, article := range articles[:3] {
				for _, tag := range []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009} {
					t.Run(fmt.Sprintf("%s/excluded-tag/%d", article, tag), func(t *testing.T) {
						assertPair(t, f, inputRow{review: 8795001, article: article, tags: []int64{42, tag, tag}}, foundation.False, foundation.False)
					})
				}
			}
			for _, review := range []int64{8795001, 8795002, 8795003, 8795004, 8795005, -1, 3, 1001} {
				t.Run(fmt.Sprintf("review/%d", review), func(t *testing.T) {
					want := foundation.True
					if review == -1 || review == 3 || review == 1001 {
						want = foundation.False
					}
					assertPair(t, f, inputRow{article: "tsa_01", review: review}, want, want)
				})
			}
			for _, brand := range []int64{8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011} {
				t.Run(fmt.Sprintf("excluded-brand/%d", brand), func(t *testing.T) {
					assertPair(t, f, inputRow{review: 8795001, article: "tsa_01", brand: brand}, foundation.False, foundation.False)
				})
			}
			for _, tc := range []struct {
				name string
				row  inputRow
				want foundation.TruthValue
			}{
				{"empty-arrays", inputRow{review: 8795001, article: "tsa_01"}, foundation.True},
				{"duplicate-misses", inputRow{review: 8795001, article: "tsa_01", brand: -1, tags: []int64{42, 42}, categories: []int64{42, 42}, malls: []int64{42, 42}}, foundation.True},
				{"category-hit", inputRow{review: 8795001, article: "tsa_01", categories: []int64{42, 8792001, 8792001}}, foundation.False},
				{"mall-hit", inputRow{review: 8795001, article: "tsa_01", malls: []int64{8793001, 8793001, 42}}, foundation.False},
				{"all-tags", inputRow{review: 8795001, article: "tsa_01", tags: []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}}, foundation.False},
			} {
				t.Run(tc.name, func(t *testing.T) { assertPair(t, f, tc.row, tc.want, tc.want) })
			}
		})
	}
}

func TestTouchstoneNullableFields(t *testing.T) {
	f := touchstoneMust(fixture.NewPrimaryTouchstone())
	// All 31 nonempty combinations, including null arrays versus empty arrays.
	fields := []string{"review", "brand", "tags", "categories", "malls"}
	for mask := 1; mask < 1<<len(fields); mask++ {
		t.Run(fmt.Sprintf("null-mask/%02d", mask), func(t *testing.T) {
			nulls := map[string]bool{}
			for index, name := range fields {
				if mask&(1<<index) != 0 {
					nulls[name] = true
				}
			}
			assertPair(t, f, inputRow{review: 8795001, article: "tsa_01", nullColumns: nulls}, foundation.Null, foundation.Null)
			assertPair(t, f, inputRow{review: 8795001, article: "tsa_04", nullColumns: nulls}, foundation.False, foundation.False)
		})
	}
	// A definite rejection dominates NULL from a different conjunct.
	for _, row := range []inputRow{
		{article: "tsa_01", review: 3, nullColumns: map[string]bool{"tags": true}},
		{review: 8795001, article: "tsa_01", brand: 8794001, nullColumns: map[string]bool{"review": true}},
		{review: 8795001, article: "tsa_01", tags: []int64{8791004}, nullColumns: map[string]bool{"categories": true}},
		{review: 8795001, article: "tsa_01", categories: []int64{8792001}, nullColumns: map[string]bool{"malls": true}},
		{review: 8795001, article: "tsa_01", malls: []int64{8793001}, nullColumns: map[string]bool{"brand": true}},
	} {
		assertPair(t, f, row, foundation.False, foundation.False)
	}
}

func TestTouchstoneNullableArticleCounterexample(t *testing.T) {
	f := touchstoneMust(fixture.NewTouchstone(foundation.MayNull, foundation.MayNull))
	for _, tc := range []struct {
		name          string
		tags          []int64
		input, output foundation.TruthValue
	}{
		{"empty", nil, foundation.Null, foundation.Null},
		{"guarded-tag-8791004", []int64{8791004}, foundation.Null, foundation.False},
		{"unconditional-tag-8791003", []int64{8791003}, foundation.False, foundation.False},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertPair(t, f, inputRow{review: 8795001, tags: tc.tags, nullColumns: map[string]bool{"article": true}}, tc.input, tc.output)
		})
	}
}
