package array_membership_normalization_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	fixture "github.com/lie-yan/predicate-optimization/testbed/touchstone"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Assemble the normalized scalar result from columns and constants, without
// transforming Input or using Expected (which additionally combines array tests).
// Its outer AND has article/review/category/brand/mall restrictions followed by
// seven separate NOT array_contains(tag_ids, ...) operands in source order.
// Category/mall use contains; the seven tag exclusions remain ungrouped.
func expectedNormalizedTouchstone(f *fixture.Touchstone) expressions.Expression {
	number := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	integer := func(n int64) *expressions.LiteralExpression {
		return must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(number, n))))
	}
	in := func(ref *expressions.ReferenceExpression, items ...expressions.Expression) expressions.Expression {
		return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(ref.Type().Nullability())), append([]expressions.Expression{ref}, items...)))
	}
	numbers := func(ns ...int64) []expressions.Expression {
		result := make([]expressions.Expression, len(ns))
		for i, n := range ns {
			result[i] = integer(n)
		}
		return result
	}
	call := func(name functions.BuiltInFunctionName, ref *expressions.ReferenceExpression, arg expressions.Expression) expressions.Expression {
		handle := must(f.Context.LookupBuiltInFunction(name, []types.Type{ref.Type(), arg.Type()}))
		return must(expressions.NewCallExpression(f.Context, handle, []expressions.Expression{ref, arg}))
	}
	not := func(e expressions.Expression) expressions.Expression { return compositionLogical(foundation.Not, e) }
	var articles []expressions.Expression
	for _, s := range []string{"tsa_01", "tsa_02", "tsa_03"} {
		articles = append(articles, must(expressions.NewLiteralExpression(must(testbedvalues.NewVarChar(text, s)))))
	}
	c := f.Columns
	children := []expressions.Expression{
		in(c.ArticleType, articles...), in(c.IsReview, numbers(8795001, 8795002, 8795003, 8795004, 8795005)...),
		not(call(functions.BuiltInArrayContains, c.CategoryIDs, integer(8792001))),
		not(in(c.BrandID, numbers(8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011)...)),
		not(call(functions.BuiltInArrayContains, c.MallIDs, integer(8793001))),
	}
	for _, tag := range []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009} {
		children = append(children, not(call(functions.BuiltInArrayContains, c.TagIDs, integer(tag))))
	}
	return compositionLogical(foundation.And, children...)
}

type normalizationTouchstoneRow struct {
	article                 string
	review, brand           int64
	tags, categories, malls []int64
	nulls                   [6]bool // Article, review, brand, tags, categories, malls; nil arrays are empty.
}

func (r normalizationTouchstoneRow) environment(f *fixture.Touchstone) *fixtureEnvironment {
	c := f.Columns
	refs := []*expressions.ReferenceExpression{c.ArticleType, c.IsReview, c.BrandID, c.TagIDs, c.CategoryIDs, c.MallIDs}
	env := &fixtureEnvironment{}
	for i, ref := range refs {
		var value valueinterfaces.Value
		if r.nulls[i] {
			value = must(testbedvalues.NewNull(ref.Type()))
		} else {
			switch i {
			case 0:
				value = must(testbedvalues.NewVarChar(ref.Type().(*types.VarCharType), r.article))
			case 1, 2:
				value = must(testbedvalues.NewInteger(ref.Type().(*types.IntType), []int64{r.review, r.brand}[i-1]))
			default:
				typ := ref.Type().(*types.ArrayType)
				var items []valueinterfaces.Value
				for _, n := range [][]int64{r.tags, r.categories, r.malls}[i-3] {
					items = append(items, must(testbedvalues.NewInteger(typ.Element().(*types.IntType), n)))
				}
				value = must(testbedvalues.NewArray(typ, items))
			}
		}
		env.bindings = append(env.bindings, fixtureBinding{ref.ID(), value})
	}
	return env
}

