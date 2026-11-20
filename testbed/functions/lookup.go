package functions

import (
	"fmt"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

// LookupBuiltInFunction resolves by standard metadata identity, not display text
// or caller-specific handle names. Execution availability does not affect lookup.
func (c *Context) LookupBuiltInFunction(name shared.BuiltInFunctionName, argumentTypes []types.Type) (functioninterfaces.FunctionHandle, error) {
	c.assertLookupInputs(argumentTypes)
	return c.lookup(argumentTypes, c.catalog.byBuiltIn[name])
}

// ArithmeticFunction validates family and arity, then binds argument types.
func (c *Context) ArithmeticFunction(operator foundation.OperatorType, argumentTypes []types.Type) (functioninterfaces.FunctionHandle, error) {
	c.assertLookupInputs(argumentTypes)
	arity := 2
	if operator == foundation.Negate {
		arity = 1
	}
	if !operator.IsArithmeticOperator() || len(argumentTypes) != arity {
		return nil, fmt.Errorf("%w: invalid arithmetic operator or arity", shared.ErrUnsupportedLookup)
	}
	return c.lookup(argumentTypes, c.catalog.byOperator[operator])
}

// ComparisonFunction resolves a binary comparison for the operand types.
func (c *Context) ComparisonFunction(operator foundation.OperatorType, leftType, rightType types.Type) (functioninterfaces.FunctionHandle, error) {
	arguments := []types.Type{leftType, rightType}
	c.assertLookupInputs(arguments)
	if !operator.IsComparisonOperator() {
		return nil, fmt.Errorf("%w: invalid comparison operator", shared.ErrUnsupportedLookup)
	}
	return c.lookup(arguments, c.catalog.byOperator[operator])
}

// Only the indexed candidates are checked using semantic binding. Inputs have
// already been checked by the public lookup methods. Ambiguous matches fail
// explicitly rather than selecting behavior by catalog insertion order.
func (c *Context) lookup(arguments []types.Type, candidates []*entry) (functioninterfaces.FunctionHandle, error) {
	var selected *entry
	for _, definition := range candidates {
		check := shared.ProbeFunctionBinding(definition.metadata, arguments)
		if !check.Supported() {
			if check.Is(shared.ErrUnsupportedLookup) {
				continue
			}
			return nil, fmt.Errorf("%w: invalid catalog binding: %w", shared.ErrContractViolation, check.Err())
		}
		if selected != nil {
			return nil, fmt.Errorf("%w: ambiguous catalog bindings", shared.ErrUnsupportedLookup)
		}
		selected = definition
	}
	if selected == nil {
		return nil, fmt.Errorf("%w: no matching catalog binding", shared.ErrUnsupportedLookup)
	}
	return &Handle{identity: c.catalog.identity, entry: selected}, nil
}

func (c *Context) assertLookupInputs(arguments []types.Type) {
	c.assertValid()
	for _, argument := range arguments {
		assertions.NotNil(argument)
	}
}
