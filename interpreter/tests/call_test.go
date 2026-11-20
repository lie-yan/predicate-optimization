package interpreter_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func intLiteral(n int64, nullable foundation.Nullability) *expressions.LiteralExpression {
	typ := must(types.NewIntType(types.Int64, nullable))
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(typ, n))))
}

func arithmetic(provider functioninterfaces.FunctionContext, op foundation.OperatorType, args ...expressions.Expression) *expressions.CallExpression {
	typesOf := make([]types.Type, len(args))
	for i, arg := range args {
		typesOf[i] = arg.Type()
	}
	return must(expressions.NewCallExpression(provider, must(provider.ArithmeticFunction(op, typesOf)), args))
}

type callProvider struct {
	functioninterfaces.FunctionContext
	calls    int
	err      error
	result   valueinterfaces.Value
	override bool
}

func (p *callProvider) Evaluate(h functioninterfaces.FunctionHandle, s *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	p.calls++

	if p.override {
		return p.result, p.err
	}
	return p.FunctionContext.Evaluate(h, s, args)
}

func TestCallKnownAndNullResults(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	one, two := intLiteral(1, foundation.NonNull), intLiteral(2, foundation.MayNull)
	inner := arithmetic(provider, foundation.Add, one, two)
	outer := arithmetic(provider, foundation.Multiply, inner, two)
	for _, partial := range []bool{false, true} {
		var value valueinterfaces.Value
		if partial {
			value, _ = must(evaluator.PartialEvaluate(outer, &environment{})).Known()
		} else {
			value = must(evaluator.Evaluate(outer, &environment{}))
		}
		if value.(*testbedvalues.IntegerValue).Integer() != 6 || !value.Type().EqualsTo(outer.Type()) {
			t.Fatal("nested arithmetic or nullable result wrong")
		}
	}
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(two.Type()))))
	got := must(evaluator.Evaluate(arithmetic(provider, foundation.Add, null, one), &environment{}))
	if !got.IsNull() || !got.Type().EqualsTo(two.Type()) {
		t.Fatal("NULL propagation/type lost")
	}
}

func TestPartialCallFoldsKnownArguments(t *testing.T) {
	provider := &callProvider{FunctionContext: must(testbed.NewStandardContext())}
	one, two := intLiteral(1, foundation.NonNull), intLiteral(2, foundation.NonNull)
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), one.Type(), "x"))
	folded := arithmetic(provider, foundation.Add, one, two)
	root := arithmetic(provider, foundation.Multiply, ref, folded)
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	result := must(evaluator.PartialEvaluate(root, &environment{}))
	residual, _ := result.Residual()
	call := residual.(*expressions.CallExpression)
	if call == root || call.Arguments()[0] != ref || provider.calls != 1 {
		t.Fatal("residual sharing or evaluation boundary wrong")
	}
	if call.Arguments()[1].(*expressions.LiteralExpression).Value().(*testbedvalues.IntegerValue).Integer() != 3 || !call.Type().EqualsTo(root.Type()) || !call.Handle().EqualsTo(root.Handle()) {
		t.Fatal("residual call contract lost")
	}
	if root.Arguments()[1] != folded {
		t.Fatal("original mutated")
	}
	if _, err := evaluator.Evaluate(root, &environment{}); !errors.Is(err, interpreter.ErrUnavailableInput) {
		t.Fatal(err)
	}
}

func TestUnavailableOverflowAndFailures(t *testing.T) {
	standard := must(testbed.NewStandardContext())
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	one, zero := intLiteral(1, foundation.NonNull), intLiteral(0, foundation.NonNull)
	overflow := arithmetic(standard, foundation.Add, intLiteral(math.MaxInt64, foundation.NonNull), one)
	if residual, _ := must(evaluator.PartialEvaluate(overflow, &environment{})).Residual(); residual != overflow {
		t.Fatal("overflow call not preserved")
	}
	if _, err := evaluator.Evaluate(overflow, &environment{}); !errors.Is(err, functions.ErrEvaluationUnavailable) || !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
		t.Fatal(err)
	}
	divide := arithmetic(standard, foundation.Divide, one, zero)
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(must(types.NewIntType(types.Int64, foundation.MayNull))))))
	for _, root := range []expressions.Expression{divide, arithmetic(standard, foundation.Add, null, divide), arithmetic(standard, foundation.Add, divide, null)} {
		if result, err := evaluator.PartialEvaluate(root, &environment{}); result.IsValid() || !errors.Is(err, functions.ErrEvaluationFailure) {
			t.Fatalf("failure suppressed: %v", err)
		}
		if value, err := evaluator.Evaluate(root, &environment{}); value != nil || !errors.Is(err, functions.ErrEvaluationFailure) {
			t.Fatal(err)
		}
	}
}

func TestCallProviderPoliciesAndContracts(t *testing.T) {
	standard := must(testbed.NewStandardContext())
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	one := intLiteral(1, foundation.NonNull)
	for _, category := range []error{functions.ErrEvaluationUnavailable, functions.ErrEvaluationDisallowed} {
		provider := &callProvider{FunctionContext: standard, override: true, err: category}
		call := arithmetic(provider, foundation.Add, one, one)
		if residual, _ := must(evaluator.PartialEvaluate(call, &environment{})).Residual(); residual != call {
			t.Fatal("unavailable call changed")
		}
		if _, err := evaluator.Evaluate(call, &environment{}); !errors.Is(err, category) || !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
			t.Fatal(err)
		}
	}
	for _, category := range []error{functions.ErrContractViolation, functions.ErrEvaluationFailure, functions.ErrUnknownHandle, errors.Join(functions.ErrEvaluationUnavailable, functions.ErrEvaluationFailure)} {
		provider := &callProvider{FunctionContext: standard, override: true, err: category}
		if result, err := evaluator.PartialEvaluate(arithmetic(provider, foundation.Add, one, one), &environment{}); result.IsValid() || !errors.Is(err, category) {
			t.Fatalf("provider error hidden: %v", err)
		}
	}
	wrong := must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true))
	for _, value := range []valueinterfaces.Value{nil, (*testbedvalues.IntegerValue)(nil)} {
		provider := &callProvider{FunctionContext: standard, override: true, result: value}
		expectNilAssertion(t, func() {
			_, _ = evaluator.PartialEvaluate(arithmetic(provider, foundation.Add, one, one), &environment{})
		})
	}
	for _, value := range []valueinterfaces.Value{wrong, intLiteral(1, foundation.MayNull).Value()} {
		provider := &callProvider{FunctionContext: standard, override: true, result: value}
		if _, err := evaluator.PartialEvaluate(arithmetic(provider, foundation.Add, one, one), &environment{}); !errors.Is(err, functions.ErrContractViolation) {
			t.Fatal("bad provider result accepted", err)
		}
	}
}

func TestMetadataOnlyZeroArgumentCall(t *testing.T) {
	typ := must(types.NewBoolType(foundation.NonNull))
	signature := must(functions.NewFunctionSignature(nil, typ))
	metadata := must(functions.NewFunctionMetadata(signature, "constant", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, nil))
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "test", Name: "constant", Metadata: metadata}}))
	provider := testbed.NewContext(catalog)
	call := must(expressions.NewCallExpression(provider, catalog.Handles()[0], nil))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	if residual, _ := must(evaluator.PartialEvaluate(call, &environment{})).Residual(); residual != call {
		t.Fatal("metadata-only call not preserved")
	}
	if _, err := evaluator.Evaluate(call, &environment{}); !errors.Is(err, functions.ErrEvaluationUnavailable) {
		t.Fatal(err)
	}
}
