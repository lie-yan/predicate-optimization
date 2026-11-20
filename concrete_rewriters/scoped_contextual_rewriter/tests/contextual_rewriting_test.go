package scoped_contextual_rewriter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type contextualProvider struct {
	functioninterfaces.FunctionContext
	evaluations int
	metadata    func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error)
}

func (p *contextualProvider) Evaluate(handle functioninterfaces.FunctionHandle, signature *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	p.evaluations++
	return p.FunctionContext.Evaluate(handle, signature, args)
}

func (p *contextualProvider) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if p.metadata != nil {
		return p.metadata(handle)
	}
	return p.FunctionContext.Metadata(handle)
}

func reference(name string, typ types.Type) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(ids.NewColumnID("public", name)), typ, name))
}

func logical(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, children))
}

func membership(target expressions.Expression, numbers ...int64) *expressions.SpecialFormExpression {
	arguments := []expressions.Expression{target}
	for _, n := range numbers {
		arguments = append(arguments, integer(n, foundation.NonNull))
	}
	boolean := must(types.NewBoolType(target.Type().Nullability()))
	return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, arguments))
}

func comparison(provider functioninterfaces.FunctionContext, op foundation.OperatorType, ref *expressions.ReferenceExpression, n int64) *expressions.CallExpression {
	literal := integer(n, foundation.NonNull)
	handle := must(provider.ComparisonFunction(op, ref.Type(), literal.Type()))
	return must(expressions.NewCallExpression(provider, handle, []expressions.Expression{ref, literal}))
}

func booleanLiteral(truth foundation.TruthValue, nullable foundation.Nullability) *expressions.LiteralExpression {
	typ := must(types.NewBoolType(nullable))
	if truth == foundation.Null {
		return must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(typ))))
	}
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(typ, truth == foundation.True))))
}

type contextualBinding struct {
	column *expressions.ReferenceExpression
	value  valueinterfaces.Value
}

type contextualRow []contextualBinding

