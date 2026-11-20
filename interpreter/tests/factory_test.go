package interpreter_test

import (
	"errors"
	"testing"

	folding "github.com/lie-yan/predicate-optimization/concrete_rewriters/folding_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

type externalResultFactory struct{ testbedvalues.ValueFactory }

func (f externalResultFactory) NewBool(typ *types.BoolType, b bool) (valueinterfaces.Value, error) {
	return fixtures.WrapScalar(must(f.ValueFactory.NewBool(typ, b))), nil
}

func (f externalResultFactory) NewNull(typ types.Type) (valueinterfaces.Value, error) {
	return fixtures.WrapScalar(must(f.ValueFactory.NewNull(typ))), nil
}

// Equality returns an external Boolean or typed null; IN must read its payload
// before constructing the overall membership result with the interpreter factory.
type externalEquality struct {
	functioninterfaces.FunctionContext
}

func (p externalEquality) Evaluate(h functioninterfaces.FunctionHandle, s *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	result, err := p.FunctionContext.Evaluate(h, s, args)
	if err != nil {
		return nil, err
	}
	return fixtures.WrapScalar(result), nil
}

func externalLiteral(value valueinterfaces.Value) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(fixtures.WrapScalar(value)))
}

func TestInterpreterFactoryResults(t *testing.T) {
	factory := &fixtures.RecordingFactory{Delegate: externalResultFactory{}}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(factory, externalEquality{must(testbed.NewStandardContext())})
	truths := []foundation.TruthValue{foundation.True, foundation.False, foundation.Null}
	and := [][]foundation.TruthValue{{foundation.True, foundation.False, foundation.Null}, {foundation.False, foundation.False, foundation.False}, {foundation.Null, foundation.False, foundation.Null}}
	or := [][]foundation.TruthValue{{foundation.True, foundation.True, foundation.True}, {foundation.True, foundation.False, foundation.Null}, {foundation.True, foundation.Null, foundation.Null}}
	not := []foundation.TruthValue{foundation.False, foundation.True, foundation.Null}
	check := func(node expressions.Expression, want foundation.TruthValue) {
		t.Helper()
		for _, partial := range []bool{false, true} {
			factory.Requests = nil
			var result valueinterfaces.Value
			if partial {
				r := must(evaluator.PartialEvaluate(node, &environment{}))
				var known bool
				result, known = r.Known()
				if !known {
					t.Fatal("expected known result")
				}
			} else {
				result = must(evaluator.Evaluate(node, &environment{}))
			}
			if _, ok := result.(fixtures.ScalarView); !ok || !result.Type().EqualsTo(node.Type()) {
				t.Fatal("lost factory representation or exact type")
			}
			got := foundation.Null
			if !result.IsNull() {
				got = foundation.False
				if result.(valueinterfaces.BoolValue).Bool() {
					got = foundation.True
				}
			}
			if got != want || len(factory.Requests) != 1 || !factory.Requests[0].Type.EqualsTo(node.Type()) {
				t.Fatalf("wrong result or factory request: %v, %+v", got, factory.Requests)
			}
			kind := "Bool"
			if want == foundation.Null {
				kind = "Null"
			}
			if factory.Requests[0].Kind != kind {
				t.Fatal("wrong factory constructor")
			}
		}
	}
	for a, left := range truths {
		l := externalLiteral(truthLiteral(left, foundation.MayNull).Value())
		check(must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{l})), not[a])
		for b, right := range truths {
			r := externalLiteral(truthLiteral(right, foundation.MayNull).Value())
			check(must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{l, r})), and[a][b])
			check(must(expressions.NewLogicalExpression(foundation.Or, []expressions.Expression{l, r})), or[a][b])
		}
	}
	one := externalLiteral(intLiteral(1, foundation.NonNull).Value())
	for _, tc := range []struct {
		value valueinterfaces.Value
		want  foundation.TruthValue
	}{
		{intLiteral(1, foundation.MayNull).Value(), foundation.True},
		{intLiteral(2, foundation.NonNull).Value(), foundation.False},
		{must(testbedvalues.NewNull(intLiteral(1, foundation.MayNull).Type())), foundation.Null},
	} {
		check(membership(externalLiteral(tc.value), one), tc.want)
	}
}

