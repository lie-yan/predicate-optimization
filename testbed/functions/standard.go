package functions

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/testbed/functions/internal/evaluators"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// NewStandardContext creates an independent testbed catalog and its evaluators.
// It currently supports Boolean NOT/equality and all six Int64/Float32/Float64/unbounded-VARCHAR
// comparisons, plus Int64 array_contains and array_contains_any from the
// semantic fixtures, plus checked Int64 arithmetic. Other signatures remain unsupported.
// Evaluators are deterministic, stateless and safe for concurrent calls.
func NewStandardContext() (*Context, error) {
	boolean, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		return nil, err
	}
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		return nil, err
	}
	text, err := types.NewVarCharType(nil, foundation.NonNull)
	if err != nil {
		return nil, err
	}

	var definitions []Definition
	var callbacks []Evaluator
	add := func(name string, arguments []types.Type, result types.Type, builtin foundation.Optional[shared.BuiltInFunctionName], operator foundation.Optional[foundation.OperatorType], policies []shared.NullPolicy, evaluate Evaluator) error {
		signature, err := shared.NewFunctionSignature(arguments, result)
		if err != nil {
			return err
		}
		if policies == nil {
			policies = make([]shared.NullPolicy, len(arguments))
			for i := range policies {
				policies[i] = shared.PropagateNull
			}
		}
		metadata, err := shared.NewFunctionMetadata(signature, name, builtin, operator, policies)
		if err != nil {
			return err
		}
		definitions = append(definitions, Definition{Namespace: "testbed.standard", Name: name, Metadata: metadata})
		callbacks = append(callbacks, evaluate)
		return nil
	}
	if err := add("not", []types.Type{boolean}, boolean, foundation.Some(shared.BuiltInNot), foundation.Optional[foundation.OperatorType]{}, nil, func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
		return testbedvalues.NewBool(boolean, !args[0].(valueinterfaces.BoolValue).Bool())
	}); err != nil {
		return nil, err
	}

	if err := add(string(shared.BuiltInBoolEqual), []types.Type{boolean, boolean}, boolean, foundation.Some(shared.BuiltInBoolEqual), foundation.Some(foundation.Equal), nil,
		func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
			return testbedvalues.NewBool(boolean, args[0].(valueinterfaces.BoolValue).Bool() == args[1].(valueinterfaces.BoolValue).Bool())
		}); err != nil {
		return nil, err
	}

	for _, operation := range []struct {
		operator    foundation.OperatorType
		integerName shared.BuiltInFunctionName
		textName    shared.BuiltInFunctionName
	}{
		{foundation.Equal, shared.BuiltInInt64Equal, shared.BuiltInVarCharEqual},
		{foundation.NotEqual, shared.BuiltInInt64NotEqual, shared.BuiltInVarCharNotEqual},
		{foundation.LessThan, shared.BuiltInInt64LessThan, shared.BuiltInVarCharLessThan},
		{foundation.LessThanOrEqual, shared.BuiltInInt64LessThanOrEqual, shared.BuiltInVarCharLessThanOrEqual},
		{foundation.GreaterThan, shared.BuiltInInt64GreaterThan, shared.BuiltInVarCharGreaterThan},
		{foundation.GreaterThanOrEqual, shared.BuiltInInt64GreaterThanOrEqual, shared.BuiltInVarCharGreaterThanOrEqual},
	} {
		for _, operand := range []types.Type{integer, text} {
			evaluate := comparisonEvaluator(operation.operator, boolean)
			name := operation.integerName
			if operand == text {
				name = operation.textName
			}
			if err := add(string(name), []types.Type{operand, operand}, boolean, foundation.Some(name), foundation.Some(operation.operator), nil, evaluate); err != nil {
				return nil, err
			}
		}
	}
	float32, err := types.NewFloatType(types.Float32, foundation.NonNull)
	if err != nil {
		return nil, err
	}
	float64, err := types.NewFloatType(types.Float64, foundation.NonNull)
	if err != nil {
		return nil, err
	}
	for _, operation := range []struct {
		operator       foundation.OperatorType
		name32, name64 shared.BuiltInFunctionName
	}{
		{foundation.Equal, shared.BuiltInFloat32Equal, shared.BuiltInFloat64Equal},
		{foundation.NotEqual, shared.BuiltInFloat32NotEqual, shared.BuiltInFloat64NotEqual},
		{foundation.LessThan, shared.BuiltInFloat32LessThan, shared.BuiltInFloat64LessThan},
		{foundation.LessThanOrEqual, shared.BuiltInFloat32LessThanOrEqual, shared.BuiltInFloat64LessThanOrEqual},
		{foundation.GreaterThan, shared.BuiltInFloat32GreaterThan, shared.BuiltInFloat64GreaterThan},
		{foundation.GreaterThanOrEqual, shared.BuiltInFloat32GreaterThanOrEqual, shared.BuiltInFloat64GreaterThanOrEqual},
	} {
		for _, entry := range []struct {
			typ  *types.FloatType
			name shared.BuiltInFunctionName
		}{{float32, operation.name32}, {float64, operation.name64}} {
			if err := add(string(entry.name), []types.Type{entry.typ, entry.typ}, boolean, foundation.Some(entry.name), foundation.Some(operation.operator), nil, evaluators.FloatComparison(operation.operator, boolean)); err != nil {
				return nil, err
			}
		}
	}
	array, err := types.NewArrayType(integer, foundation.NonNull)
	if err != nil {
		return nil, err
	}
	for _, membership := range []struct {
		name      shared.BuiltInFunctionName
		candidate types.Type
		evaluate  Evaluator
	}{
		{shared.BuiltInArrayContains, integer, evaluators.ArrayContains(boolean)},
		{shared.BuiltInArrayContainsAny, array, evaluators.ArrayContainsAny(boolean)},
	} {
		if err := add(string(membership.name), []types.Type{array, membership.candidate}, boolean, foundation.Some(membership.name), foundation.Optional[foundation.OperatorType]{}, []shared.NullPolicy{shared.PropagateNull, shared.NullPolicyUnspecified}, membership.evaluate); err != nil {
			return nil, err
		}
	}
	for _, operation := range []struct {
		name     string
		operator foundation.OperatorType
	}{
		{string(shared.BuiltInInt64Negate), foundation.Negate},
		{string(shared.BuiltInInt64Add), foundation.Add},
		{string(shared.BuiltInInt64Subtract), foundation.Subtract},
		{string(shared.BuiltInInt64Multiply), foundation.Multiply},
		{string(shared.BuiltInInt64Divide), foundation.Divide},
		{string(shared.BuiltInInt64Modulus), foundation.Modulus},
	} {
		arguments := []types.Type{integer, integer}
		if operation.operator == foundation.Negate {
			arguments = arguments[:1]
		}
		if err := add(operation.name, arguments, integer, foundation.Some(shared.BuiltInFunctionName(operation.name)), foundation.Some(operation.operator), nil, evaluators.IntegerArithmetic(operation.operator, integer)); err != nil {
			return nil, err
		}
	}
	catalog, err := NewCatalog(definitions)
	if err != nil {
		return nil, err
	}
	handles := catalog.Handles()
	bindings := make([]EvaluationBinding, len(handles))
	for i, handle := range handles {
		bindings[i] = EvaluationBinding{Handle: handle, Evaluate: callbacks[i]}
	}
	return NewContextWithEvaluators(catalog, bindings)
}

