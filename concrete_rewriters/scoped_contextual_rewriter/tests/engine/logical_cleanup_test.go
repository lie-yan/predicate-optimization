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
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	"github.com/lie-yan/predicate-optimization/types"
)

func cleanupLiteral(truth foundation.TruthValue, nullability foundation.Nullability) *expressions.LiteralExpression {
	return must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, must(types.NewBoolType(nullability)), truth))
}

func TestCleanupLogicalTruthTables(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	truths := []foundation.TruthValue{foundation.False, foundation.True, foundation.Null}
	andTable := [3][3]foundation.TruthValue{{foundation.False, foundation.False, foundation.False}, {foundation.False, foundation.True, foundation.Null}, {foundation.False, foundation.Null, foundation.Null}}
	orTable := [3][3]foundation.TruthValue{{foundation.False, foundation.True, foundation.Null}, {foundation.True, foundation.True, foundation.True}, {foundation.Null, foundation.True, foundation.Null}}
	notTable := []foundation.TruthValue{foundation.True, foundation.False, foundation.Null}
	check := func(source *expressions.LogicalExpression, want foundation.TruthValue) {
		t.Helper()
		result, present := must(rule.CleanupLogical(source, source.Children())).Get()
		expected := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, source.Type().(*types.BoolType), want))
		if !present || !result.EqualsTo(expected) || result.Type() != source.Type() {
			t.Fatalf("%s: wrong truth/Type", source)
		}
		// The independent interpreter must agree with the explicit expected table.
		got := must(interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(source, changedChildrenEnvironment{}))
		if !got.EqualsTo(expected.Value()) {
			t.Fatal("interpreter disagrees with expected truth", source)
		}
	}
	for _, nullable := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for i, left := range truths {
			if left == foundation.Null && nullable == foundation.NonNull {
				continue
			}
			l := cleanupLiteral(left, nullable)
			check(guardLogical(foundation.Not, l), notTable[i])
			for j, right := range truths {
				if right == foundation.Null && nullable == foundation.NonNull {
					continue
				}
				r := cleanupLiteral(right, nullable)
				check(guardLogical(foundation.And, l, r), andTable[i][j])
				check(guardLogical(foundation.Or, l, r), orTable[i][j])
			}
		}
	}
	// Decisive values must dominate NULL in either position, including n-ary groups.
	check(guardLogical(foundation.And, cleanupLiteral(foundation.False, foundation.NonNull), cleanupLiteral(foundation.Null, foundation.MayNull), cleanupLiteral(foundation.True, foundation.NonNull)), foundation.False)
	check(guardLogical(foundation.Or, cleanupLiteral(foundation.Null, foundation.MayNull), cleanupLiteral(foundation.True, foundation.NonNull), cleanupLiteral(foundation.False, foundation.NonNull)), foundation.True)
	if provider.queries != 0 || provider.lookups != 0 {
		t.Fatal("literal cleanup queried providers")
	}
}

func TestCleanupLogicalNeutralSurvivorsAndTypes(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, membershipEligibilityProvider())
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		neutral := foundation.True
		if op == foundation.Or {
			neutral = foundation.False
		}
		literal := cleanupLiteral(neutral, foundation.NonNull)
		nullableLiteral := cleanupLiteral(neutral, foundation.MayNull)
		a := guardReference("a", must(types.NewBoolType(foundation.NonNull)))
		b := guardReference("b", must(types.NewBoolType(foundation.MayNull)))
		nested := guardLogical(op, a, b)
		source := guardLogical(op, literal, b, literal, nested, literal, a)
		before := expressions.PrettyPrint(source)
		result, present := must(rule.CleanupLogical(source, source.Children())).Get()
		if !present || !result.Type().EqualsTo(source.Type()) {
			t.Fatal("missing neutral cleanup")
		}
		children := result.Children()
		if len(children) != 3 || children[0] != b || children[1] != nested || children[2] != a {
			t.Fatal("survivor order/grouping changed")
		}
		for _, tc := range []struct {
			source  *expressions.LogicalExpression
			want    expressions.Expression
			present bool
		}{
			{guardLogical(op, a, literal), a, true},
			{guardLogical(op, b, nullableLiteral), b, true},
			{guardLogical(op, a, nullableLiteral), nil, false},
			{guardLogical(op, a, literal, a), nil, true},
			{guardLogical(op, a, b), nil, false},
		} {
			got, ok := must(rule.CleanupLogical(tc.source, tc.source.Children())).Get()
			if ok != tc.present || ok && tc.want != nil && got != tc.want {
				t.Fatal("incorrect survivor/Type handling", tc.source)
			}
		}
		narrowing := guardLogical(op, nullableLiteral, a, a)
		if _, ok := must(rule.CleanupLogical(narrowing, narrowing.Children())).Get(); ok {
			t.Fatal("multi-survivor Type narrowing accepted")
		}
		if expressions.PrettyPrint(source) != before {
			t.Fatal("source changed")
		}
	}
}

