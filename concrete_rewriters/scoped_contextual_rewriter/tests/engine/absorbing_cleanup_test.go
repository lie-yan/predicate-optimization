package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestAbsorbingCleanupAdmittedOperands(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	x := guardReference("x", integer)
	nullableX := guardReference("n", must(integer.WithNullability(foundation.MayNull)))
	array := must(types.NewArrayType(integer, foundation.MayNull))
	candidates := eligibilityArray(must(types.NewArrayType(integer, foundation.NonNull)), 1, 2)
	arrayRef := guardReference("arr", array)
	var arrayCalls []expressions.Expression
	for _, name := range []functions.BuiltInFunctionName{functions.BuiltInArrayContains, functions.BuiltInArrayContainsAny} {
		var candidate expressions.Expression = guardInteger(1)
		if name == functions.BuiltInArrayContainsAny {
			candidate = candidates
		}
		handle := must(provider.FunctionContext.LookupBuiltInFunction(name, []types.Type{array, candidate.Type()}))
		arrayCalls = append(arrayCalls, must(expressions.NewCallExpression(provider, handle, []expressions.Expression{arrayRef, candidate})))
	}
	operands := []expressions.Expression{
		guardComparison(provider, foundation.Equal, x, 1),
		guardComparison(provider, foundation.LessThan, nullableX, 10),
		membershipExpression(x, guardInteger(1), guardInteger(2)),
		guardLogical(foundation.Not, guardReference("flag", must(types.NewBoolType(foundation.MayNull)))),
		guardLogical(foundation.And, arrayCalls...),
	}
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		decisive := foundation.False
		if op == foundation.Or {
			decisive = foundation.True
		}
		for _, operand := range operands {
			source := guardLogical(op, operand, cleanupLiteral(decisive, foundation.NonNull))
			before := expressions.PrettyPrint(source)
			result, present := must(rule.CleanupLogical(source, source.Children())).Get()
			if !present || !result.EqualsTo(must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, source.Type().(*types.BoolType), decisive))) || result.Type() != source.Type() {
				t.Fatal("wrong absorbing result or declared Type", source)
			}
			if expressions.PrettyPrint(source) != before {
				t.Fatal("source mutated")
			}
		}
	}
}

func TestAbsorbingCleanupPreservesFailingComputations(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, membershipEligibilityProvider())
	boolean := must(types.NewBoolType(foundation.NonNull))
	sentinel := errors.New("unknown function failed")
	evaluations := 0
	provider, handle := changedChildrenFunction([]types.Type{boolean}, boolean, func([]valueinterfaces.Value) (valueinterfaces.Value, error) { evaluations++; return nil, sentinel })
	failure := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{cleanupLiteral(foundation.True, foundation.NonNull)}))
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		neutral, decisive := foundation.True, foundation.False
		if op == foundation.Or {
			neutral, decisive = foundation.False, foundation.True
		}
		nested := guardLogical(foundation.Not, failure)
		source := guardLogical(op, cleanupLiteral(neutral, foundation.NonNull), nested, cleanupLiteral(decisive, foundation.NonNull))
		before := evaluations
		result, present := must(rule.CleanupLogical(source, source.Children())).Get()
		if evaluations != before {
			t.Fatal("cleanup evaluated the failing function")
		}
		if !present || len(result.Children()) != 2 || result.Children()[0] != nested {
			t.Fatal("neutral fallback did not retain failing operand")
		}
		evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
		for _, expression := range []expressions.Expression{source, result} {
			if _, err := evaluator.Evaluate(expression, changedChildrenEnvironment{}); !errors.Is(err, sentinel) {
				t.Fatal("failure suppressed or changed", err)
			}
		}
	}
}

func TestAbsorbingCleanupInspectsAllOccurrencesAndPropagatesErrors(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	comparison := guardComparison(provider, foundation.Equal, x, 1)
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "opaque")), must(types.NewBoolType(foundation.NonNull)), "opaque"))
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		decisive := foundation.False
		if op == foundation.Or {
			decisive = foundation.True
		}
		constant := cleanupLiteral(decisive, foundation.NonNull)
		source := guardLogical(op, constant, comparison, comparison)
		provider.queries = 0
		if _, present := must(rule.CleanupLogical(source, source.Children())).Get(); !present || provider.queries != 2 {
			t.Fatal("shared occurrences not inspected separately", provider.queries)
		}
		sentinel := errors.New("later metadata failure")
		provider.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, sentinel }
		source = guardLogical(op, opaque, comparison, constant)
		result, err := rule.CleanupLogical(source, source.Children())
		if _, present := result.Get(); present || !errors.Is(err, sentinel) {
			t.Fatal("ineligible sibling hid later failure", err)
		}
		provider.metadata = nil
	}
	// Inspect proposed children: an original safe read may have been replaced by
	// a Type-compatible but unsupported computation. Its evaluation must remain.
	safe := guardReference("flag", opaque.Type())
	source := guardLogical(foundation.And, safe, cleanupLiteral(foundation.False, foundation.NonNull))
	if _, present := must(rule.CleanupLogical(source, []expressions.Expression{opaque, source.Children()[1]})).Get(); present {
		t.Fatal("admitted original instead of proposed computation")
	}
}

func TestAbsorbingCleanupRetainsScopedDependencies(t *testing.T) {
	provider := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, provider)
	cleaner := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	target := guardComparison(provider, foundation.Equal, x, 3)
	a := guardReference("a", must(types.NewBoolType(foundation.NonNull)))
	group := guardLogical(foundation.And, a, target)
	negation := guardLogical(foundation.Not, group)
	source := guardLogical(foundation.And, guard, negation)
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	proof, present := must(attempt.ProveLiteral(target, engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1, 0, 1}})).Get()
	if !present {
		t.Fatal("expected False under guard")
	}
	children := []expressions.Expression{a, proof.Expression()}
	absorbed, present := must(cleaner.CleanupLogical(group, children)).Get()
	if !present {
		t.Fatal("eligible A AND False not absorbed")
	}
	absent := foundation.Optional[scoped_reasoning.ScopedConclusion]{}
	absorbStep := must(attempt.NewStep(engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1, 0}}, group, must(group.WithChildren(children)), absorbed, absent))
	cleaned, present := must(cleaner.CleanupLogical(negation, []expressions.Expression{absorbed})).Get()
	if !present || !cleaned.EqualsTo(cleanupLiteral(foundation.True, foundation.NonNull)) {
		t.Fatal("NOT cleanup missing")
	}
	notStep := must(attempt.NewStep(engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1}}, negation, must(negation.WithChildren([]expressions.Expression{absorbed})), cleaned, absent))
	result := engine.NewRewriteResult(cleaned, append(proof.Steps(), absorbStep, notStep))
	conclusion, _ := result.Steps()[0].Conclusion.Get()
	if len(result.Steps()) != 3 || len(conclusion.Premises()) != 1 || !conclusion.Premises()[0].EqualsTo(attempt.Premise()) {
		t.Fatal("absorbing cleanup lost the scoped dependency")
	}
}
