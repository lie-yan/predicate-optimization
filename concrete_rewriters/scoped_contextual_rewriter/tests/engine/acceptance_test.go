package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func acceptanceFixture(t *testing.T) (*engine.RewriteAttempt, engine.RewriteResult, *membershipEligibilityContext) {
	t.Helper()
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	flag := guardReference("a", must(types.NewBoolType(foundation.MayNull)))
	predicate := guardComparison(provider, foundation.Equal, x, 3)
	source := guardLogical(foundation.And, guard, guardLogical(foundation.Not, guardLogical(foundation.And, flag, predicate)))
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	return attempt, must(attempt.Rewrite()), provider
}

func TestAcceptCompleteEvidenceChain(t *testing.T) {
	a, result, provider := acceptanceFixture(t)
	if len(result.Steps) != 3 {
		t.Fatal("expected proof, absorbing cleanup and NOT cleanup")
	}
	queries, lookups := provider.queries, provider.lookups
	before := expressions.PrettyPrint(a.Source)
	accepted, present := must(a.Accept(result)).Get()
	if !present || accepted != result.Expression || !accepted.Type().EqualsTo(a.Source.Type()) {
		t.Fatal("complete improving result was not accepted with its exact Type")
	}
	if a.Source.Type().Nullability() != foundation.MayNull || accepted.Children()[0] != a.Guard.Expression {
		t.Fatal("nullable declaration or guard instance changed")
	}
	if !engine.Improves(reconstruction.MeasureExpression(a.Source), reconstruction.MeasureExpression(accepted)) {
		t.Fatal("accepted result did not improve the whole region")
	}
	proof, _ := result.Steps[0].Conclusion.Get()
	if len(proof.Premises()) != 1 || !proof.Premises()[0].EqualsTo(a.Premise) {
		t.Fatal("disappeared target lost its dependency")
	}
	if provider.queries != queries || provider.lookups != lookups || expressions.PrettyPrint(a.Source) != before {
		t.Fatal("acceptance repeated provider work or mutated source")
	}
	// Acceptance is read-only and does not consume or cache the result.
	if again, ok := must(a.Accept(result)).Get(); !ok || again != accepted {
		t.Fatal("repeated acceptance changed the result")
	}
}

func TestAcceptDeclinesUnchangedConstruction(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	source := guardLogical(foundation.And, guard, guardComparison(provider, foundation.Equal, x, 1))
	a := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	result := must(a.Rewrite())
	if result.Expression != source || len(result.Steps) != 0 {
		t.Fatal("fixture must remain unchanged under the selected guard")
	}
	if _, present := must(a.Accept(result)).Get(); present {
		t.Fatal("unchanged result must be ordinary absence")
	}
}

func TestAcceptRejectsMalformedConstruction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*engine.RewriteAttempt, *engine.RewriteResult)
	}{
		{"missing candidate", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) { r.Expression = nil }},
		{"missing guard", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			truth := cleanupLiteral(foundation.True, foundation.MayNull)
			r.Expression = guardLogical(foundation.And, truth, truth)
		}},
		{"moved guard", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			children := r.Expression.Children()
			r.Expression = guardLogical(foundation.And, children[1], children[0])
		}},
		{"equal distinct guard", func(a *engine.RewriteAttempt, r *engine.RewriteResult) {
			copy := *a.Guard.Expression.(*expressions.SpecialFormExpression)
			r.Expression = guardLogical(foundation.And, &copy, r.Expression.Children()[1])
		}},
		{"guard under OR", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			r.Expression = guardLogical(foundation.Or, r.Expression.Children()...)
		}},
		{"narrowed result Type", func(a *engine.RewriteAttempt, r *engine.RewriteResult) {
			r.Expression = guardLogical(foundation.And, a.Guard.Expression, cleanupLiteral(foundation.True, foundation.NonNull))
		}},
		{"unexplained sibling substitution", func(a *engine.RewriteAttempt, r *engine.RewriteResult) {
			r.Expression = guardLogical(foundation.And, a.Guard.Expression, cleanupLiteral(foundation.False, foundation.MayNull))
		}},
		{"larger equivalent tree without construction evidence", func(a *engine.RewriteAttempt, r *engine.RewriteResult) {
			children := a.Source.Children()
			children = append(children, cleanupLiteral(foundation.True, foundation.NonNull))
			r.Expression = guardLogical(foundation.And, children...)
		}},
		{"equal candidate copy without construction evidence", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			r.Expression = guardLogical(foundation.And, r.Expression.Children()...)
		}},
		{"unbound manually assembled result", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			*r = engine.NewRewriteResult(r.Expression, r.Steps)
		}},
		{"dropped evidence", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) { r.Steps = r.Steps[1:] }},
		{"duplicated evidence", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) { r.Steps = append(r.Steps, r.Steps[0]) }},
		{"reordered evidence", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			r.Steps[0], r.Steps[1] = r.Steps[1], r.Steps[0]
		}},
		{"changed source path", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) { r.Steps[0].Path[0] = 0 }},
		{"stale equal source", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			copy := *r.Steps[0].Source.(*expressions.CallExpression)
			r.Steps[0].Source = &copy
		}},
		{"changed before", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) { r.Steps[1].Before = r.Steps[1].Source }},
		{"changed cleanup after", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			r.Steps[1].After = cleanupLiteral(foundation.True, foundation.MayNull)
		}},
		{"dropped dependency", func(_ *engine.RewriteAttempt, r *engine.RewriteResult) {
			r.Steps[0].Conclusion = foundation.Optional[scoped_reasoning.ScopedConclusion]{}
		}},
		{"foreign proof", func(a *engine.RewriteAttempt, r *engine.RewriteResult) {
			other := requireAttempt(t, a.Rule, a.Source, a.Guard)
			proof, _ := r.Steps[0].Conclusion.Get()
			r.Steps[0].Conclusion = must(scoped_reasoning.ProveTruth(proof.Summary(), other.Input))
		}},
		{"foreign complete result", func(a *engine.RewriteAttempt, r *engine.RewriteResult) {
			other := requireAttempt(t, a.Rule, a.Source, a.Guard)
			*r = must(other.Rewrite())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, result, _ := acceptanceFixture(t)
			tc.alter(a, &result)
			got, err := a.Accept(result)
			if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
				t.Fatalf("malformed result returned a candidate or wrong error: %v", err)
			}
		})
	}
}

