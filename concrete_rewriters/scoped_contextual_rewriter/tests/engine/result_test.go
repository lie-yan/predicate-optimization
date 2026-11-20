package engine_test

import (
	"errors"
	"testing"

	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/types"
)

func stepFixture(t *testing.T) (*engine.RewriteAttempt, *expressions.CallExpression, scoped_reasoning.ScopedConclusion) {
	t.Helper()
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	target := guardComparison(provider, foundation.Equal, x, 3)
	source := guardLogical(foundation.And, guard, guardLogical(foundation.Not, target))
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	analysis := must(rule.Extractor.Extract(target))
	summary, _ := analysis.Summary().Get()
	proof, present := must(scoped_reasoning.ProveTruth(summary, attempt.Input)).Get()
	if !present {
		t.Fatal("expected uniform false proof")
	}
	return attempt, target, proof
}

func TestRewriteStepRecordsOriginalAndRebuiltExpressions(t *testing.T) {
	a, target, proof := stepFixture(t)
	falseLiteral := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, target.Type().(*types.BoolType), foundation.False))
	context := engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}
	substitution := must(a.NewStep(context, target, target, falseLiteral, foundation.Some(proof)))
	context.Path[0] = 99
	if !substitution.Path.EqualsTo(engine.OccurrencePath{1, 0}) || substitution.Source != target || substitution.Before != target || substitution.After != falseLiteral {
		t.Fatal("step lost association or retained caller path")
	}
	if p, present := substitution.Conclusion.Get(); !present || !p.Premises()[0].EqualsTo(a.Premise) {
		t.Fatal("step lost guard dependency")
	}

	original := must((engine.OccurrencePath{1}).Resolve(a.Source))
	rebuilt := must(original.WithChildren([]expressions.Expression{falseLiteral}))
	trueLiteral := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, original.Type().(*types.BoolType), foundation.True))
	// Simulate the future fixed NOT cleanup, whose identity is NOT False = True.
	cleanup := must(a.NewStep(engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1}}, original, rebuilt, trueLiteral, foundation.Optional[scoped_reasoning.ScopedConclusion]{}))
	if cleanup.Source == cleanup.Before || cleanup.Source != original || cleanup.Before != rebuilt {
		t.Fatal("original source confused with rebuilt before")
	}
	if _, present := cleanup.Conclusion.Get(); present {
		t.Fatal("cleanup fabricated a scoped conclusion")
	}
	result := engine.NewRewriteResult(trueLiteral, []engine.RewriteStep{substitution, cleanup})
	if len(result.Steps) != 2 {
		t.Fatal("cleanup lost earlier substitution evidence")
	}
	if p, _ := result.Steps[0].Conclusion.Get(); !p.Premises()[0].EqualsTo(a.Premise) {
		t.Fatal("disappearing target syntax lost premise")
	}
}

func TestRewriteStepRejectsBadAssociations(t *testing.T) {
	a, target, proof := stepFixture(t)
	replacement := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, target.Type().(*types.BoolType), foundation.False))
	// Construct a distinct equal call; structural equality cannot replace source association.
	equalTarget := must(expressions.NewCallExpression(target.Context(), target.Handle(), target.Arguments()))
	otherAttempt := requireAttempt(t, a.Rule, a.Source, a.Guard)
	foreignProof, _ := must(scoped_reasoning.ProveTruth(proof.Summary(), otherAttempt.Input)).Get()
	for _, tc := range []struct {
		name                  string
		context               engine.RewriteContext
		source, before, after expressions.Expression
		proof                 scoped_reasoning.ScopedConclusion
	}{
		{"foreign owner", engine.RewriteContext{Attempt: otherAttempt, Path: engine.OccurrencePath{1, 0}}, target, target, replacement, proof},
		{"invalid path", engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{5}}, target, target, replacement, proof},
		{"stale equal source", engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, equalTarget, equalTarget, replacement, proof},
		{"rebuilt before for original proof", engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, equalTarget, replacement, proof},
		{"wrong literal", engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, target, must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, target.Type().(*types.BoolType), foundation.True)), proof},
		{"wrong Type", engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, target, must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, must(types.NewBoolType(foundation.MayNull)), foundation.False)), proof},
		{"foreign scope", engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, target, replacement, foreignProof},
		{"nil source", engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, nil, target, replacement, proof},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := a.NewStep(tc.context, tc.source, tc.before, tc.after, foundation.Some(tc.proof))
			if !errors.Is(err, rewriter.ErrInvalidReplacement) {
				t.Fatalf("expected invalid replacement, got %v", err)
			}
		})
	}
	// An equal but distinct summary source cannot certify this source occurrence.
	summary, _ := must(a.Rule.Extractor.Extract(equalTarget)).Summary().Get()
	staleProof, _ := must(scoped_reasoning.ProveTruth(summary, a.Input)).Get()
	if _, err := a.NewStep(engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, target, replacement, foundation.Some(staleProof)); !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("stale proof accepted", err)
	}
	// A restriction is not a KnownTruth substitution, even under the same scope.
	restriction, _ := must(scoped_reasoning.RestrictToTrue(a.Guard.Summary, a.Input, a.Premise)).Get()
	if _, err := a.NewStep(engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, target, replacement, foundation.Some(restriction)); !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("non-truth conclusion accepted", err)
	}
}

