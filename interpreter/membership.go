package interpreter

import (
	"errors"
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Candidate admission precedes evaluation: unsupported lists remain whole and
// are never silently enabled by folding a dynamic/nullable candidate first.
func (i *ExpressionInterpreter) evaluateMembership(node *expressions.SpecialFormExpression, environment interfaces.Environment, partial bool) (Result, error) {
	unavailable := func(cause error) (Result, error) {
		if partial {
			return NewResidualResult(node.Type(), node)
		}
		return Result{}, fmt.Errorf("%w: %w", ErrEvaluationUnavailable, cause)
	}
	args := node.Arguments()
	if len(args) < 2 {
		return unavailable(fmt.Errorf("IN requires an admitted nonempty candidate list"))
	}
	candidates := make([]valueinterfaces.Value, len(args)-1)
	for index, arg := range args[1:] {
		literal, ok := arg.(*expressions.LiteralExpression)
		if !ok || literal.Type().Nullability() != foundation.NonNull || literal.Value().IsNull() {
			return unavailable(fmt.Errorf("IN candidate %d must be a non-null literal with non-nullable Type", index))
		}
		candidates[index] = literal.Value()
	}
	if i.equalityContext == nil {
		return unavailable(fmt.Errorf("IN equality provider is not configured"))
	}
	target, err := i.evaluate(args[0], environment, partial)
	if err != nil {
		return Result{}, err
	}
	value, known := target.Known()
	if !known {
		args[0], _ = target.Residual()
		rebuilt, err := node.WithChildren(args)
		if err != nil {
			return Result{}, err
		}
		return NewResidualResult(node.Type(), rebuilt)
	}
	matched, hasNull := false, false
	for _, candidate := range candidates {
		equal, err := EvaluateMembershipEquality(i.equalityContext, value, candidate)
		if err != nil {
			if membershipUnavailable(err) {
				return unavailable(err)
			}
			return Result{}, err
		}
		if equal.IsNull() {
			hasNull = true
		} else if equal.(valueinterfaces.BoolValue).Bool() {
			matched = true
		}
	}
	// No comparison is skipped after a match: provider failures remain visible.
	var result valueinterfaces.Value
	if !matched && hasNull {
		result, err = i.factory.NewNull(node.Type())
	} else {
		result, err = i.factory.NewBool(node.Type().(*types.BoolType), matched)
	}
	if err != nil {
		return Result{}, err
	}
	return NewKnownResult(node.Type(), result)
}

func membershipUnavailable(err error) bool {
	if errors.Is(err, functions.ErrContractViolation) || errors.Is(err, functions.ErrEvaluationFailure) ||
		errors.Is(err, functions.ErrUnknownHandle) || errors.Is(err, functions.ErrIncompatibleHandle) {
		return false
	}
	return errors.Is(err, functions.ErrUnsupportedLookup) || retainUnavailableCall(err)
}
