package customvalues_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	grouping "github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	cleanup "github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/examples/customvalues"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func Example_customValueWorkflow() {
	factory := customvalues.Factory{}
	provider := fixtures.Must(testfunctions.NewStandardContext()) // Its outputs use bundled storage.
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x := fixtures.ScalarReference("x", integer)
	one, two := fixtures.Must(factory.NewInteger(integer, 1)), fixtures.Must(factory.NewInteger(integer, 2))
	source := fixtures.ScalarLogical(foundation.And, fixtures.ScalarMembership(x, one, two), fixtures.ScalarComparison(provider, foundation.GreaterThan, x, fixtures.ScalarLiteral(one)))
	mustValid := func(node expressions.Expression) {
		if err := utils.ValidateForOptimization(node); err != nil {
			panic(err)
		}
	}
	mustValid(source)
	analysis := fixtures.Must(fact_extraction.NewScalarExtractor(factory, provider).Extract(source))
	summary, present := analysis.Summary().Get()
	if !present {
		panic("expected exact summary")
	}
	candidate, present := fixtures.Must(reconstruction.NewScalarReconstructor(factory, provider).Reconstruct(summary)).Get()
	if !present {
		panic("expected reconstruction")
	}
	fmt.Println("true set:", summary.TrueSet())
	// Cleanup synthesizes a caller-owned Boolean, and retains the reconstructed call.
	yes := fixtures.ScalarLiteral(fixtures.Must(factory.NewBool(fixtures.Must(types.NewBoolType(foundation.NonNull)), true)))
	wrapped := fixtures.ScalarLogical(foundation.And, candidate, fixtures.ScalarLogical(foundation.Not, fixtures.ScalarLogical(foundation.Not, yes)))
	rewritten := fixtures.Must(rewriter.NewExpressionTreeRewriter[struct{}](cleanup.NewBooleanCleanupRewriter(factory, provider)).Rewrite(wrapped, struct{}{}))
	mustValid(rewritten)
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(factory, provider)
	for _, input := range []valueinterfaces.Value{one, two} {
		env := fixtures.Environment{{ID: x.ID(), Value: input}}
		before, after := fixtures.Must(evaluator.Evaluate(source, env)), fixtures.Must(evaluator.Evaluate(rewritten, env))
		if !before.EqualsTo(after) {
			panic("rewrite changed meaning")
		}
		_, own := before.(customvalues.Value)
		_, bundled := after.(*testbedvalues.BoolValue)
		fmt.Printf("x=%d: %t; factory result=%t, provider result=%t\n", input.(valueinterfaces.IntegerValue).Integer(), after.(valueinterfaces.BoolValue).Bool(), own, bundled)
	}
	// Group mixed child implementations; publication and evaluation read interfaces.
	array := fixtures.Must(types.NewArrayType(integer, foundation.NonNull))
	items := fixtures.ScalarReference("items", array)
	builtinTwo := fixtures.Must(testbedvalues.NewInteger(integer, 2))
	exclude := func(children ...valueinterfaces.Value) expressions.Expression {
		candidates := fixtures.ScalarLiteral(fixtures.Must(factory.NewArray(array, children)))
		handle := fixtures.Must(provider.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{array, array}))
		return fixtures.ScalarLogical(foundation.Not, fixtures.Must(expressions.NewCallExpression(provider, handle, []expressions.Expression{items, candidates})))
	}
	exclusions := fixtures.ScalarLogical(foundation.And, exclude(one, builtinTwo), exclude(two, one))
	grouped := fixtures.Must(rewriter.NewExpressionTreeRewriter[struct{}](grouping.NewArrayExclusionGroupingRewriter(factory)).Rewrite(exclusions, struct{}{}))
	mustValid(grouped)
	output := grouped.Children()[0].(*expressions.CallExpression).Arguments()[1].(*expressions.LiteralExpression).Value()
	_, ownArray := output.(customvalues.Value)
	fmt.Print("grouped candidates:")
	for _, v := range output.(valueinterfaces.ArrayValue).Elements() {
		fmt.Printf(" %d", v.(valueinterfaces.IntegerValue).Integer())
	}
	env := fixtures.Environment{{ID: items.ID(), Value: fixtures.Must(factory.NewArray(array, []valueinterfaces.Value{builtinTwo}))}}
	fmt.Printf("; factory array=%t; equivalent=%t\n", ownArray, fixtures.Must(evaluator.Evaluate(exclusions, env)).EqualsTo(fixtures.Must(evaluator.Evaluate(grouped, env))))
	// Output:
	// true set: {2}
	// x=1: false; factory result=true, provider result=true
	// x=2: true; factory result=true, provider result=true
	// grouped candidates: 1 2 2 1; factory array=true; equivalent=true
}
