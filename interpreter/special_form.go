package interpreter

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
)

// evaluateSpecialForm dispatches to form-specific evaluation semantics.
// Unsupported forms remain intact in partial mode.
func (i *ExpressionInterpreter) evaluateSpecialForm(node *expressions.SpecialFormExpression, environment interfaces.Environment, partial bool) (Result, error) {
	switch node.Form() {
	case foundation.SpecialFormIn:
		return i.evaluateMembership(node, environment, partial)
	default:
		if partial {
			return NewResidualResult(node.Type(), node)
		}
		return Result{}, fmt.Errorf("%w: special form %s", ErrEvaluationUnavailable, node.Form())
	}
}
