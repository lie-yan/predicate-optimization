package interpreter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func truthLiteral(truth foundation.TruthValue, n foundation.Nullability) *expressions.LiteralExpression {
	typ := must(types.NewBoolType(n))
	var value valueinterfaces.Value
	if truth == foundation.Null {
		value = must(testbedvalues.NewNull(typ))
	} else {
		value = must(testbedvalues.NewBool(typ, truth == foundation.True))
	}
	return must(expressions.NewLiteralExpression(value))
}

func truthOf(value valueinterfaces.Value) foundation.TruthValue {
	if value.IsNull() {
		return foundation.Null
	}
	if value.(*testbedvalues.BoolValue).Bool() {
		return foundation.True
	}
	return foundation.False
}

func TestLogicalTruthTables(t *testing.T) {
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	// Rows/columns are True, False, Null. Expected tables are explicit fixtures.
	inputs := []foundation.TruthValue{foundation.True, foundation.False, foundation.Null}
	and := [][]foundation.TruthValue{{foundation.True, foundation.False, foundation.Null}, {foundation.False, foundation.False, foundation.False}, {foundation.Null, foundation.False, foundation.Null}}
	or := [][]foundation.TruthValue{{foundation.True, foundation.True, foundation.True}, {foundation.True, foundation.False, foundation.Null}, {foundation.True, foundation.Null, foundation.Null}}
	not := []foundation.TruthValue{foundation.False, foundation.True, foundation.Null}
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for a, left := range inputs {
			if n == foundation.NonNull && left == foundation.Null {
				continue
			}
			l := truthLiteral(left, n)
			unary := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{l}))
			for _, partial := range []bool{false, true} {
				checkLogical(t, evaluator, unary, not[a], partial)
			}
			for b, right := range inputs {
				if n == foundation.NonNull && right == foundation.Null {
					continue
				}
				r := truthLiteral(right, n)
				for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
					expected := and[a][b]
					if op == foundation.Or {
						expected = or[a][b]
					}
					node := must(expressions.NewLogicalExpression(op, []expressions.Expression{l, r}))
					for _, partial := range []bool{false, true} {
						checkLogical(t, evaluator, node, expected, partial)
					}
				}
			}
		}
	}
	// A nullable operand retains Bool? even when a decisive non-null operand wins.
	for _, tc := range []struct {
		op    foundation.LogicalOperator
		value foundation.TruthValue
	}{
		{foundation.And, foundation.False}, {foundation.Or, foundation.True},
	} {
		node := must(expressions.NewLogicalExpression(tc.op, []expressions.Expression{truthLiteral(foundation.Null, foundation.MayNull), truthLiteral(tc.value, foundation.NonNull), truthLiteral(foundation.Null, foundation.MayNull)}))
		checkLogical(t, evaluator, node, tc.value, true)
		checkLogical(t, evaluator, node, tc.value, false)
	}
}

func checkLogical(t *testing.T, i *interpreter.ExpressionInterpreter, node expressions.Expression, want foundation.TruthValue, partial bool) {
	t.Helper()
	var value valueinterfaces.Value
	if partial {
		result, err := i.PartialEvaluate(node, &environment{})
		if err != nil {
			t.Fatal(err)
		}
		var known bool
		value, known = result.Known()
		if !known {
			t.Fatal("all-known logical expression remained residual")
		}
	} else {
		var err error
		value, err = i.Evaluate(node, &environment{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if truthOf(value) != want || !value.Type().EqualsTo(node.Type()) {
		t.Fatalf("%s: got %v (%s), want %v (%s)", node, truthOf(value), value.Type(), want, node.Type())
	}
}

func TestLogicalResidualsAndOrder(t *testing.T) {
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	typ := must(types.NewBoolType(foundation.MayNull))
	a, b := must(ids.NewColumnID("t", "a")), must(ids.NewColumnID("t", "b"))
	refA := must(expressions.NewReferenceExpression(a, typ, "a"))
	refB := must(expressions.NewReferenceExpression(b, typ, "b"))
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		for _, known := range []foundation.TruthValue{foundation.True, foundation.False, foundation.Null} {
			value := truthLiteral(known, foundation.MayNull).Value()
			env := &environment{bindings: []binding{{a, value}}}
			root := must(expressions.NewLogicalExpression(op, []expressions.Expression{refA, refB}))
			result := must(evaluator.PartialEvaluate(root, env))
			residual, ok := result.Residual()
			if !ok || residual.Children()[1] != refB || residual.Children()[0].(*expressions.LiteralExpression).Value() != value || !result.Type().EqualsTo(root.Type()) {
				t.Fatal("residual order/type lost or unknown operand discarded")
			}
			if root.Children()[0] != refA {
				t.Fatal("original mutated")
			}
			if _, err := evaluator.Evaluate(root, env); !errors.Is(err, interpreter.ErrUnavailableInput) {
				t.Fatal(err)
			}
		}
	}
	unary := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{refB}))
	if residual, _ := must(evaluator.PartialEvaluate(unary, &environment{})).Residual(); residual != unary {
		t.Fatal("unchanged NOT not shared")
	}
}

func TestLogicalDoesNotSuppressFailures(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	one, zero := intLiteral(1, foundation.NonNull), intLiteral(0, foundation.NonNull)
	divide := arithmetic(provider, foundation.Divide, one, zero)
	handle := must(provider.ComparisonFunction(foundation.Equal, divide.Type(), one.Type()))
	failing := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{divide, one}))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, tc := range []struct {
		op    foundation.LogicalOperator
		truth foundation.TruthValue
	}{{foundation.And, foundation.False}, {foundation.Or, foundation.True}} {
		decisive := truthLiteral(tc.truth, foundation.NonNull)
		for _, children := range [][]expressions.Expression{{decisive, failing}, {failing, decisive}} {
			root := must(expressions.NewLogicalExpression(tc.op, children))
			if result, err := evaluator.PartialEvaluate(root, &environment{}); result.IsValid() || !errors.Is(err, functions.ErrEvaluationFailure) {
				t.Fatal("partial evaluation suppressed failure", err)
			}
			if _, err := evaluator.Evaluate(root, &environment{}); !errors.Is(err, functions.ErrEvaluationFailure) {
				t.Fatal("complete evaluation suppressed failure", err)
			}
		}
	}
}

func TestNotMatchesBuiltIn(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, truth := range []foundation.TruthValue{foundation.True, foundation.False, foundation.Null} {
		leaf := truthLiteral(truth, foundation.MayNull)
		logical := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{leaf}))
		handle := must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{leaf.Type()}))
		call := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{leaf}))
		a := must(evaluator.Evaluate(logical, &environment{}))
		b := must(evaluator.Evaluate(call, &environment{}))
		if !a.EqualsTo(b) || logical.EqualsTo(call) {
			t.Fatal("NOT semantic/structural distinction lost")
		}
	}
}

func TestLogicalOpaquePreservation(t *testing.T) {
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	no := truthLiteral(foundation.False, foundation.NonNull)
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("adapter", "region")), no.Type(), "opaque"))
	guarded := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{no, opaque}))
	if residual, _ := must(evaluator.PartialEvaluate(guarded, &environment{})).Residual(); residual != guarded {
		t.Fatal("decisive value discarded opaque region")
	}
	if _, err := evaluator.Evaluate(guarded, &environment{}); !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
		t.Fatal(err)
	}
}
