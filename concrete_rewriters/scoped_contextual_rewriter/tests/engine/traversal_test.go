package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"

	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"

	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type traversalContext struct {
	functioninterfaces.FunctionContext
	evaluations int
}

func (c *traversalContext) Evaluate(h functioninterfaces.FunctionHandle, s *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	c.evaluations++
	return c.FunctionContext.Evaluate(h, s, args)
}

type traversalEnvironment struct {
	x, a   *expressions.ReferenceExpression
	number int64
	truth  foundation.TruthValue
}

func (e traversalEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	if id.EqualsTo(e.x.ID()) {
		return foundation.Some[valueinterfaces.Value](guardInteger(e.number).Value()), nil
	}
	if e.a != nil && id.EqualsTo(e.a.ID()) {
		return foundation.Some(cleanupLiteral(e.truth, e.a.Type().Nullability()).Value()), nil
	}
	return foundation.Optional[valueinterfaces.Value]{}, errors.New("unexpected fixture column")
}

func assertTraversalEvidence(t *testing.T, a *engine.RewriteAttempt, result engine.RewriteResult) {
	t.Helper()
	if !result.Expression.Type().EqualsTo(a.Source.Type()) || must(a.Guard.Path.Resolve(result.Expression)) != a.Guard.Expression {
		t.Fatal("guard identity/path or whole Type changed")
	}
	for _, step := range result.Steps {
		if must(step.Path.Resolve(a.Source)) != step.Source || !step.Source.Type().EqualsTo(step.Before.Type()) || !step.Before.Type().EqualsTo(step.After.Type()) {
			t.Fatal("invalid intermediate source/Type")
		}
		if conclusion, present := step.Conclusion.Get(); present {
			if !conclusion.Scope().EqualsTo(a.Scope) || len(conclusion.Premises()) != 1 || !conclusion.Premises()[0].EqualsTo(a.Premise) {
				t.Fatal("foreign or lost dependency")
			}
		}
	}
}

