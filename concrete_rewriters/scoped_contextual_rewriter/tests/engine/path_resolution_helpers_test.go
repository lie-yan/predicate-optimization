package engine_test

import (
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Every sibling refers to the same subtree, so a wide/deep fixture stays small
// while preserving distinct ordered occurrences of shared nodes.
func resolutionLogicalFixture(width, depth int) (expressions.Expression, engine.OccurrencePath, expressions.Expression) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	leaf := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, true))))
	var root expressions.Expression = leaf
	path := make(engine.OccurrencePath, depth)
	for level := 0; level < depth; level++ {
		children := make([]expressions.Expression, width)
		for i := range children {
			children[i] = root
		}
		root = must(expressions.NewLogicalExpression(foundation.And, children))
		path[level] = width - 1
	}
	return root, path, leaf
}

func resolutionMixedFixture() (expressions.Expression, engine.OccurrencePath, expressions.Expression) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	yes := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, true))))
	no := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, false))))
	context := must(testbed.NewStandardContext())
	handle := must(context.ComparisonFunction(foundation.Equal, boolean, boolean))
	call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{yes, no}))
	membership := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{call, yes}))
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{membership, membership, yes}))
	return root, engine.OccurrencePath{1, 0, 1}, no
}

var resolvedExpressionSink expressions.Expression
