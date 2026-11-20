package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func guardInteger(value int64) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(must(types.NewIntType(types.Int64, foundation.NonNull)), value))))
}

func guardComparison(provider functioninterfaces.FunctionContext, op foundation.OperatorType, ref *expressions.ReferenceExpression, value int64) *expressions.CallExpression {
	literal := guardInteger(value)
	handle := must(provider.ComparisonFunction(op, ref.Type(), literal.Type()))
	return must(expressions.NewCallExpression(provider, handle, []expressions.Expression{ref, literal}))
}

func checkSelectedGuards(t *testing.T, source *expressions.LogicalExpression, got []engine.GuardCandidate, paths []engine.OccurrencePath) {
	t.Helper()
	if len(got) != len(paths) {
		t.Fatalf("selected %d guards, want %d", len(got), len(paths))
	}
	for i, g := range got {
		expected := must(paths[i].Resolve(source))
		if !g.Path.EqualsTo(paths[i]) || g.Expression != expected || g.Summary.Source() != expected || !g.Summary.Type().EqualsTo(expected.Type()) {
			t.Fatalf("guard %d lost path, complete source or Type association", i)
		}
	}
}

func TestSelectGuardsCompleteConjuncts(t *testing.T) {
	context := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, context)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	comparison := guardComparison(context, foundation.Equal, x, 1)
	membership := membershipExpression(x, guardInteger(1), guardInteger(2))
	disjunction := guardLogical(foundation.Or, comparison, guardComparison(context, foundation.Equal, x, 2))
	negation := guardLogical(foundation.Not, guardComparison(context, foundation.Equal, x, 3))
	source := guardLogical(foundation.And, guardLogical(foundation.And, comparison, membership), disjunction, negation)
	before := expressions.PrettyPrint(source)
	got := must(rule.SelectGuards(source))
	checkSelectedGuards(t, source, got, []engine.OccurrencePath{{0, 0}, {0, 1}, {1}, {2}})
	for _, g := range got {
		_, hasNullTruth := g.Summary.NullTruth().Get()
		if g.Summary.Operand() != x || hasNullTruth {
			t.Fatal("operand or nullability changed")
		}
	}
	if expressions.PrettyPrint(source) != before {
		t.Fatal("selection mutated source")
	}
	// A shared object remains two independent occurrences with separate path storage.
	shared := guardLogical(foundation.And, comparison, comparison)
	selected := must(rule.SelectGuards(shared))
	checkSelectedGuards(t, shared, selected, []engine.OccurrencePath{{0}, {1}})
	selected[0].Path[0] = 99
	if !selected[1].Path.EqualsTo(engine.OccurrencePath{1}) {
		t.Fatal("guard paths alias")
	}
	checkSelectedGuards(t, shared, must(rule.SelectGuards(shared)), []engine.OccurrencePath{{0}, {1}})
	if got := must(rule.SelectGuards(disjunction)); len(got) != 0 {
		t.Fatal("non-AND root selected guards")
	}
}

func TestSelectGuardsSkipsIneligibleCandidates(t *testing.T) {
	context := membershipEligibilityProvider()
	rule := engine.New(testbedvalues.ValueFactory{}, context)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	x, y := guardReference("x", integer), guardReference("y", integer)
	first := guardComparison(context, foundation.Equal, x, 1)
	second := membershipExpression(x, guardInteger(1), guardInteger(2))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "hidden")), must(types.NewBoolType(foundation.NonNull)), "x"))
	mixed := guardLogical(foundation.Or, first, guardComparison(context, foundation.Equal, y, 2))
	unsupported := guardLogical(foundation.Or, first, opaque)
	raw := guardReference("flag", must(types.NewBoolType(foundation.NonNull)))
	source := guardLogical(foundation.And, opaque, mixed, unsupported, raw, first, second)
	checkSelectedGuards(t, source, must(rule.SelectGuards(source)), []engine.OccurrencePath{{4}, {5}})
	// A matching reference in another conjunct is required, even for exact predicates.
	unrelated := guardLogical(foundation.And, first, guardComparison(context, foundation.Equal, y, 2))
	checkSelectedGuards(t, unrelated, must(rule.SelectGuards(unrelated)), nil)
	// Nullable predicates cannot become guards just because passing rows are non-null.
	nullable := guardReference("n", must(types.NewIntType(types.Int64, foundation.MayNull)))
	nullableSource := guardLogical(foundation.And, guardComparison(context, foundation.Equal, nullable, 1), membershipExpression(nullable, guardInteger(2)))
	context.queries, context.lookups = 0, 0
	checkSelectedGuards(t, nullableSource, must(rule.SelectGuards(nullableSource)), nil)
	if context.queries != 0 || context.lookups != 0 {
		t.Fatal("nullable candidates reached extraction")
	}
}