func (row contextualRow) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for _, binding := range row {
		if id.EqualsTo(binding.column.ID()) {
			return foundation.Some(binding.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, errors.New("unbound public fixture column")
}

func TestPublicContextualRewritingCoreCases(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	rule := scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, provider)
	tree := rewriter.NewExpressionTreeRewriter[struct{}](rule)
	x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	y := reference("y", x.Type())
	nx := reference("nx", must(types.NewIntType(types.Int64, foundation.MayNull)))
	a := reference("a", must(types.NewBoolType(foundation.MayNull)))
	b := reference("b", a.Type())
	truth := booleanLiteral(foundation.True, foundation.NonNull)
	falsity := booleanLiteral(foundation.False, foundation.NonNull)
	guard := membership(x, 1, 2)
	impossible := logical(foundation.Not, comparison(provider, foundation.Equal, x, 3))
	broad := membership(x, 1, 2, 3)
	one, two := comparison(provider, foundation.Equal, x, 1), comparison(provider, foundation.Equal, x, 2)
	orGuard := logical(foundation.Or, one, two)
	rangeGuard := comparison(provider, foundation.LessThan, x, 10)
	rangeTarget := logical(foundation.Not, comparison(provider, foundation.GreaterThanOrEqual, x, 20))
	zeroBound, fiveBound, tenBound := comparison(provider, foundation.GreaterThan, x, 0), comparison(provider, foundation.GreaterThan, x, 5), comparison(provider, foundation.GreaterThan, x, 10)
	independent := logical(foundation.And, one, comparison(provider, foundation.Equal, y, 2))
	universe := logical(foundation.Or, one, logical(foundation.Not, one))
	universalSource := logical(foundation.And, universe, logical(foundation.Or, a, one))
	emptySource := logical(foundation.And, logical(foundation.Not, universe), logical(foundation.Or, a, one))
	nullableSource := logical(foundation.And, membership(nx, 1), logical(foundation.Not, logical(foundation.And, a, membership(nx, 1, 2))))

	for _, tc := range []struct {
		name         string
		source, want expressions.Expression
		unchanged    bool
	}{
		{"SC01 finite", logical(foundation.And, guard, impossible), logical(foundation.And, guard, truth), false},
		{"SC02 nullable survivor", logical(foundation.And, guard, logical(foundation.Not, logical(foundation.And, a, broad))), logical(foundation.And, guard, logical(foundation.Not, a)), false},
		{"SC03 range", logical(foundation.And, rangeGuard, rangeTarget), logical(foundation.And, rangeGuard, truth), false},
		{"SC04 left nested guard", logical(foundation.And, logical(foundation.And, guard, a), impossible), logical(foundation.And, logical(foundation.And, guard, a), truth), false},
		{"SC04 right nested guard", logical(foundation.And, a, logical(foundation.And, guard, impossible)), logical(foundation.And, a, logical(foundation.And, guard, truth)), false},
		{"SC05 whole OR guard", logical(foundation.And, orGuard, impossible), logical(foundation.And, orGuard, truth), false},
		{"SC06 shared guard occurrence", logical(foundation.And, one, one), logical(foundation.And, one, truth), false},
		{"SC07 independent shared targets", logical(foundation.Or, logical(foundation.And, one, one), logical(foundation.And, two, one), one), logical(foundation.Or, logical(foundation.And, one, truth), logical(foundation.And, two, falsity), one), false},
		{"SC08 different operands", independent, independent, true},
		{"SC09 unsupported outer owner", logical(foundation.And, a, logical(foundation.Or, logical(foundation.And, guard, impossible), b)), logical(foundation.And, a, logical(foundation.Or, logical(foundation.And, guard, truth), b)), false},
		{"SC09 unhelpful outer facts retire before nested owner", logical(foundation.And, guard, logical(foundation.Or, logical(foundation.And, one, one), a)), logical(foundation.And, guard, logical(foundation.Or, logical(foundation.And, one, truth), a)), false},
		{"SC10 later useful guard", logical(foundation.And, zeroBound, fiveBound, tenBound), logical(foundation.And, truth, fiveBound, tenBound), false},
		{"SC11 universal guard", universalSource, universalSource, true},
		{"SC11 unreachable guard", emptySource, emptySource, true},
		{"SC12 nullable guard", nullableSource, nullableSource, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := expressions.PrettyPrint(tc.source)
			calls := provider.evaluations
			got := must(tree.Rewrite(tc.source, struct{}{}))
			if provider.evaluations != calls {
				t.Fatal("rewriting evaluated expressions")
			}
			if !got.EqualsTo(tc.want) || !got.Type().EqualsTo(tc.source.Type()) {
				t.Fatalf("unexpected public result\n%s\nwant\n%s", expressions.PrettyPrint(got), expressions.PrettyPrint(tc.want))
			}
			if tc.unchanged && got != tc.source {
				t.Fatal("unchanged tree lost identity")
			}
			if tc.name == "SC07 independent shared targets" && got.Children()[2] != one {
				t.Fatal("shared target outside both owners was replaced")
			}
			if expressions.PrettyPrint(tc.source) != before || reconstruction.MeasureExpression(got).NodeCount > reconstruction.MeasureExpression(tc.source).NodeCount {
				t.Fatal("source changed or result grew")
			}
			// Reusing the same rule on the original input creates independent attempts.
			again := must(tree.Rewrite(tc.source, struct{}{}))
			if !again.EqualsTo(got) || provider.evaluations != calls {
				t.Fatal("rule retained attempt state or evaluated")
			}

			oracle := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
			for _, n := range []int64{-1, 0, 1, 2, 3, 5, 6, 10, 11, 20} {
				for _, av := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
					for _, bv := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
						row := contextualRow{
							{x, integer(n, foundation.NonNull).Value()},
							{y, integer(2, foundation.NonNull).Value()},
							{nx, integer(n, foundation.MayNull).Value()},
							{a, booleanLiteral(av, foundation.MayNull).Value()},
							{b, booleanLiteral(bv, foundation.MayNull).Value()},
						}
						if bv == foundation.Null {
							row[2].value = must(testbedvalues.NewNull(nx.Type()))
						}
						original, rewritten := must(oracle.Evaluate(tc.source, row)), must(oracle.Evaluate(got, row))
						expected := must(oracle.Evaluate(tc.want, row))
						if !original.EqualsTo(rewritten) || !rewritten.EqualsTo(expected) {
							t.Fatalf("evaluation differs at x=%d A=%d B=%d", n, av, bv)
						}
						if tc.name == "SC12 nullable guard" && bv == foundation.Null && av == foundation.True && !rewritten.IsNull() {
							t.Fatal("nullable counterexample lost NULL")
						}
					}
				}
			}
		})
	}
}

