package logical_flattening_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	fixture "github.com/lie-yan/predicate-optimization/testbed/touchstone"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Assemble the flattened scalar result from columns and constants, without
// transforming Input or using Expected (which additionally combines array tests).
// Its outer AND has article/review/category/brand/mall restrictions followed by
// seven separate NOT array_contains(tag_ids, ...) operands in source order.
func expectedFlatTouchstone(f *fixture.Touchstone) expressions.Expression {
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
	any := func(ref *expressions.ReferenceExpression, n int64) expressions.Expression {
		arr := must(testbedvalues.NewArray(must(types.NewArrayType(number, foundation.NonNull)), []valueinterfaces.Value{integer(n).Value()}))
		return call(functions.BuiltInArrayContainsAny, ref, must(expressions.NewLiteralExpression(arr)))
	}
	not := func(e expressions.Expression) expressions.Expression { return logical(foundation.Not, e) }
	var articles []expressions.Expression
	for _, s := range []string{"tsa_01", "tsa_02", "tsa_03"} {
		articles = append(articles, must(expressions.NewLiteralExpression(must(testbedvalues.NewVarChar(text, s)))))
	}
	c := f.Columns
	children := []expressions.Expression{
		in(c.ArticleType, articles...), in(c.IsReview, numbers(8795001, 8795002, 8795003, 8795004, 8795005)...),
		not(any(c.CategoryIDs, 8792001)),
		not(in(c.BrandID, numbers(8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011)...)),
		not(any(c.MallIDs, 8793001)),
	}
	for _, tag := range []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009} {
		children = append(children, not(call(functions.BuiltInArrayContains, c.TagIDs, integer(tag))))
	}
	return logical(foundation.And, children...)
}

type flattenTouchstoneRow struct {
	article                 string
	review, brand           int64
	tags, categories, malls []int64
	nulls                   [6]bool // Article, review, brand, tags, categories, malls; nil arrays are empty.
}

func (r flattenTouchstoneRow) environment(f *fixture.Touchstone) truthEnvironment {
	c := f.Columns
	refs := []*expressions.ReferenceExpression{c.ArticleType, c.IsReview, c.BrandID, c.TagIDs, c.CategoryIDs, c.MallIDs}
	env := truthEnvironment{}
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
		env = append(env, truthBinding{ref.ID(), value})
	}
	return env
}