func TestCleanupLogicalRetainsNonliteralComputations(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, membershipEligibilityProvider())
	boolean := must(types.NewBoolType(foundation.NonNull))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "effect")), boolean, "unknown computation"))
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		neutral, decisive := foundation.True, foundation.False
		if op == foundation.Or {
			neutral, decisive = foundation.False, foundation.True
		}
		decisiveLiteral := cleanupLiteral(decisive, foundation.NonNull)
		original := guardLogical(op, opaque, decisiveLiteral)
		if _, present := must(rule.CleanupLogical(original, original.Children())).Get(); present {
			t.Fatal("absorbing cleanup removed an opaque computation")
		}
		withNeutral := guardLogical(op, cleanupLiteral(neutral, foundation.NonNull), opaque, decisiveLiteral)
		result, present := must(rule.CleanupLogical(withNeutral, withNeutral.Children())).Get()
		if !present || !result.EqualsTo(original) {
			t.Fatal("neutral cleanup lost nonliteral evaluation")
		}
	}
	source := guardLogical(foundation.Not, opaque)
	if _, present := must(rule.CleanupLogical(source, source.Children())).Get(); present {
		t.Fatal("nonliteral NOT evaluated")
	}
}

func TestCleanupLogicalChangedChildrenAndEvidence(t *testing.T) {
	a, target, _ := stepFixture(t)
	cleaner := boolean_cleanup.New(testbedvalues.ValueFactory{}, membershipEligibilityProvider())
	proven, present := must(a.ProveLiteral(target, engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1, 0}})).Get()
	if !present {
		t.Fatal("expected scoped proof")
	}
	original := must((engine.OccurrencePath{1}).Resolve(a.Source)).(*expressions.LogicalExpression)
	children := []expressions.Expression{proven.Expression}
	candidate, present := must(cleaner.CleanupLogical(original, children)).Get()
	if !present || !candidate.EqualsTo(cleanupLiteral(foundation.True, foundation.NonNull)) {
		t.Fatal("NOT False cleanup missing")
	}
	rebuilt := must(original.WithChildren(children))
	step := must(a.NewStep(engine.RewriteContext{Attempt: a, Path: engine.OccurrencePath{1}}, original, rebuilt, candidate, foundation.Optional[scoped_reasoning.ScopedConclusion]{}))
	result := engine.NewRewriteResult(candidate, append(proven.Steps, step))
	proof, ok := result.Steps[0].Conclusion.Get()
	if len(result.Steps) != 2 || !ok || !proof.Premises()[0].EqualsTo(a.Premise) {
		t.Fatal("cleanup dropped earlier dependency")
	}
	// A refused cleanup still permits rebuilding with already-justified children.
	nullableSource := guardLogical(foundation.And, guardReference("a", must(types.NewBoolType(foundation.NonNull))), guardReference("b", must(types.NewBoolType(foundation.MayNull))))
	proposed := []expressions.Expression{nullableSource.Children()[0], cleanupLiteral(foundation.True, foundation.MayNull)}
	if _, ok := must(cleaner.CleanupLogical(nullableSource, proposed)).Get(); ok {
		t.Fatal("narrowed Type accepted")
	}
	if got := must(nullableSource.WithChildren(proposed)); !got.Type().EqualsTo(nullableSource.Type()) {
		t.Fatal("valid child rebuilding failed")
	}
	for _, bad := range [][]expressions.Expression{nil, {cleanupLiteral(foundation.False, foundation.MayNull)}} {
		result, err := cleaner.CleanupLogical(original, bad)
		if _, ok := result.Get(); ok || !errors.Is(err, rewriter.ErrInvalidReplacement) {
			t.Fatal("invalid child contract not reported", err)
		}
	}
}
