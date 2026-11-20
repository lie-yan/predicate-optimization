package scoped_contextual_rewriter_test

import (
	"errors"
	"testing"

	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestScalarDomainsIndependentOwnersAndProviders(t *testing.T) {
	for _, d := range scalarDomains() {
		t.Run(d.name, func(t *testing.T) {
			membershipProvider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			guardProvider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			targetProvider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			x := reference("x", d.typ)
			// Equal IDs from distinct reference objects must match, while the same target
			// instance can be proven true, false, or left untouched at different paths.
			alias := reference("x", d.typ)
			shared := scalarComparison(targetProvider, foundation.Equal, alias, d.low)
			low := scalarComparison(guardProvider, foundation.Equal, x, d.low)
			high := scalarComparison(guardProvider, foundation.Equal, x, d.high)
			source := logical(foundation.Or, logical(foundation.And, low, shared), logical(foundation.And, high, shared), shared)
			want := logical(foundation.Or, low, shared)
			stages := rewriteScalarFixture(t, membershipProvider, source, want)
			intermediate := logical(foundation.Or, logical(foundation.And, low, booleanLiteral(foundation.True, foundation.NonNull)), logical(foundation.And, high, booleanLiteral(foundation.False, foundation.NonNull)), shared)
			if !stages[1].EqualsTo(intermediate) || stages[1].Children()[2] != shared || stages[2].Children()[1] != shared {
				t.Fatal("owner facts escaped to a shared occurrence")
			}
			if guardProvider.evaluations != 0 || targetProvider.evaluations != 0 {
				t.Fatal("call contexts evaluated during optimization")
			}
			rows := make([]contextualRow, len(d.samples))
			truths := make([]foundation.TruthValue, len(rows))
			for j, v := range d.samples {
				rows[j] = contextualRow{{x, v}}
				truths[j] = truthOf(nativeEqual(v, d.low))
			}
			assertScalarRows(t, membershipProvider, stages, rows, truths)
			if guardProvider.evaluations == 0 || targetProvider.evaluations == 0 {
				t.Fatal("oracle did not use the paired call contexts")
			}
		})
	}
}

// Unlike contextualRow, absence is intentional in this partial-input fixture.
type partialScalarRow contextualRow

func (row partialScalarRow) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for _, binding := range row {
		if id.EqualsTo(binding.column.ID()) {
			return foundation.Some(binding.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

func TestScalarDomainsUnavailableInputIsNotNull(t *testing.T) {
	for _, d := range scalarDomains() {
		t.Run(d.name, func(t *testing.T) {
			provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			x := reference("x", d.typ)
			a := reference("a", must(types.NewBoolType(foundation.MayNull)))
			guard := scalarMembership(x, d.low)
			impossible := scalarComparison(provider, foundation.Equal, x, d.high)
			source := logical(foundation.And, guard, logical(foundation.Or, a, impossible))
			stages := rewriteScalarFixture(t, provider, source, logical(foundation.And, guard, a))
			evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
			missing := partialScalarRow{{x, d.low}}
			knownNull := partialScalarRow{{x, d.low}, {a, must(testbedvalues.NewNull(a.Type()))}}
			for j, node := range stages {
				partial := must(evaluator.PartialEvaluate(node, missing))
				residual, present := partial.Residual()
				if !present || !partial.Type().EqualsTo(source.Type()) {
					t.Fatalf("stage %d: missing input became a known value", j)
				}
				if _, err := evaluator.Evaluate(node, missing); !errors.Is(err, interpreter.ErrUnavailableInput) {
					t.Fatal("missing input lost error category", err)
				}
				known := must(evaluator.PartialEvaluate(node, knownNull))
				v, present := known.Known()
				if !present || !v.IsNull() || !v.Type().EqualsTo(source.Type()) {
					t.Fatalf("stage %d: present NULL became unavailable", j)
				}
				scalarMissing := partialScalarRow{{a, booleanLiteral(foundation.True, foundation.MayNull).Value()}}
				scalarPartial := must(evaluator.PartialEvaluate(node, scalarMissing))
				scalarResidual, present := scalarPartial.Residual()
				if !present || !scalarPartial.Type().EqualsTo(source.Type()) {
					t.Fatal("unavailable scalar became known")
				}
				for _, sample := range d.samples {
					completed := contextualRow{{x, sample}, {a, booleanLiteral(foundation.True, foundation.MayNull).Value()}}
					expected := booleanLiteral(truthOf(nativeEqual(sample, d.low)), source.Type().Nullability()).Value()
					if !must(evaluator.Evaluate(scalarResidual, completed)).EqualsTo(expected) {
						t.Fatal("scalar residual changed semantics")
					}
				}
				// Residual structures may differ. Completing the missing binding must give
				// the same truth and full declared Type for every remaining Boolean value.
				for _, av := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
					complete := contextualRow{{x, d.low}, {a, booleanLiteral(av, foundation.MayNull).Value()}}
					expected := booleanLiteral(av, source.Type().Nullability()).Value()
					if !must(evaluator.Evaluate(residual, complete)).EqualsTo(expected) || !must(evaluator.Evaluate(node, complete)).EqualsTo(expected) {
						t.Fatal("partial residual changed semantics")
					}
				}
			}
		})
	}
}
