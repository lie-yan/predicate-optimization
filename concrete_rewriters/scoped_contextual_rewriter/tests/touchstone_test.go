package scoped_contextual_rewriter_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
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

// Construct the scalar result from columns and constants, without inspecting
// Input, invoking analysis, or using the fixture's final array-optimized Expected.
// With neutral=false, the deparsed predicate is:
//
//	article_type in ["tsa_01", "tsa_02", "tsa_03"]
//	and is_review in [8795001, 8795002, 8795003, 8795004, 8795005]
//	and not array_contains_any(category_ids, [8792001])
//	and not (brand_id in [
//	    8794001, 8794002, 8794003, 8794004, 8794005, 8794006,
//	    8794007, 8794008, 8794009, 8794010, 8794011
//	])
//	and not array_contains_any(mall_ids, [8793001])
//	and (
//	    not array_contains(tag_ids, 8791003)
//	    and not array_contains(tag_ids, 8791004)
//	    and not array_contains(tag_ids, 8791005)
//	    and not array_contains(tag_ids, 8791006)
//	    and not array_contains(tag_ids, 8791007)
//	    and not array_contains(tag_ids, 8791008)
//	    and not array_contains(tag_ids, 8791009)
//	)
//
// With neutral=true, insert AND True immediately after the article_type
// membership to represent the accepted contextual result before Boolean cleanup.
// That literal retains tag_ids' declared nullability, which this syntax omits.
// Both variants keep the seven tag exclusions separate and the category/mall
// singleton array_contains_any calls unchanged.
func expectedTouchstoneScalar(f *fixture.Touchstone, neutral bool) expressions.Expression {
	c := f.Columns
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	articleValues := []valueinterfaces.Value{must(testbedvalues.NewVarChar(text, "tsa_01")), must(testbedvalues.NewVarChar(text, "tsa_02")), must(testbedvalues.NewVarChar(text, "tsa_03"))}
	call := func(name functions.BuiltInFunctionName, column *expressions.ReferenceExpression, candidate expressions.Expression) expressions.Expression {
		handle := must(f.Context.LookupBuiltInFunction(name, []types.Type{column.Type(), candidate.Type()}))
		return must(expressions.NewCallExpression(f.Context, handle, []expressions.Expression{column, candidate}))
	}
	any := func(column *expressions.ReferenceExpression, n int64) expressions.Expression {
		item := integer(n, foundation.NonNull).Value()
		arrayType := must(types.NewArrayType(item.Type(), foundation.NonNull))
		return call(functions.BuiltInArrayContainsAny, column, scalarLiteral(must(testbedvalues.NewArray(arrayType, []valueinterfaces.Value{item}))))
	}
	children := []expressions.Expression{scalarMembership(c.ArticleType, articleValues...)}
	if neutral {
		children = append(children, booleanLiteral(foundation.True, c.TagIDs.Type().Nullability()))
	}
	children = append(children,
		membership(c.IsReview, 8795001, 8795002, 8795003, 8795004, 8795005),
		logical(foundation.Not, any(c.CategoryIDs, 8792001)),
		logical(foundation.Not, membership(c.BrandID, 8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011)),
		logical(foundation.Not, any(c.MallIDs, 8793001)),
	)
	var tags []expressions.Expression
	for _, n := range []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009} {
		tags = append(tags, logical(foundation.Not, call(functions.BuiltInArrayContains, c.TagIDs, integer(n, foundation.NonNull))))
	}
	return logical(foundation.And, append(children, logical(foundation.And, tags...))...)
}

func rewriteTouchstone(t *testing.T, f *fixture.Touchstone) []expressions.Expression {
	t.Helper()
	before := expressions.PrettyPrint(f.Input)
	contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, f.Context))
	cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, f.Context))
	intermediate := must(contextual.Rewrite(f.Input, struct{}{}))
	cleaned := must(cleanup.Rewrite(intermediate, struct{}{}))
	stages := []expressions.Expression{f.Input, intermediate, cleaned}
	for i, node := range stages {
		if !node.Type().EqualsTo(f.Input.Type()) {
			t.Fatal("touchstone declared Type changed")
		}
		if i > 0 && reconstruction.MeasureExpression(node).NodeCount > reconstruction.MeasureExpression(stages[i-1]).NodeCount {
			t.Fatal("touchstone grew")
		}
	}
	if expressions.PrettyPrint(f.Input) != before {
		t.Fatal("touchstone input mutated")
	}
	if !must(contextual.Rewrite(f.Input, struct{}{})).EqualsTo(intermediate) {
		t.Fatal("rule reused prior attempt state")
	}
	return stages
}

