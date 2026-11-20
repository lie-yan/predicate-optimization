package interpreter

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Evaluate every operand in stored order before applying the truth table.
// A decisive value never suppresses a child error or discards a residual region.
func (i *ExpressionInterpreter) evaluateLogical(node *expressions.LogicalExpression, environment interfaces.Environment, partial bool) (Result, error) {
	children := node.Children()
	changed := false
	count := len(children)
	truths := make([]foundation.TruthValue, count)
	allKnown := true
	for index, child := range children {
		replacement := child
		result, err := i.evaluate(child, environment, partial)
		if err != nil {
			return Result{}, fmt.Errorf("%s operand %d: %w", node.Operator(), index, err)
		}
		if value, known := result.Known(); known {
			if value.IsNull() {
				truths[index] = foundation.Null
			} else if value.(valueinterfaces.BoolValue).Bool() {
				truths[index] = foundation.True
			} else {
				truths[index] = foundation.False
			}
			if partial {
				if _, literal := child.(*expressions.LiteralExpression); !literal {
					folded, err := expressions.NewLiteralExpression(value)
					if err != nil {
						return Result{}, err
					}
					replacement = folded
				}
			}
		} else {
			allKnown = false
			replacement, _ = result.Residual()
		}
		if replacement != child {
			changed = true
			children[index] = replacement
		}
	}
	if !allKnown {
		if !changed {
			return NewResidualResult(node.Type(), node)
		}
		rebuilt, err := node.WithChildren(children)
		if err != nil {
			return Result{}, err
		}
		return NewResidualResult(node.Type(), rebuilt)
	}
	truth := logicalTruth(node.Operator(), truths)
	var value valueinterfaces.Value
	var err error
	if truth == foundation.Null {
		value, err = i.factory.NewNull(node.Type())
	} else {
		value, err = i.factory.NewBool(node.Type().(*types.BoolType), truth == foundation.True)
	}
	if err != nil {
		return Result{}, err
	}
	return NewKnownResult(node.Type(), value)
}

func logicalTruth(operator foundation.LogicalOperator, children []foundation.TruthValue) foundation.TruthValue {
	if operator == foundation.Not {
		switch children[0] {
		case foundation.True:
			return foundation.False
		case foundation.False:
			return foundation.True
		default:
			return foundation.Null
		}
	}
	identity, decisive := foundation.True, foundation.False
	if operator == foundation.Or {
		identity, decisive = foundation.False, foundation.True
	}
	result := identity
	for _, child := range children {
		if child == decisive {
			return decisive
		}
		if child == foundation.Null {
			result = foundation.Null
		}
	}
	return result
}