func comparisonEvaluator(operator foundation.OperatorType, resultType *types.BoolType) Evaluator {
	return func(arguments []valueinterfaces.Value) (valueinterfaces.Value, error) {
		var order int
		switch arguments[0].Type().(type) {
		case *types.IntType:
			// Compare directly rather than subtracting, which could overflow.
			order = cmp.Compare(arguments[0].(valueinterfaces.IntegerValue).Integer(), arguments[1].(valueinterfaces.IntegerValue).Integer())
		case *types.VarCharType:
			order = strings.Compare(arguments[0].(valueinterfaces.VarCharValue).VarChar(), arguments[1].(valueinterfaces.VarCharValue).VarChar())
		default:
			return nil, fmt.Errorf("%w: unsupported comparison operand", shared.ErrContractViolation)
		}
		var result bool
		switch operator {
		case foundation.Equal:
			result = order == 0
		case foundation.NotEqual:
			result = order != 0
		case foundation.LessThan:
			result = order < 0
		case foundation.LessThanOrEqual:
			result = order <= 0
		case foundation.GreaterThan:
			result = order > 0
		case foundation.GreaterThanOrEqual:
			result = order >= 0
		default:
			return nil, fmt.Errorf("%w: unsupported comparison operator", shared.ErrContractViolation)
		}
		return testbedvalues.NewBool(resultType, result)
	}
}