func TestInterpreterFactoryFailuresAndResiduals(t *testing.T) {
	sentinel := errors.New("construction failed")
	factory := &fixtures.RecordingFactory{Failure: sentinel}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(factory, externalEquality{must(testbed.NewStandardContext())})
	yes := externalLiteral(truthLiteral(foundation.True, foundation.MayNull).Value())
	null := externalLiteral(truthLiteral(foundation.Null, foundation.MayNull).Value())
	one := externalLiteral(intLiteral(1, foundation.NonNull).Value())
	nullInt := externalLiteral(must(testbedvalues.NewNull(intLiteral(1, foundation.MayNull).Type())))
	nodes := []expressions.Expression{must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{yes})), must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{null})), membership(one, one), membership(nullInt, one)}
	for _, node := range nodes {
		if result, err := evaluator.Evaluate(node, &environment{}); result != nil || !errors.Is(err, sentinel) {
			t.Fatal("complete evaluation concealed factory error")
		}
		if result, err := evaluator.PartialEvaluate(node, &environment{}); result.IsValid() || !errors.Is(err, sentinel) {
			t.Fatal("partial evaluation concealed factory error")
		}
		for _, preserve := range []bool{false, true} {
			result, err := folding.NewFoldingRewriter(evaluator, preserve).RewriteExpression(node, &environment{}, nil)
			if _, present := result.Get(); present || !errors.Is(err, sentinel) {
				t.Fatal("folding concealed uncategorized construction failure")
			}
		}
	}
	// Folding's explicit semantic-error policy still applies to categorized errors.
	factory.Failure = functions.ErrEvaluationFailure
	retained, err := folding.NewFoldingRewriter(evaluator, true).RewriteExpression(nodes[0], &environment{}, nil)
	if node, present := retained.Get(); err != nil || !present || node != nodes[0] {
		t.Fatal("folding semantic-error policy changed")
	}
	factory.Requests = nil
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("factory", "missing")), yes.Type(), "missing"))
	residual := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, ref}))
	partial := must(evaluator.PartialEvaluate(residual, &environment{}))
	if _, ok := partial.Residual(); !ok || len(factory.Requests) != 0 {
		t.Fatal("residual evaluation synthesized a result")
	}
	if result, err := evaluator.Evaluate(residual, &environment{}); result != nil || !errors.Is(err, interpreter.ErrUnavailableInput) {
		t.Fatal("missing input became a value")
	}
}

func TestInterpreterPreservesProviderAndLeafResults(t *testing.T) {
	factory := &fixtures.RecordingFactory{Failure: errors.New("unexpected construction")}
	evaluator := interpreter.NewExpressionInterpreter(factory)
	value := fixtures.WrapScalar(intLiteral(9, foundation.NonNull).Value())
	literal := must(expressions.NewLiteralExpression(value))
	id := must(ids.NewColumnID("factory", "present"))
	ref := must(expressions.NewReferenceExpression(id, value.Type(), "present"))
	provider := &callProvider{FunctionContext: must(testbed.NewStandardContext()), override: true, result: value}
	call := arithmetic(provider, foundation.Add, literal, literal)
	env := &environment{bindings: []binding{{id, value}}}
	for _, node := range []expressions.Expression{literal, ref, call} {
		result := must(evaluator.Evaluate(node, env))
		if _, ok := result.(fixtures.ScalarView); !ok || !result.EqualsTo(value) {
			t.Fatal("source representation was replaced")
		}
		known, ok := must(evaluator.PartialEvaluate(node, env)).Known()
		if !ok || !known.EqualsTo(value) {
			t.Fatal("partial result changed")
		}
	}
	if len(factory.Requests) != 0 {
		t.Fatal("provider/leaf results reconstructed")
	}
}

func TestInterpreterFactoryPreconditions(t *testing.T) {
	if !assertions.Enabled {
		return
	}
	provider := must(testbed.NewStandardContext())
	for _, factory := range []valueinterfaces.ValueFactory{nil, (*testbedvalues.ValueFactory)(nil)} {
		for _, withProvider := range []bool{false, true} {
			func() {
				defer func() {
					if recover() == nil {
						t.Error("nil factory accepted")
					}
				}()
				if withProvider {
					interpreter.NewExpressionInterpreterWithFunctionContext(factory, provider)
				} else {
					interpreter.NewExpressionInterpreter(factory)
				}
			}()
		}
	}
}
