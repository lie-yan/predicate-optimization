package expressions

import (
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Private fixtures exercise states public immutable constructors cannot create.
func MalformedNodesForValidationTest() []Expression {
	boolean, _ := types.NewBoolType(foundation.NonNull)
	nullable, _ := types.NewBoolType(foundation.MayNull)
	val, _ := testbedvalues.NewBool(boolean, true)
	literal, _ := NewLiteralExpression(val)
	cycle := &LogicalExpression{operator: foundation.Not, typ: boolean}
	cycle.children = []Expression{cycle}
	malformed := []Expression{
		&LiteralExpression{}, &ReferenceExpression{}, &OpaqueExpression{},
		&CallExpression{}, &LogicalExpression{}, &SpecialFormExpression{}, cycle,
		&LogicalExpression{operator: foundation.Not, typ: boolean, children: []Expression{nil}},
		&LogicalExpression{operator: foundation.Not, typ: nullable, children: []Expression{literal}},
		&SpecialFormExpression{form: foundation.SpecialFormIn, typ: nullable, arguments: []Expression{literal}},
	}
	return malformed
}

type validationHandle struct{}

func (validationHandle) Namespace() string                               { return "test" }
func (validationHandle) Name() string                                    { return "test" }
func (validationHandle) ArgumentTypes() []types.Type                     { return nil }
func (validationHandle) EqualsTo(functioninterfaces.FunctionHandle) bool { return true }

func MalformedCallsForValidationTest() []Expression {
	boolean, _ := types.NewBoolType(foundation.NonNull)
	nullable, _ := types.NewBoolType(foundation.MayNull)
	val, _ := testbedvalues.NewBool(boolean, true)
	literal, _ := NewLiteralExpression(val)
	wrongType, _ := functions.NewFunctionSignature([]types.Type{nullable}, boolean)
	wrongArity, _ := functions.NewFunctionSignature(nil, boolean)
	var malformed []Expression
	for _, signature := range []*functions.FunctionSignature{{}, wrongType, wrongArity} {
		node := &CallExpression{context: struct {
			functioninterfaces.FunctionContext
		}{}, handle: validationHandle{}, signature: signature, arguments: []Expression{literal}}
		malformed = append(malformed, node)
	}
	return malformed
}
