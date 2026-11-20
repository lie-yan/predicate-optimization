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
)

func membership(target expressions.Expression, candidates ...expressions.Expression) *expressions.SpecialFormExpression {
	n := target.Type().Nullability()
	for _, c := range candidates {
		if c.Type().Nullability() == foundation.MayNull {
			n = foundation.MayNull
		}
	}
	args := append([]expressions.Expression{target}, candidates...)
	return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(n)), args))
}

func TestMembershipAndNotIn(t *testing.T) {
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	textLiteral := func(s string) *expressions.LiteralExpression {
		return must(expressions.NewLiteralExpression(must(testbedvalues.NewVarChar(text, s))))
	}
	cases := []struct {
		target     *expressions.LiteralExpression
		candidates []expressions.Expression
		want       foundation.TruthValue
	}{
		{truthLiteral(foundation.True, foundation.NonNull), []expressions.Expression{truthLiteral(foundation.False, foundation.NonNull)}, foundation.False},
		{truthLiteral(foundation.True, foundation.NonNull), []expressions.Expression{truthLiteral(foundation.False, foundation.NonNull), truthLiteral(foundation.True, foundation.NonNull)}, foundation.True},
		{intLiteral(1, foundation.NonNull), []expressions.Expression{intLiteral(2, foundation.NonNull), intLiteral(1, foundation.NonNull), intLiteral(1, foundation.NonNull)}, foundation.True},
		{intLiteral(3, foundation.NonNull), []expressions.Expression{intLiteral(1, foundation.NonNull), intLiteral(2, foundation.NonNull)}, foundation.False},
		{textLiteral("tsa_01"), []expressions.Expression{textLiteral("tsa_01"), textLiteral("tsa_02"), textLiteral("tsa_03")}, foundation.True},
		{textLiteral("tsa_04"), []expressions.Expression{textLiteral("tsa_01"), textLiteral("tsa_02"), textLiteral("tsa_03")}, foundation.False},
		{textLiteral("A"), []expressions.Expression{textLiteral("a")}, foundation.False},
	}
	for _, tc := range cases {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			payload := must(tc.target.Value().WithNullability(n))
			target := must(expressions.NewLiteralExpression(payload))
			root := membership(target, tc.candidates...)
			checkLogical(t, evaluator, root, tc.want, false)
			checkLogical(t, evaluator, root, tc.want, true)
			not := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{root}))
			want := foundation.True
			if tc.want == foundation.True {
				want = foundation.False
			}
			checkLogical(t, evaluator, not, want, false)
			checkLogical(t, evaluator, not, want, true)
			if n == foundation.MayNull {
				null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(target.Type()))))
				root = membership(null, tc.candidates...)
				checkLogical(t, evaluator, root, foundation.Null, false)
				checkLogical(t, evaluator, root, foundation.Null, true)
				checkLogical(t, evaluator, must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{root})), foundation.Null, true)
			}
		}
	}
}

func TestMembershipAdmissionAndMissingInputs(t *testing.T) {
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	one := intLiteral(1, foundation.NonNull)
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), one.Type(), "x"))
	nullable := intLiteral(1, foundation.MayNull)
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullable.Type()))))
	for _, root := range []*expressions.SpecialFormExpression{membership(ref), membership(ref, ref), membership(ref, nullable), membership(ref, null)} {
		env := &environment{err: errors.New("gated input must not be evaluated")}
		result := must(evaluator.PartialEvaluate(root, env))
		if residual, _ := result.Residual(); residual != root || env.calls != 0 {
			t.Fatal("gated membership changed or executed")
		}
		if _, err := evaluator.Evaluate(root, env); !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
			t.Fatal(err)
		}
	}
	root := membership(ref, one, one)
	if residual, _ := must(evaluator.PartialEvaluate(root, &environment{})).Residual(); residual != root {
		t.Fatal("missing target changed")
	}
	if _, err := evaluator.Evaluate(root, &environment{}); !errors.Is(err, interpreter.ErrUnavailableInput) {
		t.Fatal(err)
	}
	bound := &environment{bindings: []binding{{ref.ID(), one.Value()}}}
	if got := must(evaluator.Evaluate(root, bound)); !got.(*testbedvalues.BoolValue).Bool() {
		t.Fatal("bound target did not match")
	}
	if root.Arguments()[0] != ref || len(root.Arguments()) != 3 {
		t.Fatal("membership candidates mutated")
	}
	noProvider := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	if residual, _ := must(noProvider.PartialEvaluate(root, bound)).Residual(); residual != root {
		t.Fatal("missing provider not preserved")
	}
	if _, err := noProvider.Evaluate(root, bound); !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
		t.Fatal(err)
	}
}

func TestMembershipProviderFailures(t *testing.T) {
	standard := must(testbed.NewStandardContext())
	one := intLiteral(1, foundation.NonNull)
	root := membership(one, one)
	for _, failure := range []error{functions.ErrEvaluationUnavailable, functions.ErrEvaluationDisallowed, functions.ErrEvaluationFailure, functions.ErrContractViolation} {
		provider := &callProvider{FunctionContext: standard, override: true, err: failure}
		evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
		result, err := evaluator.PartialEvaluate(root, &environment{})
		unavailable := failure == functions.ErrEvaluationUnavailable || failure == functions.ErrEvaluationDisallowed
		if unavailable {
			if err != nil {
				t.Fatal(err)
			}
			if residual, _ := result.Residual(); residual != root {
				t.Fatal("unavailable membership changed")
			}
		} else if result.IsValid() || !errors.Is(err, failure) {
			t.Fatal("failure hidden", err)
		}
		if _, err := evaluator.Evaluate(root, &environment{}); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	narrow := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(must(types.NewIntType(types.Int8, foundation.NonNull)), 1))))
	unsupported := membership(narrow, narrow)
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, standard)
	if residual, _ := must(evaluator.PartialEvaluate(unsupported, &environment{})).Residual(); residual != unsupported {
		t.Fatal("unsupported width evaluated")
	}
	if _, err := evaluator.Evaluate(unsupported, &environment{}); !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatal(err)
	}
}

func TestEmptyMembershipRemainsGatedForEveryTarget(t *testing.T) {
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	nullable := intLiteral(1, foundation.MayNull)
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullable.Type()))))
	for _, target := range []expressions.Expression{intLiteral(1, foundation.NonNull), nullable, null} {
		in := membership(target)
		not := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{in}))
		for _, root := range []expressions.Expression{in, not} {
			if residual, _ := must(evaluator.PartialEvaluate(root, &environment{})).Residual(); residual != root {
				t.Fatal("empty list was folded")
			}
			if _, err := evaluator.Evaluate(root, &environment{}); !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
				t.Fatal(err)
			}
		}
	}
}