func TestAttemptTraversalCoreCases(t *testing.T) {
	provider := &traversalContext{FunctionContext: must(testbed.NewStandardContext())}
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	flag := guardReference("a", must(types.NewBoolType(foundation.NonNull)))
	nullableFlag := guardReference("a", must(types.NewBoolType(foundation.MayNull)))
	g := membershipExpression(x, guardInteger(1), guardInteger(2))
	broad := membershipExpression(x, guardInteger(1), guardInteger(2), guardInteger(3))
	impossible := guardLogical(foundation.Not, guardComparison(provider, foundation.Equal, x, 3))
	rangeGuard := guardComparison(provider, foundation.LessThan, x, 10)
	rangeTarget := guardLogical(foundation.Not, guardComparison(provider, foundation.GreaterThanOrEqual, x, 20))
	orGuard := guardLogical(foundation.Or, guardComparison(provider, foundation.Equal, x, 1), guardComparison(provider, foundation.Equal, x, 2))
	truth := cleanupLiteral(foundation.True, foundation.NonNull)
	for _, tc := range []struct {
		name         string
		source, want *expressions.LogicalExpression
		path         engine.OccurrencePath
		flag         *expressions.ReferenceExpression
	}{
		{"SC01 finite whole proof", guardLogical(foundation.And, g, impossible), guardLogical(foundation.And, g, truth), engine.OccurrencePath{0}, flag},
		{"SC02 child proof then cleanup", guardLogical(foundation.And, g, guardLogical(foundation.Not, guardLogical(foundation.And, flag, broad))), guardLogical(foundation.And, g, guardLogical(foundation.Not, flag)), engine.OccurrencePath{0}, flag},
		{"SC02 nullable surviving child", guardLogical(foundation.And, g, guardLogical(foundation.Not, guardLogical(foundation.And, nullableFlag, broad))), guardLogical(foundation.And, g, guardLogical(foundation.Not, nullableFlag)), engine.OccurrencePath{0}, nullableFlag},
		{"absorbing evidence chain", guardLogical(foundation.And, g, guardLogical(foundation.Not, guardLogical(foundation.And, flag, guardComparison(provider, foundation.Equal, x, 3)))), guardLogical(foundation.And, g, truth), engine.OccurrencePath{0}, flag},
		{"SC03 range whole proof", guardLogical(foundation.And, rangeGuard, rangeTarget), guardLogical(foundation.And, rangeGuard, truth), engine.OccurrencePath{0}, flag},
		{"SC04 left protected ancestors", guardLogical(foundation.And, guardLogical(foundation.And, g, truth), impossible), guardLogical(foundation.And, guardLogical(foundation.And, g, truth), truth), engine.OccurrencePath{0, 0}, flag},
		{"SC04 right protected ancestors", guardLogical(foundation.And, flag, guardLogical(foundation.And, g, impossible)), guardLogical(foundation.And, flag, guardLogical(foundation.And, g, truth)), engine.OccurrencePath{1, 0}, flag},
		{"SC05 whole OR guard", guardLogical(foundation.And, orGuard, impossible), guardLogical(foundation.And, orGuard, truth), engine.OccurrencePath{0}, flag},
		{"SC06 shared guard", guardLogical(foundation.And, g, g), guardLogical(foundation.And, g, truth), engine.OccurrencePath{0}, flag},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var selected engine.GuardCandidate
			for _, guard := range must(rule.SelectGuards(tc.source)) {
				if guard.Path.EqualsTo(tc.path) {
					selected = guard
					break
				}
			}
			if selected.Summary == nil {
				t.Fatal("fixture guard not selected")
			}
			attempt := requireAttempt(t, rule, tc.source, selected)
			before := expressions.PrettyPrint(tc.source)
			calls := provider.evaluations
			result := must(attempt.Rewrite())
			if provider.evaluations != calls {
				t.Fatal("traversal evaluated expressions")
			}
			if !result.Expression.EqualsTo(tc.want) {
				t.Fatalf("unexpected result\n%s\nwant\n%s", expressions.PrettyPrint(result.Expression), expressions.PrettyPrint(tc.want))
			}
			if len(result.Steps) == 0 {
				t.Fatal("rewriting lost evidence")
			}
			if tc.name == "absorbing evidence chain" && len(result.Steps) != 3 {
				t.Fatal("absorption lost prior steps")
			}
			assertTraversalEvidence(t, attempt, result)
			accepted, present := must(attempt.Accept(result)).Get()
			if !present || accepted != result.Expression || provider.evaluations != calls {
				t.Fatal("complete improving traversal result was not accepted without evaluation")
			}
			if expressions.PrettyPrint(tc.source) != before {
				t.Fatal("source mutated")
			}
			if tc.name == "SC01 finite whole proof" && (len(result.Steps) != 1 || !result.Steps[0].Path.EqualsTo(engine.OccurrencePath{1})) {
				t.Fatal("did not prove whole target before descent")
			}
			evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
			for _, n := range []int64{-1, 0, 1, 2, 3, 9, 10, 20, 21} {
				for _, truth := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
					if truth == foundation.Null && tc.flag.Type().Nullability() == foundation.NonNull {
						continue
					}
					env := traversalEnvironment{x: x, a: tc.flag, number: n, truth: truth}
					original := must(evaluator.Evaluate(tc.source, env))
					rewritten := must(evaluator.Evaluate(result.Expression, env))
					expected := must(evaluator.Evaluate(tc.want, env))
					if !original.EqualsTo(rewritten) || !rewritten.EqualsTo(expected) {
						t.Fatalf("evaluation differs at x=%d A=%d", n, truth)
					}
				}
			}
		})
	}
}

func TestAttemptTraversalSharedTargetsAcrossScopes(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	shared := guardComparison(provider, foundation.Equal, x, 1)
	for _, n := range []int64{1, 2} {
		guard := guardComparison(provider, foundation.Equal, x, n)
		source := guardLogical(foundation.And, guard, shared)
		attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
		result := must(attempt.Rewrite())
		want := foundation.False
		if n == 1 {
			want = foundation.True
		}
		if !result.Expression.Children()[1].EqualsTo(cleanupLiteral(want, foundation.NonNull)) {
			t.Fatal("shared target reused another scope's truth")
		}
		assertTraversalEvidence(t, attempt, result)
	}
	if shared.Arguments()[0] != x {
		t.Fatal("shared source changed")
	}
}

func TestAttemptTraversalRejectsParentAndKeepsSibling(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := guardComparison(provider, foundation.Equal, x, 1)
	shared := guardComparison(provider, foundation.Equal, x, 1)
	boolean := must(types.NewBoolType(foundation.NonNull))
	failure := errors.New("foo(false) failed")
	calls := 0
	context, handle := changedChildrenFunction([]types.Type{boolean}, boolean, func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
		calls++
		if !args[0].(*testbedvalues.BoolValue).Bool() {
			return nil, failure
		}
		return args[0], nil
	})
	call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{shared}))
	source := guardLogical(foundation.And, guard, call, shared)
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	result := must(attempt.Rewrite())
	if calls != 0 || result.Expression.Children()[1] != call || len(result.Steps) != 1 || !result.Steps[0].Path.EqualsTo(engine.OccurrencePath{2}) {
		t.Fatal("parent rollback lost source or retained rejected evidence")
	}
	assertTraversalEvidence(t, attempt, result)
	if accepted, present := must(attempt.Accept(result)).Get(); !present || accepted != result.Expression {
		t.Fatal("complete result failed acceptance")
	}
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, expression := range []expressions.Expression{source, result.Expression} {
		if _, err := evaluator.Evaluate(expression, traversalEnvironment{x: x, number: 2}); !errors.Is(err, failure) {
			t.Fatal("false-guard failure suppressed", err)
		}
		if got := must(evaluator.Evaluate(expression, traversalEnvironment{x: x, number: 1})); !got.(*testbedvalues.BoolValue).Bool() {
			t.Fatal("true-guard result changed")
		}
	}
}

