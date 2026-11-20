package interpreter

import (
	"errors"
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ErrEvaluationUnavailable means the interpreter cannot execute an expression.
// It is distinct from missing reference input and from semantic/provider errors.
var ErrEvaluationUnavailable = errors.New("expression evaluation unavailable")

// NewExpressionInterpreter retains the factory for synthesized results. It
// evaluates literals, references, calls and logical operators and preserves opaque regions in partial mode.
// Use NewExpressionInterpreterWithFunctionContext to also evaluate scalar IN.
// Imported graphs must already be valid and immutable. Evaluation does not
// invoke rewrite rules or recursively inspect composite literal payloads as nodes.
func NewExpressionInterpreter(factory valueinterfaces.ValueFactory) *ExpressionInterpreter {
	assertions.NotNil(factory)
	return &ExpressionInterpreter{factory: factory}
}

// ExpressionInterpreter implements complete and partial evaluation independently of
// rewrite rules. NewExpressionInterpreter implements leaf, call and logical evaluation; subsequent
// scalar IN additionally needs a configured equality provider.
// Inputs must be valid, immutable and non-nil. Calls use their retained function
// contexts; environments provide input bindings, not function implementations.
// Callers can establish graph validity with expressions/utils.ValidateForOptimization
// at entry, or controlled construction preserving identity/Type consistency.
// Interpretation does not validate the whole graph; runtime checks still apply.
//
// Both operations preserve the expression's exact effective Type. Errors
// are never represented by NULL or an absent result. Execution is synchronous.
// An error makes the returned result unusable.
// Construct with a non-nil ValueFactory; the zero value is invalid.
// The factory constructs interpreter-owned results; provider results stay owned
// by their provider and are not reconstructed.
type ExpressionInterpreter struct {
	factory         valueinterfaces.ValueFactory
	equalityContext functioninterfaces.FunctionContext
}

// NewExpressionInterpreterWithFunctionContext supplies the equality provider for
// scalar IN. Calls still use their own retained contexts. The provider must be
// non-nil and keep its meanings stable for the interpreter's lifetime.
func NewExpressionInterpreterWithFunctionContext(factory valueinterfaces.ValueFactory, provider functioninterfaces.FunctionContext) *ExpressionInterpreter {
	assertions.NotNil(factory)
	assertions.NotNil(provider)
	return &ExpressionInterpreter{factory: factory, equalityContext: provider}
}

// Evaluate returns a known Value or an error when required input or execution
// is unavailable. A successful typed NULL is a Value, not a nil result.
func (i *ExpressionInterpreter) Evaluate(expression expressions.Expression, environment interfaces.Environment) (valueinterfaces.Value, error) {
	result, err := i.evaluate(expression, environment, false)
	if err != nil {
		return nil, err
	}
	value, _ := result.Known()
	return value, nil
}

// PartialEvaluate retains unavailable inputs as residual expressions. Provider
// errors and invalid bindings remain errors.
func (i *ExpressionInterpreter) PartialEvaluate(expression expressions.Expression, environment interfaces.Environment) (Result, error) {
	return i.evaluate(expression, environment, true)
}

func (i *ExpressionInterpreter) evaluate(expression expressions.Expression, environment interfaces.Environment, partial bool) (Result, error) {
	assertions.NotNilPointer(i)
	assertions.NotNil(i.factory)
	assertions.NotNil(expression)
	assertions.NotNil(environment)
	result, err := i.evaluateNode(expression, environment, partial)
	return result, err
}

func (i *ExpressionInterpreter) evaluateNode(expression expressions.Expression, environment interfaces.Environment, partial bool) (Result, error) {
	switch node := expression.(type) {
	case *expressions.LiteralExpression:
		return NewKnownResult(node.Type(), node.Value())
	case *expressions.ReferenceExpression:
		resolved, err := ResolveReference(environment, node)
		if err != nil {
			return Result{}, err
		}
		if value, present := resolved.Get(); present {
			return NewKnownResult(node.Type(), value)
		}
		if !partial {
			return Result{}, fmt.Errorf("%w: reference could not be resolved", ErrUnavailableInput)
		}
		return NewResidualResult(node.Type(), node)
	case *expressions.SpecialFormExpression:
		return i.evaluateSpecialForm(node, environment, partial)
	case *expressions.LogicalExpression:
		return i.evaluateLogical(node, environment, partial)
	case *expressions.CallExpression:
		return i.evaluateCall(node, environment, partial)
	case *expressions.OpaqueExpression:
		if !partial {
			return Result{}, fmt.Errorf("%w: opaque expression", ErrEvaluationUnavailable)
		}
		return NewResidualResult(node.Type(), node)
	default:
		// Preserve unsupported forms without inspecting children or guessing
		// evaluation order for future forms.
		if !partial {
			return Result{}, fmt.Errorf("%w: %T", ErrEvaluationUnavailable, expression)
		}
		return NewResidualResult(expression.Type(), expression)
	}
}
