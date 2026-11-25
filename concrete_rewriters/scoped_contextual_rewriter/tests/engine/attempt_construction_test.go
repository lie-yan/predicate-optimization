package engine_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"

	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func requireAttempt(t *testing.T, rule *engine.Engine, source *expressions.LogicalExpression, guard engine.GuardCandidate) *engine.RewriteAttempt {
	t.Helper()
	attempt, present := must(rule.NewAttempt(source, guard)).Get()
	if !present {
		t.Fatal("expected a proper nonempty restriction")
	}
	return attempt
}

func TestNewAttemptRestrictsDeclaredUniverse(t *testing.T) {
	for _, kind := range []string{"finite", "range"} {
		t.Run(kind, func(t *testing.T) {
			context := membershipEligibilityProvider()
			rule := engine.New(testbedvalues.ValueFactory{}, context)
			typ := must(types.NewIntType(types.Int64, foundation.NonNull))
			x := guardReference("x", typ)
			var predicate expressions.Expression = membershipExpression(x, guardInteger(1), guardInteger(2))
			if kind == "range" {
				predicate = guardComparison(context, foundation.LessThan, x, 10)
			}
			target := guardComparison(context, foundation.Equal, x, 1)
			source := guardLogical(foundation.And, predicate, target)
			guard := must(rule.SelectGuards(source))[0]
			context.queries, context.lookups = 0, 0
			before := expressions.PrettyPrint(source)
			attempt := requireAttempt(t, rule, source, guard)
			if attempt.Rule() != rule || attempt.Source() != source || attempt.Guard().Expression != predicate || attempt.Guard().Summary != guard.Summary {
				t.Fatal("attempt lost shared immutable inputs")
			}
			if attempt.Input().Operand() != x || attempt.Input().Operand().Type() != typ || attempt.Input().NullPossible() {
				t.Fatal("declared operand Type or null possibility changed")
			}
			if !must(attempt.Input().PossibleValues().EqualsTo(guard.Summary.TrueSet())) {
				t.Fatal("restriction differs from guard true set")
			}
			if !must(attempt.Input().PossibleValues().Contains(guardInteger(1).Value())) || must(attempt.Input().PossibleValues().Contains(guardInteger(20).Value())) {
				t.Fatal("wrong restricted values")
			}
			premises := attempt.Input().Premises()
			if len(premises) != 1 || !premises[0].EqualsTo(attempt.Premise()) || attempt.Premise().Expression() != predicate || !attempt.Scope().EqualsTo(attempt.Input().Scope()) || !attempt.Scope().EqualsTo(attempt.Premise().Scope()) {
				t.Fatal("restriction must depend on exactly its own guard occurrence")
			}
			conclusion, _ := must(scoped_reasoning.ProveTruth(guard.Summary, attempt.Input())).Get()
			truth, present := conclusion.Truth().Get()
			if !present || truth != foundation.True || len(conclusion.Premises()) != 1 || !conclusion.Premises()[0].EqualsTo(attempt.Premise()) {
				t.Fatal("guard truth did not retain its dependency")
			}
			if context.queries != 0 || context.lookups != 0 || expressions.PrettyPrint(source) != before {
				t.Fatal("construction queried provider or changed source")
			}
		})
	}
}

func TestNewAttemptDeclinesUnchangedAndUnreachable(t *testing.T) {
	context := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, context)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	equal := guardComparison(context, foundation.Equal, x, 1)
	universal := guardLogical(foundation.Or, equal, guardLogical(foundation.Not, equal))
	empty := guardLogical(foundation.Not, universal)
	for _, predicate := range []expressions.Expression{universal, empty} {
		source := guardLogical(foundation.And, predicate, equal)
		guards := must(rule.SelectGuards(source))
		if len(guards) != 2 {
			t.Fatal("selection must leave usefulness to attempts")
		}
		if _, present := must(rule.NewAttempt(source, guards[0])).Get(); present {
			t.Fatal("unchanged/unreachable restriction created an attempt")
		}
		requireAttempt(t, rule, source, guards[1])
	}
}

