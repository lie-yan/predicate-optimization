package engine_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestScalarDomainsConditionalProofOwnership(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	pairs := [][2]valueinterfaces.Value{
		{must(testbedvalues.NewBool(boolean, false)), must(testbedvalues.NewBool(boolean, true))},
		{must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))},
		{must(testbedvalues.NewVarChar(text, "a")), must(testbedvalues.NewVarChar(text, "b"))},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		pairs = append(pairs, [2]valueinterfaces.Value{must(testbedvalues.NewFloat(typ, 0)), must(testbedvalues.NewFloat(typ, 1))})
	}
	for _, pair := range pairs {
		t.Run(pair[0].Type().String(), func(t *testing.T) {
			// This fixture's provider panics on Evaluate, so proof construction cannot
			// accidentally use execution. Public fixtures separately verify native truths.
			provider := membershipEligibilityProvider()
			rule := engine.New(testbedvalues.ValueFactory{}, provider)
			x := guardReference("x", pair[0].Type())
			literal := func(v valueinterfaces.Value) *expressions.LiteralExpression {
				return must(expressions.NewLiteralExpression(v))
			}
			target := must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(foundation.Equal, x.Type(), pair[1].Type())), []expressions.Expression{x, literal(pair[1])}))
			var previous *engine.RewriteAttempt
			for j, v := range pair {
				guard := membershipExpression(x, literal(v))
				source := guardLogical(foundation.And, guard, target)
				attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
				result, present := must(attempt.ProveLiteral(target, engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1}})).Get()
				if !present || len(result.Steps()) != 1 {
					t.Fatal("missing conditional proof")
				}
				expected := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(target.Type().(*types.BoolType), j == 1))))
				if !result.Expression().EqualsTo(expected) || result.Expression().Type() != target.Type() {
					t.Fatal("wrong truth or lost exact Type instance")
				}
				conclusion, ok := result.Steps()[0].Conclusion.Get()
				if !ok || conclusion.InputDomain() != attempt.Input() || len(conclusion.Premises()) != 1 || !conclusion.Premises()[0].EqualsTo(attempt.Premise()) || conclusion.Summary().Source() != target {
					t.Fatal("proof lost owner association")
				}
				if previous != nil && (previous.Scope().EqualsTo(attempt.Scope()) || previous.Premise().EqualsTo(attempt.Premise())) {
					t.Fatal("independent attempts shared proof identity")
				}
				previous = attempt
			}
		})
	}
}