func normalizationTouchstoneRows(nullable, alternateSchedule bool) []normalizationTouchstoneRow {
	var rows []normalizationTouchstoneRow
	articles := []string{"tsa_01", "tsa_02", "tsa_03", "tsa_05", "tsa_04", "tsa_06", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15", "unlisted", ""}
	if !alternateSchedule {
		// The grouping integration suite covers all articles at these default
		// pipeline stages. Keep the full matrix for the alternate pass order.
		articles = []string{"tsa_01", "tsa_04", "unlisted"}
	}
	for _, article := range articles {
		for _, tags := range [][]int64{nil, {42, 42}, {8791001, 8791002, 8791001}, {8791003}, {8791004}, {8791005}, {8791006}, {8791007}, {8791008}, {8791009}, {8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}} {
			rows = append(rows, normalizationTouchstoneRow{review: 8795001, article: article, tags: tags})
		}
		for _, review := range []int64{-1, 8795001, 8795002, 8795003, 3, 8795004, 8795005, 1001} {
			rows = append(rows, normalizationTouchstoneRow{article: article, review: review})
		}
		for _, brand := range []int64{-1, 8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011} {
			rows = append(rows, normalizationTouchstoneRow{review: 8795001, article: article, brand: brand})
		}
		for _, categories := range [][]int64{nil, {42, 42}, {8792001, 8792001}} {
			for _, malls := range [][]int64{nil, {42, 42}, {8793001, 8793001}} {
				rows = append(rows, normalizationTouchstoneRow{review: 8795001, article: article, categories: categories, malls: malls})
			}
		}
		if nullable {
			for mask := 1; mask < 32; mask++ {
				row := normalizationTouchstoneRow{review: 8795001, article: article}
				for i := 1; i < 6; i++ {
					row.nulls[i] = mask&(1<<(i-1)) != 0
				}
				rows = append(rows, row)
				row.tags, row.categories, row.malls = []int64{8791004}, []int64{8792001}, []int64{8793001}
				rows = append(rows, row)
			}
		}
	}
	return rows
}

// Assert actual call families, referenced columns and candidate payloads, rather
// than treating unchanged node counts as proof that normalization happened.
func assertNormalizedTouchstoneCalls(t *testing.T, node expressions.Expression, f *fixture.Touchstone, nullableArticle bool) {
	t.Helper()
	counts := make(map[string]int)
	var visit func(expressions.Expression)
	visit = func(node expressions.Expression) {
		if call, ok := node.(*expressions.CallExpression); ok {
			name, present := must(call.Context().Metadata(call.Handle())).BuiltInName().Get()
			if present && (name == functions.BuiltInArrayContains || name == functions.BuiltInArrayContainsAny) {
				args := call.Arguments()
				ref := args[0].(*expressions.ReferenceExpression)
				switch ref {
				case f.Columns.CategoryIDs, f.Columns.MallIDs:
					want := int64(8792001)
					key := "category"
					if ref == f.Columns.MallIDs {
						want, key = 8793001, "mall"
					}
					if name != functions.BuiltInArrayContains || !args[1].EqualsTo(must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(must(types.NewIntType(types.Int64, foundation.NonNull)), want))))) {
						t.Fatal("category/mall call did not normalize")
					}
					counts[key]++
				case f.Columns.TagIDs:
					if name == functions.BuiltInArrayContains {
						counts["tag"]++
					} else {
						counts["tag-any"]++
						elements := args[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue).Elements()
						if len(elements) != 2 {
							t.Fatal("tag exclusions were grouped prematurely")
						}
					}
				default:
					t.Fatal("unexpected membership array")
				}
			}
		}
		for _, child := range node.Children() {
			visit(child)
		}
	}
	visit(node)
	anyCount := 0
	if nullableArticle {
		anyCount = 1
	}
	if counts["category"] != 1 || counts["mall"] != 1 || counts["tag"] != 7 || counts["tag-any"] != anyCount {
		t.Fatalf("unexpected membership calls: %v", counts)
	}
}

