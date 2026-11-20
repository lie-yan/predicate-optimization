package interpreter

import (
	"errors"
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Arguments are processed in stored order before invoking the function context.
// A known null does not skip sibling computations. This is the prototype's
// conservative policy, not a claim about every Milvus execution path.
func (i *ExpressionInterpreter) evaluateCall(call *expressions.CallExpression, environment interfaces.Environment, partial bool) (Result, error) {
	arguments := call.Arguments()
	actual := make([]valueinterfaces.Value, len(arguments))
	allKnown := true
	for index, argument := range arguments {
		result, err := i.evaluate(argument, environment, partial)
		if err != nil {
			return Result{}, fmt.Errorf("call argument %d: %w", index, err)
		}
		if value, known := result.Known(); known {
			actual[index] = value
			if partial {
				if _, literal := argument.(*expressions.LiteralExpression); !literal {
					folded, err := expressions.NewLiteralExpression(value)
					if err != nil {
						return Result{}, err
					}
					arguments[index] = folded
				}
			}
		} else {
			allKnown = false
			arguments[index], _ = result.Residual()
		}
	}
	if allKnown {
		value, err := call.Context().Evaluate(call.Handle(), call.Signature(), actual)
		if err == nil {
			assertions.NotNil(value)
			if !value.Type().EqualsTo(call.Type()) {
				return Result{}, fmt.Errorf("%w: function returned incorrect effective type", functions.ErrContractViolation)
			}
			return NewKnownResult(call.Type(), value)
		}
		if !retainUnavailableCall(err) {
			return Result{}, err
		}
		if !partial {
			return Result{}, fmt.Errorf("%w: %w", ErrEvaluationUnavailable, err)
		}
	}
	rebuilt, err := call.WithChildren(arguments)
	if err != nil {
		return Result{}, err
	}
	return NewResidualResult(call.Type(), rebuilt)
}

// Contract/semantic errors take precedence even if a provider joins
// them with an availability category.
func retainUnavailableCall(err error) bool {
	for _, failure := range []error{functions.ErrContractViolation, functions.ErrEvaluationFailure, functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrUnsupportedLookup} {
		if errors.Is(err, failure) {
			return false
		}
	}
	return errors.Is(err, functions.ErrEvaluationUnavailable) || errors.Is(err, functions.ErrEvaluationDisallowed)
}
