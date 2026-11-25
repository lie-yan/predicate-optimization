package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
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
	if len(result.Steps()) != 3 {
		t.Fatal("expected proof, absorbing cleanup and NOT cleanup")
	}
	queries, lookups := provider.queries, provider.lookups
	before := expressions.PrettyPrint(a.Source())
	accepted, present := must(a.Accept(result)).Get()
	if !present || accepted != result.Expression() || !accepted.Type().EqualsTo(a.Source().Type()) {
		t.Fatal("complete improving result was not accepted with its exact Type")
	}
	if a.Source().Type().Nullability() != foundation.MayNull || accepted.Children()[0] != a.Guard().Expression {
		t.Fatal("nullable declaration or guard instance changed")
	}
	if !engine.Improves(reconstruction.MeasureExpression(a.Source()), reconstruction.MeasureExpression(accepted)) {
		t.Fatal("accepted result did not improve the whole region")
	}
	proof, _ := result.Steps()[0].Conclusion.Get()
	if len(proof.Premises()) != 1 || !proof.Premises()[0].EqualsTo(a.Premise()) {
		t.Fatal("disappeared target lost its dependency")
	}
	if provider.queries != queries || provider.lookups != lookups || expressions.PrettyPrint(a.Source()) != before {
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
	if result.Expression() != source || len(result.Steps()) != 0 {
		t.Fatal("fixture must remain unchanged under the selected guard")
	}
	if _, present := must(a.Accept(result)).Get(); present {
		t.Fatal("unchanged result must be ordinary absence")
	}
}

func TestAcceptRejectsUncompletedOrForeignResults(t *testing.T) {
	a, result, _ := acceptanceFixture(t)
	other := requireAttempt(t, a.Rule(), a.Source(), a.Guard())
	for _, candidate := range []engine.RewriteResult{
		{},
		engine.NewRewriteResult(result.Expression(), result.Steps()),
		must(a.Visit(a.Source(), engine.RewriteContext{Attempt: a})),
		must(other.Rewrite()),
	} {
		got, err := a.Accept(candidate)
		if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
			t.Fatal("uncompleted or foreign construction accepted", err)
		}
	}
}

func TestAttemptOwnsGuardPath(t *testing.T) {
	a, result, _ := acceptanceFixture(t)
	guard := a.Guard()
	guard.Path[0] = 99
	guard.Expression = a.Source()
	if a.Guard().Path[0] != 0 {
		t.Fatal("guard accessor exposed attempt storage")
	}
	if _, present := must(a.Accept(result)).Get(); !present {
		t.Fatal("editing a guard copy changed acceptance")
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
	combined := engine.NewRewriteResult(
		guardLogical(foundation.And, second.Expression().Children()[0], first.Expression().Children()[1]),
		append(first.Steps(), second.Steps()...),
	)
	got, err := left.Accept(combined)
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
	summary, _ := must(rule.Extractor().Extract(guard)).Summary().Get()
	// Bypass selection to exercise acceptance's two-valued-guard boundary.
	a := requireAttempt(t, rule, source, engine.GuardCandidate{Path: engine.OccurrencePath{0}, Expression: guard, Summary: summary})
	got, err := a.Accept(must(a.Visit(a.Source(), engine.RewriteContext{Attempt: a})))
	if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("nullable guard accepted", err)
	}
	a, _, _ = acceptanceFixture(t)
	// Even a whole-root Visit result is tentative until Rewrite closes construction.
	got, err = a.Accept(must(a.Visit(a.Source(), engine.RewriteContext{Attempt: a})))
	if _, present := got.Get(); present || !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("unclosed traversal accepted", err)
	}
}

func TestRepeatedRewritesOwnEvidence(t *testing.T) {
	attempt, first, _ := acceptanceFixture(t)
	original := expressions.PrettyPrint(attempt.Source())
	second := must(attempt.Rewrite())
	if len(first.Steps()) == 0 || len(second.Steps()) == 0 || !first.Expression().EqualsTo(second.Expression()) {
		t.Fatal("expected repeatable rewrite with evidence")
	}
	steps := first.Steps()
	steps[0].Path[0] = 99
	steps[0].After = attempt.Source()
	if _, present := must(attempt.Accept(first)).Get(); !present {
		t.Fatal("editing copied evidence changed the completed result")
	}
	if _, present := must(attempt.Accept(second)).Get(); !present {
		t.Fatal("mutating one result affected another result")
	}
	if expressions.PrettyPrint(attempt.Source()) != original {
		t.Fatal("rewrite mutated source")
	}
}