func TestRewriteResultOwnsEvidence(t *testing.T) {
	a, target, proof := stepFixture(t)
	replacement := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, target.Type().(*types.BoolType), foundation.False))
	step := must(a.NewStep(engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, target, replacement, foundation.Some(proof)))
	steps := make([]engine.RewriteStep, 1, 4)
	steps[0] = step
	left, right := engine.NewRewriteResult(replacement, steps), engine.NewRewriteResult(replacement, steps)
	steps[0].Path[0] = 99
	steps[0].After = target
	left.Steps[0].Path[0] = 98
	left.Steps = append(left.Steps, step)
	if len(right.Steps) != 1 || !right.Steps[0].Path.EqualsTo(engine.OccurrencePath{1, 0}) || right.Steps[0].After != replacement {
		t.Fatal("results share mutable evidence storage")
	}
	// A later traversal can reject one child without changing the sibling result.
	rejected := engine.NewRewriteResult(target, nil)
	if rejected.Expression != target || len(rejected.Steps) != 0 || len(right.Steps) != 1 {
		t.Fatal("discarded changes contaminated retained sibling")
	}
}

func TestRewriteStepRejectsForeignPremise(t *testing.T) {
	a, target, proof := stepFixture(t)
	owner := scoped_reasoning.NewScopedFactContext()
	scope := owner.NewScope()
	first := must(owner.RegisterPremise(scope, a.Guard.Expression))
	second := must(owner.RegisterPremise(scope, a.Guard.Expression))
	local := *a
	local.Scope = scope
	local.Premise = first
	local.Input = must(scoped_reasoning.NewInputDomain(scope, a.Input.Operand(), a.Input.PossibleValues(), a.Input.NullPossible(), []*scoped_reasoning.Premise{first}))
	conclusion, _ := must(scoped_reasoning.ProveTruth(proof.Summary(), local.Input)).Get()
	replacement := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, target.Type().(*types.BoolType), foundation.False))
	context := engine.RewriteContext{Attempt: &local, Path: engine.OccurrencePath{1, 0}}
	must(local.NewStep(context, target, target, replacement, foundation.Some(conclusion)))
	// Corrupt just the expected premise: the scope, source, domain and truth match.
	local.Premise = second
	if _, err := local.NewStep(context, target, target, replacement, foundation.Some(conclusion)); !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("foreign same-scope premise accepted", err)
	}
}

func TestRewriteStepProtectsGuardAndCleanupShape(t *testing.T) {
	a, target, _ := stepFixture(t)
	absent := foundation.Optional[scoped_reasoning.ScopedConclusion]{}
	for _, path := range []engine.OccurrencePath{nil, {0}, {0, 0}} {
		source := must(path.Resolve(a.Source))
		if _, err := a.NewStep(engine.RewriteContext{Attempt: a, Path: path}, source, source, source, absent); !errors.Is(err, rewriter.ErrInvalidReplacement) {
			t.Fatal("protected occurrence admitted a local step", path, err)
		}
	}
	if _, err := a.NewStep(engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}}, target, target, target, absent); !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("non-logical cleanup accepted", err)
	}
	source := must((engine.OccurrencePath{1}).Resolve(a.Source))
	changedForm := guardLogical(foundation.Or, target, target)
	if _, err := a.NewStep(engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1}}, source, changedForm, target, absent); !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("changed logical form before cleanup accepted", err)
	}
}