type touchstoneRow struct {
	article                 string
	review, brand           int64
	tags, categories, malls []int64
	nulls                   [6]bool // article, review, brand, tags, categories, malls; empty arrays are not NULL.
}

func (r touchstoneRow) environment(f *fixture.Touchstone) contextualRow {
	c := f.Columns
	refs := []*expressions.ReferenceExpression{c.ArticleType, c.IsReview, c.BrandID, c.TagIDs, c.CategoryIDs, c.MallIDs}
	var row contextualRow
	for i, ref := range refs {
		var v valueinterfaces.Value
		if r.nulls[i] {
			v = must(testbedvalues.NewNull(ref.Type()))
		} else {
			switch i {
			case 0:
				v = must(testbedvalues.NewVarChar(ref.Type().(*types.VarCharType), r.article))
			case 1, 2:
				v = must(testbedvalues.NewInteger(ref.Type().(*types.IntType), []int64{r.review, r.brand}[i-1]))
			default:
				typ := ref.Type().(*types.ArrayType)
				var elements []valueinterfaces.Value
				for _, n := range [][]int64{r.tags, r.categories, r.malls}[i-3] {
					elements = append(elements, must(testbedvalues.NewInteger(typ.Element().(*types.IntType), n)))
				}
				v = must(testbedvalues.NewArray(typ, elements))
			}
		}
		row = append(row, contextualBinding{ref, v})
	}
	return row
}

// This oracle uses ordinary membership and an explicit three-valued conjunction.
// It applies when article is a concrete value, even under a nullable schema.
func (r touchstoneRow) expectedTruth() foundation.TruthValue {
	if r.nulls[0] {
		panic("nullable article needs a separate counterexample oracle")
	}
	tests := []bool{
		slices.Contains([]string{"tsa_01", "tsa_02", "tsa_03"}, r.article),
		slices.Contains([]int64{8795001, 8795002, 8795003, 8795004, 8795005}, r.review),
		!slices.Contains([]int64{8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011}, r.brand),
		true, !slices.Contains(r.categories, 8792001), !slices.Contains(r.malls, 8793001),
	}
	for _, tag := range r.tags {
		if slices.Contains([]int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}, tag) {
			tests[3] = false
		}
	}
	result := foundation.True
	for i, pass := range tests {
		if r.nulls[i] {
			result = foundation.Null
		} else if !pass {
			return foundation.False
		}
	}
	return result
}

func assertTouchstoneRows(t *testing.T, f *fixture.Touchstone, stages []expressions.Expression, rows []touchstoneRow) {
	t.Helper()
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
	for i, row := range rows {
		want := booleanLiteral(row.expectedTruth(), f.Input.Type().Nullability()).Value()
		environment := row.environment(f)
		for j, node := range stages {
			if got := must(evaluator.Evaluate(node, environment)); !got.EqualsTo(want) {
				t.Fatalf("row %d stage %d: truth/Type differs; row=%+v", i, j, row)
			}
		}
	}
}