func TestAttemptTraversalDoesNotCreateNestedOwners(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	p := guardComparison(provider, foundation.Equal, x, 1)
	source := guardLogical(foundation.And, guard, guardLogical(foundation.And, p, p))
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	result := must(attempt.Rewrite())
	if result.Expression != source || len(result.Steps) != 0 {
		t.Fatal("nested owner introduced new assumptions")
	}
}

func TestAttemptTraversalErrorsReturnNoPartialResult(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	first := guardComparison(provider, foundation.Equal, x, 3)
	later := guardComparison(provider, foundation.GreaterThan, x, 10)
	source := guardLogical(foundation.And, guard, first, later)
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	sentinel := errors.New("later inspection failed")
	provider.metadata = func(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
		if handle.EqualsTo(later.Handle()) {
			return nil, sentinel
		}
		return provider.FunctionContext.Metadata(handle)
	}
	result, err := attempt.Rewrite()
	if !errors.Is(err, sentinel) || result.Expression != nil || len(result.Steps) != 0 {
		t.Fatal("error returned partial tree/evidence", err)
	}
	provider.metadata = nil
	other := requireAttempt(t, rule, source, attempt.Guard)
	for _, context := range []engine.RewriteContext{{Attempt: other}, {Attempt: attempt, Path: engine.OccurrencePath{99}}, {Attempt: attempt, Path: engine.OccurrencePath{1}}} {
		result, err := attempt.Visit(source, context)
		if !errors.Is(err, rewriter.ErrInvalidReplacement) || result.Expression != nil || len(result.Steps) != 0 {
			t.Fatal("bad occurrence association accepted", err)
		}
	}
}

func TestAttemptTraversalAdmitsSupportedEnclosingCall(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	predicate := guardComparison(provider, foundation.LessThan, x, 3)
	handle := must(provider.FunctionContext.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{predicate.Type()}))
	enclosing := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{predicate}))
	source := guardLogical(foundation.And, guard, enclosing)
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	result := must(attempt.Rewrite())
	expected := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{cleanupLiteral(foundation.True, foundation.NonNull)}))
	if !result.Expression.Children()[1].EqualsTo(expected) || len(result.Steps) != 1 || !result.Steps[0].Path.EqualsTo(engine.OccurrencePath{1, 0}) {
		t.Fatal("admitted enclosing call did not retain its child proof")
	}
	assertTraversalEvidence(t, attempt, result)
	if accepted, present := must(attempt.Accept(result)).Get(); !present || accepted != result.Expression {
		t.Fatal("complete result failed acceptance")
	}
}

func TestTraversalEvidenceTransferPreservesOwnershipAndOrder(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	flag := guardReference("flag", must(types.NewBoolType(foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	broad := membershipExpression(x, guardInteger(1), guardInteger(2), guardInteger(3))
	shared := guardLogical(foundation.Not, guardLogical(foundation.And, flag, broad))
	source := guardLogical(foundation.And, guard, shared, shared)
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	first := must(attempt.Rewrite())
	second := must(attempt.Rewrite())
	paths := []engine.OccurrencePath{{1, 0, 1}, {1, 0}, {2, 0, 1}, {2, 0}}
	if len(first.Steps) != len(paths) || len(second.Steps) != len(paths) {
		t.Fatal("lost child or cleanup evidence")
	}
	for i, path := range paths {
		if !first.Steps[i].Path.EqualsTo(path) || !second.Steps[i].Path.EqualsTo(path) {
			t.Fatal("evidence order changed")
		}
	}
	// Mutating a caller-owned result must not affect another invocation, another
	// occurrence of the shared source, or the completed receipt's frozen evidence.
	first.Steps[0].Path[0] = 99
	first.Steps[1].Source = nil
	for i, path := range paths {
		if !second.Steps[i].Path.EqualsTo(path) || second.Steps[i].Source == nil {
			t.Fatal("separate rewrite results share evidence")
		}
	}
	if !first.Steps[2].Path.EqualsTo(paths[2]) {
		t.Fatal("shared source occurrences share paths")
	}
	if _, err := attempt.Accept(first); !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatalf("mutated receipt accepted: %v", err)
	}
	if _, present := must(attempt.Accept(second)).Get(); !present {
		t.Fatal("independent receipt no longer accepted")
	}
}