func TestAcceptRejectsChangedAttemptAssociations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*engine.RewriteAttempt)
	}{
		{"source snapshot", func(a *engine.RewriteAttempt) { copy := *a.Source; a.Source = &copy }},
		{"scope", func(a *engine.RewriteAttempt) { a.Scope = scoped_reasoning.NewScopedFactContext().NewScope() }},
		{"foreign premise", func(a *engine.RewriteAttempt) {
			owner := scoped_reasoning.NewScopedFactContext()
			a.Premise = must(owner.RegisterPremise(owner.NewScope(), a.Guard.Expression))
		}},
		{"narrower inputs with same premise", func(a *engine.RewriteAttempt) {
			narrow := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, a.Input.Operand().Type(), []valueinterfaces.Value{guardInteger(1).Value()}))
			a.Input = must(scoped_reasoning.NewInputDomain(a.Scope, a.Input.Operand(), narrow, false, []*scoped_reasoning.Premise{a.Premise}))
		}},
		{"different summary", func(a *engine.RewriteAttempt) {
			s := a.Guard.Summary
			a.Guard.Summary = must(facts.NewPredicateSummary(s.Operand(), s.TrueSet(), s.NullTruth(), s.Source()))
		}},
		{"guard path", func(a *engine.RewriteAttempt) { a.Guard.Path[0] = 1 }},
		{"guard expression", func(a *engine.RewriteAttempt) { a.Guard.Expression = a.Source }},
		{"engine", func(a *engine.RewriteAttempt) {
			a.Rule = engine.New(testbedvalues.ValueFactory{}, membershipEligibilityProvider())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, result, _ := acceptanceFixture(t)
			tc.alter(a)
			got, err := a.Accept(result)
			if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
				t.Fatalf("changed attempt returned a candidate or wrong error: %v", err)
			}
			if built, err := a.Rewrite(); !errors.Is(err, rewriter.ErrInvalidReplacement) || built.Expression != nil || len(built.Steps) != 0 {
				t.Fatal("altered attempt produced a completed result", err)
			}
		})
	}
}

func TestAcceptRejectsMutualGuardDeletion(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := guardComparison(provider, foundation.Equal, x, 1)
	source := guardLogical(foundation.And, guard, guard)
	guards := must(rule.SelectGuards(source))
	left, right := requireAttempt(t, rule, source, guards[0]), requireAttempt(t, rule, source, guards[1])
	first, second := must(left.Rewrite()), must(right.Rewrite())
	if _, ok := must(left.Accept(first)).Get(); !ok {
		t.Fatal("first guard's own result rejected")
	}
	if _, ok := must(right.Accept(second)).Get(); !ok {
		t.Fatal("second guard's own result rejected")
	}
	// Each attempt may replace only the other occurrence. Combining both
	// conclusions cannot remove the premises that made either conclusion true.
	first.Expression = guardLogical(foundation.And, second.Expression.Children()[0], first.Expression.Children()[1])
	first.Steps = append(first.Steps, second.Steps...)
	got, err := left.Accept(first)
	if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("circular guard deletion accepted", err)
	}
}

func TestAcceptRejectsNullableGuardAndIncompleteVisit(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.MayNull)))
	guard := membershipExpression(x, guardInteger(1))
	source := guardLogical(foundation.And, guard, membershipExpression(x, guardInteger(1), guardInteger(2)))
	summary, _ := must(rule.Extractor.Extract(guard)).Summary().Get()
	// Bypass selection to exercise acceptance's two-valued-guard boundary.
	a := requireAttempt(t, rule, source, engine.GuardCandidate{Path: engine.OccurrencePath{0}, Expression: guard, Summary: summary})
	got, err := a.Accept(must(a.Visit(a.Source, engine.RewriteContext{Attempt: a})))
	if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("nullable guard accepted", err)
	}
	a, _, _ = acceptanceFixture(t)
	// Even a whole-root Visit result is tentative until Rewrite closes construction.
	got, err = a.Accept(must(a.Visit(a.Source, engine.RewriteContext{Attempt: a})))
	if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("unclosed traversal accepted", err)
	}
}

func TestRepeatedRewritesOwnEvidenceAndReceipts(t *testing.T) {
	attempt, first, _ := acceptanceFixture(t)
	original := expressions.PrettyPrint(attempt.Source)
	second := must(attempt.Rewrite())
	if len(first.Steps) == 0 || len(second.Steps) == 0 || !first.Expression.EqualsTo(second.Expression) {
		t.Fatal("expected repeatable rewrite with evidence")
	}
	first.Steps[0].Path[0] = 99
	first.Steps[0].After = attempt.Source
	if _, err := attempt.Accept(first); !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("mutated evidence was accepted", err)
	}
	if _, present := must(attempt.Accept(second)).Get(); !present {
		t.Fatal("mutating one result affected another result or its receipt")
	}
	if expressions.PrettyPrint(attempt.Source) != original {
		t.Fatal("rewrite mutated source")
	}
}
