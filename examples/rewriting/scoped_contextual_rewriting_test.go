package rewriting_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Compose public rules without supplying guard paths, premises or proof records.
func Example_scopedContextualRewriting() {
	// The standard provider supplies stable, effect-free, successful supported
	// comparisons. Production providers must satisfy the same complete-domain
	// contract, including null behavior; calls keep their paired contexts.
	functionContext := fixtures.Must(functions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x := fixtures.ScalarReference("x", integer)
	one, two, three := fixtures.Must(testbedvalues.NewInteger(integer, 1)), fixtures.Must(testbedvalues.NewInteger(integer, 2)), fixtures.Must(testbedvalues.NewInteger(integer, 3))
	guard := fixtures.ScalarMembership(x, one, two)
	source := fixtures.ScalarLogical(foundation.And, guard,
		fixtures.ScalarLogical(foundation.Not, fixtures.ScalarComparison(functionContext, foundation.Equal, x, fixtures.ScalarLiteral(three))),
	)

	// Validate once before the composed pipeline; each pass preserves validity.
	if err := utils.ValidateForOptimization(source); err != nil {
		panic(err)
	}

	// Each Rewrite call performs one traversal. The contextual pass keeps its
	// accepted guard; the separately scheduled cleanup pass removes neutral True.
	contextualRule := scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, functionContext)
	contextualTree := rewriter.NewExpressionTreeRewriter[struct{}](contextualRule)
	accepted := fixtures.Must(contextualTree.Rewrite(source, struct{}{}))
	cleanupRule := boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, functionContext)
	cleanupTree := rewriter.NewExpressionTreeRewriter[struct{}](cleanupRule)
	cleaned := fixtures.Must(cleanupTree.Rewrite(accepted, struct{}{}))

	fmt.Println("Input:")
	fmt.Println(expressions.PrettyPrint(source))
	fmt.Println("After contextual rewriting:")
	fmt.Println(expressions.PrettyPrint(accepted))
	fmt.Println("After Boolean cleanup:")
	fmt.Println(expressions.PrettyPrint(cleaned))
	fmt.Println("exact Type preserved:", accepted.Type().EqualsTo(source.Type()) && cleaned.Type().EqualsTo(source.Type()))

	// Interpretation is an independent check, not part of either rewrite pass.
	// Bindings are complete, stable and effect-free; test inside and outside G.
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, functionContext)
	for _, n := range []int64{1, 2, 3, 4} {
		row := fixtures.Environment{{ID: x.ID(), Value: fixtures.Must(testbedvalues.NewInteger(integer, n))}}
		original := fixtures.Must(evaluator.Evaluate(source, row))
		intermediate := fixtures.Must(evaluator.Evaluate(accepted, row))
		result := fixtures.Must(evaluator.Evaluate(cleaned, row))
		fmt.Printf("x=%d: %t, both stages match input: %t\n", n, result.(*testbedvalues.BoolValue).Bool(), original.EqualsTo(intermediate) && original.EqualsTo(result))
	}
	// Output:
	// Input:
	// LogicalExpression AND : Bool!
	// ├ operand[0]: SpecialFormExpression IN : Bool!
	// │ ├ target: ReferenceExpression name="x" id="ColumnID(scope=\"example\", symbol=\"x\")" : Int64!
	// │ ├ candidate[0]: LiteralExpression value=1 : Int64!
	// │ └ candidate[1]: LiteralExpression value=2 : Int64!
	// └ operand[1]: LogicalExpression NOT : Bool!
	//   └ operand[0]: CallExpression function="testbed.standard"."int64.equal" : Bool!
	//     ├ arg[0]: ReferenceExpression name="x" id="ColumnID(scope=\"example\", symbol=\"x\")" : Int64!
	//     └ arg[1]: LiteralExpression value=3 : Int64!
	// After contextual rewriting:
	// LogicalExpression AND : Bool!
	// ├ operand[0]: SpecialFormExpression IN : Bool!
	// │ ├ target: ReferenceExpression name="x" id="ColumnID(scope=\"example\", symbol=\"x\")" : Int64!
	// │ ├ candidate[0]: LiteralExpression value=1 : Int64!
	// │ └ candidate[1]: LiteralExpression value=2 : Int64!
	// └ operand[1]: LiteralExpression value=true : Bool!
	// After Boolean cleanup:
	// SpecialFormExpression IN : Bool!
	// ├ target: ReferenceExpression name="x" id="ColumnID(scope=\"example\", symbol=\"x\")" : Int64!
	// ├ candidate[0]: LiteralExpression value=1 : Int64!
	// └ candidate[1]: LiteralExpression value=2 : Int64!
	// exact Type preserved: true
	// x=1: true, both stages match input: true
	// x=2: true, both stages match input: true
	// x=3: false, both stages match input: true
	// x=4: false, both stages match input: true
}
