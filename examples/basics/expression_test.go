package basics_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/folding_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	interpreterinterfaces "github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	"github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Build x + (1 + 2), retaining the context that owns the resolved handle.
func arithmeticTree() (functioninterfaces.FunctionContext, *expressions.ReferenceExpression, *expressions.CallExpression) {
	provider := fixtures.Must(functions.NewStandardContext())
	typ := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x := fixtures.Must(expressions.NewReferenceExpression(fixtures.Must(ids.NewColumnID("example", "x")), typ, "x"))
	literal := func(n int64) expressions.Expression {
		return fixtures.Must(expressions.NewLiteralExpression(fixtures.Must(testbedvalues.NewInteger(typ, n))))
	}
	handle := fixtures.Must(provider.ArithmeticFunction(foundation.Add, []types.Type{typ, typ}))
	sum := fixtures.Must(expressions.NewCallExpression(provider, handle, []expressions.Expression{literal(1), literal(2)}))
	root := fixtures.Must(expressions.NewCallExpression(provider, handle, []expressions.Expression{x, sum}))
	return provider, x, root
}

func Example_construction() {
	_, x, root := arithmeticTree()
	fmt.Println("valid:", utils.ValidateForOptimization(root) == nil)
	fmt.Println("reference shared:", root.Arguments()[0] == x)
	fmt.Println("arguments:", len(root.Arguments()))
	// Output:
	// valid: true
	// reference shared: true
	// arguments: 2
}

func Example_interpretation() {
	provider, x, root := arithmeticTree()
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	env := fixtures.Environment{{ID: x.ID(), Value: fixtures.Must(testbedvalues.NewInteger(x.Type().(*types.IntType), 7))}}
	value := fixtures.Must(evaluator.Evaluate(root, env))
	fmt.Println(value.(*testbedvalues.IntegerValue).Integer())
	// Output: 10
}

func Example_partialFolding() {
	provider, x, root := arithmeticTree()
	if err := utils.ValidateForOptimization(root); err != nil {
		panic(err)
	}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	// No binding for x: absence means unavailable, not SQL NULL.
	rule := folding_rewriter.NewFoldingRewriter(evaluator, false)
	tree := rewriter.NewExpressionTreeRewriter[interpreterinterfaces.Environment](rule)
	folded := fixtures.Must(tree.Rewrite(root, fixtures.Environment{})).(*expressions.CallExpression)
	fmt.Println("reference retained:", folded.Arguments()[0] == x)
	constant := folded.Arguments()[1].(*expressions.LiteralExpression)
	fmt.Println("folded sum:", constant.Value().(*testbedvalues.IntegerValue).Integer())
	fmt.Println("original preserved:", root.Arguments()[1] != constant)
	// Output:
	// reference retained: true
	// folded sum: 3
	// original preserved: true
}

// Double negation preserves all three truth values and the exact result Type.
type doubleNegationRewriter struct {
	rewriter.BaseExpressionRewriter[struct{}]
}

func (r *doubleNegationRewriter) RewriteLogical(node *expressions.LogicalExpression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	if node.Operator() == foundation.Not {
		if inner, ok := node.Children()[0].(*expressions.LogicalExpression); ok && inner.Operator() == foundation.Not {
			return foundation.Some(inner.Children()[0]), nil
		}
	}
	return foundation.Optional[expressions.Expression]{}, nil
}

func Example_sharedSubtreeRewriting() {
	typ := fixtures.Must(types.NewBoolType(foundation.MayNull))
	ref := fixtures.Must(expressions.NewReferenceExpression(fixtures.Must(ids.NewColumnID("example", "flag")), typ, "flag"))
	not := fixtures.Must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{ref}))
	shared := fixtures.Must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{not}))
	root := fixtures.Must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{shared, shared}))
	tree := rewriter.NewExpressionTreeRewriter[struct{}](&doubleNegationRewriter{})
	rewritten := fixtures.Must(tree.Rewrite(root, struct{}{}))
	fmt.Println("both occurrences rewritten:", rewritten.Children()[0] == ref && rewritten.Children()[1] == ref)
	fmt.Println("original sharing preserved:", root.Children()[0] == shared && root.Children()[1] == shared)
	// Output:
	// both occurrences rewritten: true
	// original sharing preserved: true
}