func TestSelectGuardsKeepsLaterAlternatives(t *testing.T) {
	context := membershipEligibilityProvider()
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	// x > 0 cannot fix the truth of either stronger predicate. A later guard can
	// prove earlier predicates true; selection must not stop at the first match.
	source := guardLogical(foundation.And, guardComparison(context, foundation.GreaterThan, x, 0), guardComparison(context, foundation.GreaterThan, x, 5), guardComparison(context, foundation.GreaterThan, x, 10))
	checkSelectedGuards(t, source, must(engine.New(testbedvalues.ValueFactory{}, context).SelectGuards(source)), []engine.OccurrencePath{{0}, {1}, {2}})
}

func TestSelectGuardsPropagatesProviderErrors(t *testing.T) {
	for _, stage := range []string{"comparison metadata", "membership lookup"} {
		t.Run(stage, func(t *testing.T) {
			context := membershipEligibilityProvider()
			x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
			first := guardComparison(context, foundation.Equal, x, 1)
			later := guardComparison(context, foundation.Equal, x, 2)
			var target expressions.Expression = later
			sentinel := errors.New("injected selection failure")
			if stage == "comparison metadata" {
				context.metadata = func(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
					if handle.EqualsTo(later.Handle()) {
						return nil, sentinel
					}
					return context.FunctionContext.Metadata(handle)
				}
				// Use a different operator for the first candidate, so it is selected
				// successfully before the later handle's metadata fails.
				first = guardComparison(context, foundation.LessThan, x, 10)
			} else {
				target = membershipExpression(x, guardInteger(2))
				context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
					return nil, sentinel
				}
			}
			source := guardLogical(foundation.And, first, target)
			got, err := engine.New(testbedvalues.ValueFactory{}, context).SelectGuards(source)
			if got != nil || !errors.Is(err, sentinel) {
				t.Fatalf("selection = %v, %v; want no partial result and provider error", got, err)
			}
		})
	}
}

func TestSelectGuardsScalarMembershipTypes(t *testing.T) {
	for _, typ := range []types.Type{
		must(types.NewBoolType(foundation.NonNull)),
		must(types.NewIntType(types.Int64, foundation.NonNull)),
		must(types.NewFloatType(types.Float32, foundation.NonNull)),
		must(types.NewFloatType(types.Float64, foundation.NonNull)),
		must(types.NewVarCharType(nil, foundation.NonNull)),
	} {
		t.Run(typ.String(), func(t *testing.T) {
			context := membershipEligibilityProvider()
			ref := guardReference("x", typ)
			candidate := membershipExpression(ref, membershipCandidate(typ))
			source := guardLogical(foundation.And, candidate, candidate)
			checkSelectedGuards(t, source, must(engine.New(testbedvalues.ValueFactory{}, context).SelectGuards(source)), []engine.OccurrencePath{{0}, {1}})
		})
	}
}

func TestSelectGuardsUnsupportedLookupIsSkipped(t *testing.T) {
	context := membershipEligibilityProvider()
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	comparison := guardComparison(context, foundation.Equal, x, 1)
	source := guardLogical(foundation.And, membershipExpression(x, guardInteger(1)), comparison)
	context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		return nil, functions.ErrUnsupportedLookup
	}
	checkSelectedGuards(t, source, must(engine.New(testbedvalues.ValueFactory{}, context).SelectGuards(source)), []engine.OccurrencePath{{1}})
}