func touchstoneRows(nullable bool) []touchstoneRow {
	articles := []string{"tsa_01", "tsa_02", "tsa_03", "tsa_05", "tsa_04", "tsa_06", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15", "unlisted", ""}
	var rows []touchstoneRow
	for _, article := range articles {
		// Baseline, impossible-clause tags, and every individual tag exclusion.
		for _, tags := range [][]int64{nil, {42, 42}, {8791001, 8791002, 8791001}, {8791003}, {8791004}, {8791005}, {8791006}, {8791007}, {8791008}, {8791009}, {8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}} {
			rows = append(rows, touchstoneRow{review: 8795001, article: article, tags: tags})
		}
		for _, review := range []int64{-1, 8795001, 8795002, 8795003, 3, 8795004, 8795005, 1001} {
			rows = append(rows, touchstoneRow{article: article, review: review})
		}
		for _, brand := range []int64{-1, 8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011} {
			rows = append(rows, touchstoneRow{review: 8795001, article: article, brand: brand})
		}
		for _, categories := range [][]int64{nil, {42, 42}, {8792001, 8792001}} {
			for _, malls := range [][]int64{nil, {42, 42}, {8793001, 8793001}} {
				rows = append(rows, touchstoneRow{review: 8795001, article: article, categories: categories, malls: malls})
			}
		}
		if nullable {
			for mask := 1; mask < 32; mask++ {
				row := touchstoneRow{review: 8795001, article: article}
				for i := 1; i < 6; i++ {
					row.nulls[i] = mask&(1<<(i-1)) != 0
				}
				rows = append(rows, row)
				// A concrete failing conjunct must dominate NULLs elsewhere.
				row.tags = []int64{8791004}
				row.categories = []int64{8792001}
				row.malls = []int64{8793001}
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func TestTouchstoneAutomaticScalarReductions(t *testing.T) {
	for _, nullable := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		t.Run(fmt.Sprint(nullable), func(t *testing.T) {
			f := must(fixture.NewTouchstone(foundation.NonNull, nullable))
			stages := rewriteTouchstone(t, f)
			for i, neutral := range []bool{true, false} {
				want := expectedTouchstoneScalar(f, neutral)
				if !stages[i+1].EqualsTo(want) {
					t.Fatalf("stage %d\ngot:\n%s\nwant:\n%s", i+1, expressions.PrettyPrint(stages[i+1]), expressions.PrettyPrint(want))
				}
				if stages[i+1].Children()[0] != f.Input.Children()[0] {
					t.Fatal("article guard occurrence not retained")
				}
			}
			if stages[2].EqualsTo(f.Expected) {
				t.Fatal("scalar result unexpectedly performed array normalization/grouping")
			}
			// Unaffected restrictions retain their original instances; lookup is only an
			// assertion on output, never input supplied to guard/target discovery.
			for j := 2; j <= 5; j++ {
				if stages[1].Children()[j] != f.Input.Children()[j] || stages[2].Children()[j-1] != f.Input.Children()[j] {
					t.Fatal("unaffected restriction replaced")
				}
			}
			rows := touchstoneRows(nullable == foundation.MayNull)
			assertTouchstoneRows(t, f, stages, rows)
			t.Logf("rows=%d; source=%+v; contextual=%+v; cleanup=%+v", len(rows), reconstruction.MeasureExpression(stages[0]), reconstruction.MeasureExpression(stages[1]), reconstruction.MeasureExpression(stages[2]))
		})
	}
}

func TestTouchstoneNullableArticleRemainsEquivalent(t *testing.T) {
	f := must(fixture.NewTouchstone(foundation.MayNull, foundation.MayNull))
	stages := rewriteTouchstone(t, f)
	if stages[1] != f.Input || stages[2] != f.Input {
		t.Fatal("nullable article guard authorized a reduction")
	}
	assertTouchstoneRows(t, f, stages, touchstoneRows(true))
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
	for _, tc := range []struct {
		tags         []int64
		want, unsafe foundation.TruthValue
	}{
		{nil, foundation.Null, foundation.Null},
		{[]int64{8791004}, foundation.Null, foundation.False},
		{[]int64{8791003}, foundation.False, foundation.False},
	} {
		row := touchstoneRow{review: 8795001, tags: tc.tags, nulls: [6]bool{true}}
		for _, node := range stages {
			got := must(evaluator.Evaluate(node, row.environment(f)))
			if !got.EqualsTo(booleanLiteral(tc.want, foundation.MayNull).Value()) {
				t.Fatal("nullable article counterexample changed")
			}
		}
		// Expected is used only to exhibit the known invalid nullable transformation.
		got := must(evaluator.Evaluate(f.Expected, row.environment(f)))
		if !got.EqualsTo(booleanLiteral(tc.unsafe, foundation.MayNull).Value()) {
			t.Fatal("counterexample no longer distinguishes proposed full reduction")
		}
	}
}
