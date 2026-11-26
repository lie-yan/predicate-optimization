package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"

	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestProveLiteralFiniteAndRangeTargets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rangeGuard bool
		op         foundation.OperatorType
		n          int64
		want       foundation.TruthValue
	}{
		{"finite false", false, foundation.Equal, 3, foundation.False},
		{"finite true", false, foundation.LessThan, 3, foundation.True},
		{"range false", true, foundation.GreaterThanOrEqual, 20, foundation.False},
		{"range true", true, foundation.LessThan, 20, foundation.True},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := membershipEligibilityProvider()
			rule := engine.New(testbedvalues.ValueFactory{}, provider)
			x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
			var guard expressions.Expression = membershipExpression(x, guardInteger(1), guardInteger(2))
			if tc.rangeGuard {
				guard = guardComparison(provider, foundation.LessThan, x, 10)
			}
			target := guardComparison(provider, tc.op, x, tc.n)
			source := guardLogical(foundation.And, guard, target)
			attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
			before := expressions.PrettyPrint(source)
			result, present := must(attempt.ProveLiteral(target, engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1}})).Get()
			if !present {
				t.Fatal("expected proved literal")
			}
			expected := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, target.Type().(*types.BoolType), tc.want))
			if !result.Expression().EqualsTo(expected) || result.Expression().Type() != target.Type() || len(result.Steps()) != 1 {
				t.Fatal("wrong literal, original Type or evidence count")
			}
			step := result.Steps()[0]
			proof, hasProof := step.Conclusion.Get()
			if !hasProof || proof.Summary().Source() != target || proof.InputDomain() != attempt.Input() || step.Source != target || step.Before != target || step.After != result.Expression() || !step.Path.EqualsTo(engine.OccurrencePath{1}) || !proof.Premises()[0].EqualsTo(attempt.Premise()) {
				t.Fatal("proof lost source/domain/dependency association")
			}
			// Independent execution is only an oracle: the optimization provider's
			// Evaluate method panics, and the interpreter uses its real delegate here.
			evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider.FunctionContext)
			oracleGuard := guard
			if tc.rangeGuard {
				oracleGuard = guardComparison(provider.FunctionContext, foundation.LessThan, x, 10)
			}
			oracleTarget := guardComparison(provider.FunctionContext, tc.op, x, tc.n)
			oracleSource := guardLogical(foundation.And, oracleGuard, oracleTarget)
			candidate := guardLogical(foundation.And, oracleGuard, result.Expression())
			for _, n := range []int64{-1, 0, 1, 2, 3, 9, 10, 20, 21} {
				env := changedChildrenEnvironment{value: guardInteger(n).Value()}
				got := must(evaluator.Evaluate(candidate, env))
				original := must(evaluator.Evaluate(oracleSource, env))
				if !got.EqualsTo(original) {
					t.Fatalf("whole guarded region differs at x=%d", n)
				}
			}
			if expressions.PrettyPrint(source) != before {
				t.Fatal("proof mutated source")
			}
		})
	}
}

func TestProveLiteralAbsenceAndAssociation(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	x := guardReference("x", typ)
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	unknown := guardComparison(provider, foundation.Equal, x, 1)
	other := guardComparison(provider, foundation.Equal, guardReference("y", typ), 1)
	nullable := guardComparison(provider, foundation.Equal, guardReference("x", must(types.NewIntType(types.Int64, foundation.MayNull))), 1)
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "opaque")), must(types.NewBoolType(foundation.NonNull)), "opaque"))
	source := guardLogical(foundation.And, guard, unknown, other, nullable, opaque, guard)
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	for _, path := range []engine.OccurrencePath{nil, {0}, {0, 0}, {1}, {1, 0}, {2}, {3}, {4}} {
		node := must(path.Resolve(source))
		if _, present := must(attempt.ProveLiteral(node, engine.RewriteContext{Attempt: attempt, Path: path})).Get(); present {
			t.Fatal("unexpected substitution at", path)
		}
	}
	// Sharing the guard object does not freeze a different occurrence.
	if _, present := must(attempt.ProveLiteral(guard, engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{5}})).Get(); !present {
		t.Fatal("other shared occurrence was frozen")
	}
	equalButDistinct := must(expressions.NewCallExpression(unknown.Context(), unknown.Handle(), unknown.Arguments()))
	for _, tc := range []struct {
		context engine.RewriteContext
		node    expressions.Expression
	}{
		{engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1}}, equalButDistinct},
		{engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{99}}, unknown},
		{engine.RewriteContext{Attempt: requireAttempt(t, rule, source, attempt.Guard()), Path: engine.OccurrencePath{1}}, unknown},
	} {
		result, err := attempt.ProveLiteral(tc.node, tc.context)
		if _, present := result.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
			t.Fatal("invalid association did not fail", err)
		}
	}

}

func TestProveLiteralPreservesNullableTypeAndKnownNull(t *testing.T) {
	for _, nullOnly := range []bool{false, true} {
		provider := membershipEligibilityProvider()
		rule := engine.New(testbedvalues.ValueFactory{}, provider)
		typ := must(types.NewIntType(types.Int64, foundation.MayNull))
		x := guardReference("x", typ)
		target := guardComparison(provider, foundation.Equal, x, 1)
		// Exact producer fixtures for future two-valued guards over nullable x:
		// x IS NOT DISTINCT FROM 1, or x IS NULL. Current selection does not extract them.
		guard := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "nullable")), must(types.NewBoolType(foundation.NonNull)), "future two-valued guard"))
		source := guardLogical(foundation.And, guard, target)
		entries := []valueinterfaces.Value{guardInteger(1).Value()}
		nullTruth := foundation.False
		want := foundation.True
		if nullOnly {
			entries = nil
			nullTruth = foundation.True
			want = foundation.Null
		}
		set := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, entries))
		summary := must(facts.NewPredicateSummary(x, set, foundation.Some(nullTruth), guard))
		attempt := requireAttempt(t, rule, source, engine.GuardCandidate{Path: engine.OccurrencePath{0}, Expression: guard, Summary: summary})
		result, present := must(attempt.ProveLiteral(target, engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1}})).Get()
		if !present || result.Expression().Type() != target.Type() || !result.Expression().EqualsTo(must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, target.Type().(*types.BoolType), want))) {
			t.Fatal("nullable truth substitution lost its declared Type or payload")
		}
	}
}

func TestProveLiteralErrorsAndOmissionAdmission(t *testing.T) {
	for _, stage := range []string{"extraction", "omission error", "omission refusal"} {
		t.Run(stage, func(t *testing.T) {
			a, target, _ := stepFixture(t)
			provider := target.Context().(*membershipEligibilityContext)
			sentinel := errors.New("injected proof failure")
			provider.queries = 0
			// Deliberately vary inspection responses after valid construction to
			// isolate omission handling; this is not a conforming stable provider.
			provider.metadata = func(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
				if stage == "extraction" || provider.queries == 2 && stage == "omission error" {
					return nil, sentinel
				}
				metadata, err := provider.FunctionContext.Metadata(handle)
				if provider.queries == 2 && stage == "omission refusal" {
					metadata = must(functions.NewFunctionMetadata(metadata.Signature(), metadata.DisplayName(), foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, metadata.NullPolicies()))
				}
				return metadata, err
			}
			result, err := a.ProveLiteral(target, engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}})
			if _, present := result.Get(); present {
				t.Fatal("failed proof returned a candidate")
			}
			if stage == "omission refusal" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, sentinel) {
				t.Fatal("lost failure cause", err)
			}
		})
	}
}