func TestNewAttemptOwnsIdentitiesAndPaths(t *testing.T) {
	context := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, context)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	predicate := guardComparison(context, foundation.Equal, x, 1)
	source := guardLogical(foundation.And, predicate, predicate)
	guards := must(rule.SelectGuards(source))
	a := requireAttempt(t, rule, source, guards[0])
	b := requireAttempt(t, rule, source, guards[0])
	c := requireAttempt(t, rule, source, guards[1])
	for _, pair := range [][2]*engine.RewriteAttempt{{a, b}, {a, c}, {b, c}} {
		if pair[0].Scope().EqualsTo(pair[1].Scope()) || pair[0].Premise().EqualsTo(pair[1].Premise()) {
			t.Fatal("fresh attempts shared identities")
		}
	}
	if !a.Guard().Path.EqualsTo(b.Guard().Path) || a.Guard().Path.EqualsTo(c.Guard().Path) {
		t.Fatal("scope identity confused with occurrence path")
	}
	guards[0].Path[0] = 99
	if !a.Guard().Path.EqualsTo(engine.OccurrencePath{0}) || !b.Guard().Path.EqualsTo(engine.OccurrencePath{0}) {
		t.Fatal("attempt retained caller-owned path storage")
	}
	a.Guard().Path[0] = 98
	if !b.Guard().Path.EqualsTo(engine.OccurrencePath{0}) {
		t.Fatal("paths alias across attempts")
	}
	premises := b.Input().Premises()
	premises[0] = c.Premise()
	if !b.Input().Premises()[0].EqualsTo(b.Premise()) {
		t.Fatal("premise slice aliases domain")
	}
	root := engine.RewriteContext{Attempt: b}
	parent := root.Child(0)
	left, right := parent.Child(1), parent.Child(2)
	left.Path[0] = 99
	if left.Attempt != b || right.Attempt != b || !right.Path.EqualsTo(engine.OccurrencePath{0, 2}) || !parent.Path.EqualsTo(engine.OccurrencePath{0}) || len(root.Path) != 0 {
		t.Fatal("child context lost owner or aliases another path")
	}
}

func TestNewAttemptTracksNullableUniverseSeparately(t *testing.T) {
	// Current selection cannot extract this opaque predicate. Supply an exact
	// producer fixtures for future Bool! guards: x IS NOT NULL and a tautology.
	// This isolates
	// attempt construction's treatment of a nullable operand, without claiming
	// the selector already supports IS NOT NULL.
	rule := engine.New(testbedvalues.ValueFactory{}, membershipEligibilityProvider())
	typ := must(types.NewIntType(types.Int64, foundation.MayNull))
	x := guardReference("x", typ)
	boolean := must(types.NewBoolType(foundation.NonNull))
	universe := must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	for _, nullTruth := range []foundation.TruthValue{foundation.False, foundation.True} {
		predicate := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "two-valued")), boolean, "two-valued nullable-operand fixture"))
		source := guardLogical(foundation.And, predicate, predicate)
		summary := must(facts.NewPredicateSummary(x, universe, foundation.Some(nullTruth), predicate))
		guard := engine.GuardCandidate{Path: engine.OccurrencePath{0}, Expression: predicate, Summary: summary}
		got, present := must(rule.NewAttempt(source, guard)).Get()
		if nullTruth == foundation.True {
			if present {
				t.Fatal("full nullable universe is unchanged")
			}
			continue
		}
		if !present || got.Input().NullPossible() || got.Input().Operand() != x || got.Input().Operand().Type() != typ || !must(got.Input().PossibleValues().IsUniversal()) {
			t.Fatal("null-only narrowing lost full operand Type or non-null universe")
		}
	}
}
