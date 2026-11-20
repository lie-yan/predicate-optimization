package interpreter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestLiteralAndReferenceEvaluation(t *testing.T) {
	typ := must(types.NewIntType(types.Int64, foundation.MayNull))
	scalar := must(testbedvalues.NewInteger(typ, 8))
	null := must(testbedvalues.NewNull(typ))
	innerType := must(types.NewArrayType(typ, foundation.NonNull))
	inner := must(testbedvalues.NewArray(innerType, []valueinterfaces.Value{scalar, null}))
	outerType := must(types.NewArrayType(innerType, foundation.MayNull))
	nested := must(testbedvalues.NewArray(outerType, []valueinterfaces.Value{inner, inner}))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, value := range []valueinterfaces.Value{scalar, null, nested} {
		literal := must(expressions.NewLiteralExpression(value))
		id := must(ids.NewColumnID("table", "x"))
		reference := must(expressions.NewReferenceExpression(id, value.Type(), "x"))
		env := &environment{bindings: []binding{{id, value}}}
		for _, node := range []expressions.Expression{literal, reference} {
			if got := must(evaluator.Evaluate(node, env)); got != value {
				t.Fatal("immutable value not shared")
			}
			result := must(evaluator.PartialEvaluate(node, env))
			got, known := result.Known()
			if !known || got != value || !result.Type().EqualsTo(node.Type()) {
				t.Fatal("known result or exact Type lost")
			}
			folded := must(expressions.NewLiteralExpression(got))
			if !folded.Type().EqualsTo(node.Type()) {
				t.Fatal("literal wrapping changed Type")
			}
		}
		if env.calls != 2 {
			t.Fatal("literal evaluation queried bindings")
		}
	}
	// Accessor slices remain caller-owned after evaluation.
	result := must(evaluator.Evaluate(must(expressions.NewLiteralExpression(nested)), &environment{})).(*testbedvalues.ArrayValue)
	elements := result.Elements()
	elements[0] = must(testbedvalues.NewArray(innerType, nil))
	if nested.Elements()[0] != inner || result.Elements()[0] != inner {
		t.Fatal("nested payload mutated through accessor")
	}
}

func TestUnavailableLeaves(t *testing.T) {
	typ := must(types.NewBoolType(foundation.NonNull))
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), typ, "x"))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("adapter", "region")), typ, "unsupported"))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	env := &environment{}
	for _, tc := range []struct {
		node expressions.Expression
		want error
	}{
		{ref, interpreter.ErrUnavailableInput},
		{opaque, interpreter.ErrEvaluationUnavailable},
	} {
		result := must(evaluator.PartialEvaluate(tc.node, env))
		residual, ok := result.Residual()
		if !ok || residual != tc.node || !result.Type().EqualsTo(typ) {
			t.Fatal("unavailable input became known or changed")
		}
		if got, err := evaluator.Evaluate(tc.node, env); got != nil || !errors.Is(err, tc.want) {
			t.Fatalf("wrong complete-mode outcome: %v", err)
		}
	}
	if env.calls != 2 {
		t.Fatal("opaque region queried bindings")
	}
}

func TestReferenceScopesAndErrors(t *testing.T) {
	typ := must(types.NewBoolType(foundation.NonNull))
	a, b := must(ids.NewColumnID("a", "x")), must(ids.NewColumnID("b", "x"))
	yes, no := must(testbedvalues.NewBool(typ, true)), must(testbedvalues.NewBool(typ, false))
	env := &environment{bindings: []binding{{a, yes}, {b, no}}}
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, entry := range env.bindings {
		ref := must(expressions.NewReferenceExpression(entry.id, typ, "same display"))
		if got := must(evaluator.Evaluate(ref, env)); got != entry.value {
			t.Fatal("scopes conflated")
		}
	}
	ref := must(expressions.NewReferenceExpression(a, typ, "x"))
	for _, tc := range []struct {
		value                    valueinterfaces.Value
		providerError, errorWant error
	}{
		{must(testbedvalues.NewBool(must(types.NewBoolType(foundation.MayNull)), true)), nil, interpreter.ErrInvalidBinding},
		{yes, errors.New("provider failed"), nil},
	} {
		want := tc.errorWant
		if tc.providerError != nil {
			want = tc.providerError
		}
		env.bindings[0].value, env.err = tc.value, tc.providerError
		if got, err := evaluator.Evaluate(ref, env); got != nil || !errors.Is(err, want) {
			t.Fatal("complete mode lost error", err)
		}
		if got, err := evaluator.PartialEvaluate(ref, env); got.IsValid() || !errors.Is(err, want) {
			t.Fatal("partial mode concealed error", err)
		}
	}
}

func TestUnimplementedFormsRemainUntouched(t *testing.T) {
	typ := must(types.NewBoolType(foundation.NonNull))
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), typ, "x"))
	root := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, typ, []expressions.Expression{ref}))
	env := &environment{err: errors.New("must not query")}
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	result := must(evaluator.PartialEvaluate(root, env))
	if got, _ := result.Residual(); got != root || env.calls != 0 {
		t.Fatal("unsupported form traversed")
	}
	if _, err := evaluator.Evaluate(root, env); !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
		t.Fatal(err)
	}
}