func flattenTouchstoneRows(nullable, alternateSchedule bool) []flattenTouchstoneRow {
	var rows []flattenTouchstoneRow
	articles := []string{"tsa_01", "tsa_02", "tsa_03", "tsa_05", "tsa_04", "tsa_06", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15", "unlisted", ""}
	if !alternateSchedule {
		// The grouping integration suite covers all articles at these default
		// pipeline stages. Keep the full matrix for the alternate pass order.
		articles = []string{"tsa_01", "tsa_04", "unlisted"}
	}
	for _, article := range articles {
		for _, tags := range [][]int64{nil, {42, 42}, {8791001, 8791002, 8791001}, {8791003}, {8791004}, {8791005}, {8791006}, {8791007}, {8791008}, {8791009}, {8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}} {
			rows = append(rows, flattenTouchstoneRow{review: 8795001, article: article, tags: tags})
		}
		for _, review := range []int64{-1, 8795001, 8795002, 8795003, 3, 8795004, 8795005, 1001} {
			rows = append(rows, flattenTouchstoneRow{article: article, review: review})
		}
		for _, brand := range []int64{-1, 8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011} {
			rows = append(rows, flattenTouchstoneRow{review: 8795001, article: article, brand: brand})
		}
		for _, categories := range [][]int64{nil, {42, 42}, {8792001, 8792001}} {
			for _, malls := range [][]int64{nil, {42, 42}, {8793001, 8793001}} {
				rows = append(rows, flattenTouchstoneRow{review: 8795001, article: article, categories: categories, malls: malls})
			}
		}
		if nullable {
			for mask := 1; mask < 32; mask++ {
				row := flattenTouchstoneRow{review: 8795001, article: article}
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

func touchstoneFlatteningStages(t *testing.T, f *fixture.Touchstone, first bool) []expressions.Expression {
	t.Helper()
	flatten := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
	contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, f.Context))
	cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, f.Context))
	passes := []rewriter.ExpressionTreeRewriter[struct{}]{contextual, cleanup, flatten}
	if first {
		passes = []rewriter.ExpressionTreeRewriter[struct{}]{flatten, contextual, cleanup}
	}
	stages := []expressions.Expression{f.Input}
	sourceSnapshots := snapshotChildren(f.Input)
	for _, pass := range passes {
		previous := stages[len(stages)-1]
		snapshots := snapshotChildren(previous)
		result := must(pass.Rewrite(previous, struct{}{}))
		if !result.Type().EqualsTo(f.Input.Type()) || reconstruction.MeasureExpression(result).NodeCount > reconstruction.MeasureExpression(previous).NodeCount {
			t.Fatal("touchstone stage changed Type or grew")
		}
		checkSnapshots(t, snapshots)
		checkSnapshots(t, sourceSnapshots)
		stages = append(stages, result)
	}
	return stages
}

func TestTouchstoneFlatteningSchedules(t *testing.T) {
	for _, articleNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for _, otherNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			for _, first := range []bool{true, false} {
				t.Run(fmt.Sprintf("article-%v/others-%v/flatten-first-%t", articleNullability, otherNullability, first), func(t *testing.T) {
					f := must(fixture.NewTouchstone(articleNullability, otherNullability))
					stages := touchstoneFlatteningStages(t, f, first)
					result := stages[3]
					expectedSizes := []reconstruction.ExpressionSize{{NodeCount: 111, LiteralOccurrences: 57, MembershipEntries: 46}, {NodeCount: 65, LiteralOccurrences: 29, MembershipEntries: 19}, {NodeCount: 64, LiteralOccurrences: 28, MembershipEntries: 19}, {NodeCount: 63, LiteralOccurrences: 28, MembershipEntries: 19}}
					if first {
						expectedSizes = []reconstruction.ExpressionSize{{NodeCount: 111, LiteralOccurrences: 57, MembershipEntries: 46}, {NodeCount: 110, LiteralOccurrences: 57, MembershipEntries: 46}, {NodeCount: 64, LiteralOccurrences: 29, MembershipEntries: 19}, {NodeCount: 63, LiteralOccurrences: 28, MembershipEntries: 19}}
					}
					checks := append([]expressions.Expression{}, stages...)
					if articleNullability == foundation.NonNull {
						want := expectedFlatTouchstone(f)
						if !result.EqualsTo(want) || result.EqualsTo(f.Expected) {
							t.Fatalf("unexpected scalar-stage shape\n%s\nwant\n%s", expressions.PrettyPrint(result), expressions.PrettyPrint(want))
						}
						if result.Children()[0] != f.Input.Children()[0] {
							t.Fatal("article guard instance changed")
						}
						for i := 1; i < 5; i++ {
							if result.Children()[i] != f.Input.Children()[i+1] {
								t.Fatal("unaffected restriction replaced")
							}
						}
						checks = append(checks, want)
					} else {
						// All guards remain nullable; only grouping changes. Do not require
						// unchanged root identity across the added flattening pass.
						for i := range expectedSizes {
							expectedSizes[i] = reconstruction.ExpressionSize{NodeCount: 111, LiteralOccurrences: 57, MembershipEntries: 46}
							if first && i > 0 || !first && i == 3 {
								expectedSizes[i].NodeCount = 110
							}
						}
						if first && (stages[2] != stages[1] || stages[3] != stages[2]) || !first && (stages[1] != f.Input || stages[2] != f.Input) {
							t.Fatal("nullable article authorized scalar reduction")
						}
					}
					for i, node := range stages {
						if actual := reconstruction.MeasureExpression(node); actual != expectedSizes[i] {
							t.Fatalf("stage %d counts: got %+v want %+v", i, actual, expectedSizes[i])
						}
					}
					evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
					rows := flattenTouchstoneRows(otherNullability == foundation.MayNull, first)
					for i, row := range rows {
						env := row.environment(f)
						original := must(evaluator.Evaluate(f.Input, env))
						for j, node := range checks {
							actual := must(evaluator.Evaluate(node, env))
							if !actual.EqualsTo(original) || !actual.Type().EqualsTo(f.Input.Type()) {
								t.Fatalf("row %d stage %d changed truth/Type: %+v", i, j, row)
							}
						}
					}
					if articleNullability == foundation.MayNull {
						for _, tc := range []struct {
							tags         []int64
							want, unsafe foundation.TruthValue
						}{
							{nil, foundation.Null, foundation.Null}, {[]int64{8791004}, foundation.Null, foundation.False}, {[]int64{8791003}, foundation.False, foundation.False},
						} {
							env := (flattenTouchstoneRow{review: 8795001, tags: tc.tags, nulls: [6]bool{true}}).environment(f)
							want := booleanValue(tc.want, f.Input.Type().(*types.BoolType))
							for _, node := range stages {
								if !must(evaluator.Evaluate(node, env)).EqualsTo(want) {
									t.Fatal("nullable-article counterexample changed")
								}
							}
							unsafe := booleanValue(tc.unsafe, f.Expected.Type().(*types.BoolType))
							if !must(evaluator.Evaluate(f.Expected, env)).EqualsTo(unsafe) {
								t.Fatal("nullable-article counterexample no longer distinguishes full proposed reduction")
							}
						}
					}
					t.Logf("rows=%d; stage sizes=%+v", len(rows), expectedSizes)
				})
			}
		}
	}
}