func TestTouchstoneNormalizationSchedules(t *testing.T) {
	for _, articleNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for _, otherNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			for _, first := range []bool{true, false} {
				t.Run(fmt.Sprintf("article=%v/others=%v/normalize-first=%t", articleNullability, otherNullability, first), func(t *testing.T) {
					f := must(fixture.NewTouchstone(articleNullability, otherNullability))
					stages := normalizationCompositionStages(t, f.Input, f.Context, first)
					result := stages[4]
					assertNormalizedTouchstoneCalls(t, result, f, articleNullability == foundation.MayNull)
					checks := append([]expressions.Expression{}, stages...)
					if articleNullability == foundation.NonNull {
						expected := expectedNormalizedTouchstone(f)
						if !result.EqualsTo(expected) || result.EqualsTo(f.Expected) {
							t.Fatalf("unexpected normalized scalar intermediate\n%s\nwant\n%s", expressions.PrettyPrint(result), expressions.PrettyPrint(expected))
						}
						if actual := reconstruction.MeasureExpression(result); actual != (reconstruction.ExpressionSize{NodeCount: 63, LiteralOccurrences: 28, MembershipEntries: 19}) {
							t.Fatalf("unexpected normalized size: %+v", actual)
						}
						checks = append(checks, expected)
						// Untouched article/review/brand restrictions retain their instances.
						for _, positions := range [][2]int{{0, 0}, {1, 2}, {3, 4}} {
							if result.Children()[positions[0]] != f.Input.Children()[positions[1]] {
								t.Fatal("unaffected restriction was rebuilt")
							}
						}
					} else {
						// The nullable schema admits no scoped scalar guard reduction. Only
						// flattening and the two singleton membership replacements may happen.
						if first && (stages[2] != stages[1] || stages[3] != stages[2]) || !first && (stages[1] != f.Input || stages[2] != f.Input) {
							t.Fatal("nullable article authorized scalar guard reductions")
						}
						if actual := reconstruction.MeasureExpression(result); actual != (reconstruction.ExpressionSize{NodeCount: 110, LiteralOccurrences: 57, MembershipEntries: 46}) {
							t.Fatalf("nullable input lost restrictions: %+v", actual)
						}
					}
					evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
					rows := normalizationTouchstoneRows(otherNullability == foundation.MayNull, first)
					for i, row := range rows {
						expected := must(evaluator.Evaluate(f.Input, row.environment(f)))
						for j, expression := range checks {
							actual := must(evaluator.Evaluate(expression, row.environment(f)))
							if !actual.EqualsTo(expected) || !actual.Type().EqualsTo(f.Input.Type()) {
								t.Fatalf("row %d, stage %d changed truth or Type: %+v", i, j, row)
							}
						}
					}
					if articleNullability == foundation.MayNull {
						for _, tc := range []struct {
							tags          []int64
							truth, unsafe foundation.TruthValue
						}{
							{nil, foundation.Null, foundation.Null},
							{[]int64{8791004}, foundation.Null, foundation.False},
							{[]int64{8791003}, foundation.False, foundation.False},
						} {
							row := normalizationTouchstoneRow{review: 8795001, tags: tc.tags, nulls: [6]bool{true}}
							expected := membershipTruthValue(f.Input.Type().(*types.BoolType), tc.truth)
							for _, expression := range stages {
								if actual := must(evaluator.Evaluate(expression, row.environment(f))); !actual.EqualsTo(expected) || !actual.Type().EqualsTo(expected.Type()) {
									t.Fatal("nullable article counterexample changed")
								}
							}
							unsafe := membershipTruthValue(f.Expected.Type().(*types.BoolType), tc.unsafe)
							if !must(evaluator.Evaluate(f.Expected, row.environment(f))).EqualsTo(unsafe) {
								t.Fatal("proposed final output no longer distinguishes nullable-article counterexample")
							}
						}
					}
					t.Logf("verified %d complete rows for this schedule; final size %+v", len(rows), reconstruction.MeasureExpression(result))
				})
			}
		}
	}
}