func TestPublicContextualStopsAtFirstAcceptedOwner(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membership(x, 1, 2)
	one := comparison(provider, foundation.Equal, x, 1)
	inner := logical(foundation.And, one, one)
	source := logical(foundation.And, guard, logical(foundation.Not, comparison(provider, foundation.Equal, x, 3)), inner)
	tree := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, provider))
	got := must(tree.Rewrite(source, struct{}{}))
	// The outer guard proves its middle sibling but cannot determine x == 1.
	// A successful owner finishes the occurrence; nested ownership waits for a later pass.
	if got.Children()[0] != guard || got.Children()[2] != inner || !got.Children()[1].EqualsTo(booleanLiteral(foundation.True, foundation.NonNull)) {
		t.Fatal("accepted owner was re-traversed or its guard changed")
	}
	if provider.evaluations != 0 {
		t.Fatal("rewriting evaluated")
	}
}

func TestPublicContextualErrorsReturnNoPartialTree(t *testing.T) {
	for _, phase := range []string{"selection", "attempt", "default descent"} {
		t.Run(phase, func(t *testing.T) {
			provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
			guard := membership(x, 1, 2)
			first := logical(foundation.Not, comparison(provider, foundation.Equal, x, 3))
			literal := integer(10, foundation.NonNull)
			if phase == "attempt" {
				literal = integer(10, foundation.MayNull)
			}
			handle := must(provider.ComparisonFunction(foundation.GreaterThan, x.Type(), literal.Type()))
			later := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{x, literal}))
			var source expressions.Expression = logical(foundation.And, guard, first, later)
			if phase == "default descent" {
				source = logical(foundation.Or, logical(foundation.And, guard, first), logical(foundation.And, guard, later))
			}
			before := expressions.PrettyPrint(source)
			sentinel := errors.New("provider inspection failed")
			provider.metadata = func(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
				if h.EqualsTo(handle) {
					return nil, sentinel
				}
				return provider.FunctionContext.Metadata(h)
			}
			tree := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, provider))
			got, err := tree.Rewrite(source, struct{}{})
			if got != nil || !errors.Is(err, sentinel) || provider.evaluations != 0 || expressions.PrettyPrint(source) != before {
				t.Fatal("failure was hidden or returned a partial/mutated tree", err)
			}
			provider.metadata = nil
			if _, err := tree.Rewrite(source, struct{}{}); err != nil {
				t.Fatal("failed attempt state survived into retry", err)
			}
		})
	}
}

func TestPublicContextualRequiresConstructedRule(t *testing.T) {
	for _, rule := range []*scoped_contextual_rewriter.ScopedContextualRewriter{nil, {}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("unconstructed rule did not assert")
				}
			}()
			truth := booleanLiteral(foundation.True, foundation.NonNull)
			rule.RewriteLogical(logical(foundation.And, truth, truth), struct{}{}, nil)
		}()
	}
}
