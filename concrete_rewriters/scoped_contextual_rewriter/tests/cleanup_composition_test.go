package scoped_contextual_rewriter_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestContextualThenBooleanCleanup(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, provider))
	cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, provider))
	x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	a := reference("a", must(types.NewBoolType(foundation.MayNull)))
	guard := membership(x, 1, 2)
	truth := booleanLiteral(foundation.True, foundation.NonNull)
	maybeTrue := booleanLiteral(foundation.True, foundation.MayNull)
	impossible := comparison(provider, foundation.Equal, x, 3)
	for _, tc := range []struct {
		name                       string
		source, intermediate, want expressions.Expression
	}{
		{"retire guard path before collapsing root", logical(foundation.And, guard, logical(foundation.Not, impossible)), logical(foundation.And, guard, truth), guard},
		{"retire shifted guard path", logical(foundation.And, truth, guard, logical(foundation.Not, impossible)), logical(foundation.And, truth, guard, truth), guard},
		{"keep conservative nullable result", logical(foundation.And, guard, logical(foundation.Not, logical(foundation.And, a, impossible))), logical(foundation.And, guard, maybeTrue), logical(foundation.And, guard, maybeTrue)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, calls := expressions.PrettyPrint(tc.source), provider.evaluations
			intermediate := must(contextual.Rewrite(tc.source, struct{}{}))
			if !intermediate.EqualsTo(tc.intermediate) {
				t.Fatal("contextual pass performed premature cleanup")
			}
			got := must(cleanup.Rewrite(intermediate, struct{}{}))
			if !got.EqualsTo(tc.want) || !got.Type().EqualsTo(tc.source.Type()) || provider.evaluations != calls || expressions.PrettyPrint(tc.source) != before {
				t.Fatal("incorrect composition or unexpected evaluation")
			}
			if tc.want == guard && got != guard {
				t.Fatal("cleanup did not retain shared guard")
			}
			// A subsequent owner derives new paths and inputs from this new source.
			later := logical(foundation.And, got, logical(foundation.Not, impossible))
			laterResult := must(contextual.Rewrite(later, struct{}{}))
			if !laterResult.EqualsTo(logical(foundation.And, got, truth)) || provider.evaluations != calls {
				t.Fatal("later contextual pass reused retired attempt state")
			}
			oracle := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
			for _, n := range []int64{-1, 0, 1, 2, 3, 4} {
				for _, av := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
					row := contextualRow{{x, integer(n, foundation.NonNull).Value()}, {a, booleanLiteral(av, foundation.MayNull).Value()}}
					original := must(oracle.Evaluate(tc.source, row))
					if !original.EqualsTo(must(oracle.Evaluate(intermediate, row))) || !original.EqualsTo(must(oracle.Evaluate(got, row))) {
						t.Fatal("composed passes changed truth or Type")
					}
				}
			}
		})
	}
}

func TestCleanupRemovesTouchstoneStyleNeutralConjunct(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, provider))
	cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, provider))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	article := reference("article_type", text)
	textLiteral := func(s string) *expressions.LiteralExpression {
		return must(expressions.NewLiteralExpression(must(testbedvalues.NewVarChar(text, s))))
	}
	boolean := must(types.NewBoolType(foundation.NonNull))
	guard := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{article, textLiteral("tsa_01"), textLiteral("tsa_02"), textLiteral("tsa_03")}))
	common := must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(foundation.Equal, text, text)), []expressions.Expression{article, textLiteral("tsa_04")}))
	integerType := must(types.NewIntType(types.Int64, foundation.NonNull))
	arrayType := must(types.NewArrayType(integerType, foundation.NonNull))
	array := func(numbers ...int64) *expressions.LiteralExpression {
		elements := make([]valueinterfaces.Value, len(numbers))
		for i, n := range numbers {
			elements[i] = integer(n, foundation.NonNull).Value()
		}
		return must(expressions.NewLiteralExpression(must(testbedvalues.NewArray(arrayType, elements))))
	}
	tagHandle := must(provider.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{arrayType, arrayType}))
	tagTest := must(expressions.NewCallExpression(provider, tagHandle, []expressions.Expression{array(8791001), array(8791001, 8791002)}))
	review := reference("is_review", integerType)
	reviewFilter := membership(review, 8795001, 8795002, 8795003)
	source := logical(foundation.And, guard, logical(foundation.Not, logical(foundation.And, tagTest, common)), reviewFilter)
	intermediate := must(contextual.Rewrite(source, struct{}{}))
	if !intermediate.EqualsTo(logical(foundation.And, guard, booleanLiteral(foundation.True, foundation.NonNull), reviewFilter)) {
		t.Fatal("expected a retained neutral touchstone-style conjunct")
	}
	got := must(cleanup.Rewrite(intermediate, struct{}{}))
	if !got.EqualsTo(logical(foundation.And, guard, reviewFilter)) || got.Children()[0] != guard || got.Children()[1] != reviewFilter || provider.evaluations != 0 {
		t.Fatal("cleanup lost retained conjuncts or evaluated functions")
	}
	oracle := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	for _, articleType := range []string{"tsa_01", "tsa_02", "tsa_03", "tsa_04", "other"} {
		for _, reviewValue := range []int64{-1, 8795001, 8795003, 9} {
			row := contextualRow{{article, textLiteral(articleType).Value()}, {review, integer(reviewValue, foundation.NonNull).Value()}}
			if !must(oracle.Evaluate(source, row)).EqualsTo(must(oracle.Evaluate(got, row))) {
				t.Fatal("touchstone-style cleanup changed evaluation")
			}
		}
	}
}
