package scoped_contextual_rewriter_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type scalarCase struct {
	name          string
	guard, target expressions.Expression
	expected      func(valueinterfaces.Value) bool
}

type scalarDomain struct {
	name              string
	typ               types.Type
	low, middle, high valueinterfaces.Value
	samples           []valueinterfaces.Value
}

// Samples enumerate complete rows around boundaries; they are not an exhaustive
// enumeration of the numeric or VARCHAR type's full declared domain.
func scalarDomains() []scalarDomain {
	b := must(types.NewBoolType(foundation.NonNull))
	i := must(types.NewIntType(types.Int64, foundation.NonNull))
	s := must(types.NewVarCharType(nil, foundation.NonNull))
	bv := func(v bool) valueinterfaces.Value { return must(testbedvalues.NewBool(b, v)) }
	iv := func(v int64) valueinterfaces.Value { return must(testbedvalues.NewInteger(i, v)) }
	sv := func(v string) valueinterfaces.Value { return must(testbedvalues.NewVarChar(s, v)) }
	domains := []scalarDomain{
		{"Bool", b, bv(false), bv(false), bv(true), []valueinterfaces.Value{bv(false), bv(true)}},
		{"Int64", i, iv(-1), iv(0), iv(1), nil},
		{"VARCHAR", s, sv("a"), sv("m"), sv("z"), nil},
	}
	for _, n := range []int64{math.MinInt64, -2, -1, 0, 1, 2, math.MaxInt64} {
		domains[1].samples = append(domains[1].samples, iv(n))
	}
	for _, text := range []string{"", "a", "a\x00", "l", "m", "m\x00", "z", "zz", "中"} {
		domains[2].samples = append(domains[2].samples, sv(text))
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		fv := func(v float64) valueinterfaces.Value { return must(testbedvalues.NewFloat(typ, v)) }
		before, after, tiny, max := math.Nextafter(1, 0), math.Nextafter(1, 2), math.SmallestNonzeroFloat64, math.MaxFloat64
		if width == types.Float32 {
			before, after, tiny, max = float64(math.Nextafter32(1, 0)), float64(math.Nextafter32(1, 2)), math.SmallestNonzeroFloat32, math.MaxFloat32
		}
		domain := scalarDomain{typ.String(), typ, fv(-1), fv(0), fv(1), nil}
		for _, n := range []float64{math.Inf(-1), -max, -1, -tiny, math.Copysign(0, -1), 0, tiny, before, 1, after, max, math.Inf(1), math.NaN()} {
			domain.samples = append(domain.samples, fv(n))
		}
		domains = append(domains, domain)
	}
	return domains
}

func scalarLiteral(v valueinterfaces.Value) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(v))
}

func scalarComparison(provider functioninterfaces.FunctionContext, op foundation.OperatorType, x *expressions.ReferenceExpression, v valueinterfaces.Value) *expressions.CallExpression {
	return must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(op, x.Type(), v.Type())), []expressions.Expression{x, scalarLiteral(v)}))
}

func scalarMembership(x *expressions.ReferenceExpression, candidates ...valueinterfaces.Value) *expressions.SpecialFormExpression {
	args := []expressions.Expression{x}
	for _, v := range candidates {
		args = append(args, scalarLiteral(v))
	}
	return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(x.Type().Nullability())), args))
}

// The oracle uses native comparisons, independent of ValueSet construction and
// Value.EqualsTo (whose NaN/signed-zero structural equality is deliberately different).
func nativeEqual(a, b valueinterfaces.Value) bool {
	switch x := a.(type) {
	case *testbedvalues.BoolValue:
		return x.Bool() == b.(*testbedvalues.BoolValue).Bool()
	case *testbedvalues.IntegerValue:
		return x.Integer() == b.(*testbedvalues.IntegerValue).Integer()
	case *testbedvalues.VarCharValue:
		return x.VarChar() == b.(*testbedvalues.VarCharValue).VarChar()
	case *testbedvalues.FloatValue:
		return x.Float() == b.(*testbedvalues.FloatValue).Float()
	default:
		panic("unexpected scalar fixture type")
	}
}

func nativeLess(a, b valueinterfaces.Value) bool {
	switch x := a.(type) {
	case *testbedvalues.IntegerValue:
		return x.Integer() < b.(*testbedvalues.IntegerValue).Integer()
	case *testbedvalues.VarCharValue:
		return x.VarChar() < b.(*testbedvalues.VarCharValue).VarChar()
	case *testbedvalues.FloatValue:
		return x.Float() < b.(*testbedvalues.FloatValue).Float()
	default:
		panic("unexpected ordered fixture type")
	}
}

func rewriteScalarFixture(t *testing.T, provider *contextualProvider, source expressions.Expression, want expressions.Expression) []expressions.Expression {
	t.Helper()
	before, calls := expressions.PrettyPrint(source), provider.evaluations
	contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, provider))
	cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, provider))
	intermediate := must(contextual.Rewrite(source, struct{}{}))
	result := must(cleanup.Rewrite(intermediate, struct{}{}))
	if want != nil && !result.EqualsTo(want) {
		t.Fatalf("unexpected result\n%s\nwant\n%s", expressions.PrettyPrint(result), expressions.PrettyPrint(want))
	}
	if provider.evaluations != calls || expressions.PrettyPrint(source) != before {
		t.Fatal("rewrite evaluated or mutated source")
	}
	if !must(contextual.Rewrite(source, struct{}{})).EqualsTo(intermediate) || provider.evaluations != calls {
		t.Fatal("reused rule leaked state")
	}
	stages := []expressions.Expression{source, intermediate, result}
	for j, node := range stages {
		if !node.Type().EqualsTo(source.Type()) {
			t.Fatal("declared Type changed")
		}
		if j > 0 && reconstruction.MeasureExpression(node).NodeCount > reconstruction.MeasureExpression(stages[j-1]).NodeCount {
			t.Fatal("rewrite grew tree")
		}
	}
	return stages
}

func assertScalarRows(t *testing.T, provider *contextualProvider, stages []expressions.Expression, rows []contextualRow, truths []foundation.TruthValue) {
	t.Helper()
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	for j, row := range rows {
		want := booleanLiteral(truths[j], stages[0].Type().Nullability()).Value()
		for k, node := range stages {
			got := must(evaluator.Evaluate(node, row))
			if !got.EqualsTo(want) {
				t.Fatalf("row %d stage %d: got %v, want truth %v under %s", j, k, got, truths[j], want.Type())
			}
		}
	}
}

func truthOf(v bool) foundation.TruthValue {
	if v {
		return foundation.True
	}
	return foundation.False
}

func TestScalarDomainsPublicComposition(t *testing.T) {
	for _, d := range scalarDomains() {
		t.Run(d.name, func(t *testing.T) {
			provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			x := reference("x", d.typ)
			eq := func(v valueinterfaces.Value) expressions.Expression {
				return scalarComparison(provider, foundation.Equal, x, v)
			}
			finite := scalarMembership(x, d.low, d.middle)
			cases := []scalarCase{
				{"finite membership", finite, logical(foundation.Not, eq(d.high)), func(v valueinterfaces.Value) bool { return nativeEqual(v, d.low) || nativeEqual(v, d.middle) }},
			}
			if _, boolean := d.typ.(*types.BoolType); !boolean {
				cmp := func(op foundation.OperatorType, v valueinterfaces.Value) expressions.Expression {
					return scalarComparison(provider, op, x, v)
				}
				cases = append(cases,
					scalarCase{"open upper", cmp(foundation.LessThan, d.middle), cmp(foundation.LessThan, d.high), func(v valueinterfaces.Value) bool { return nativeLess(v, d.middle) }},
					scalarCase{"closed upper", cmp(foundation.LessThanOrEqual, d.middle), cmp(foundation.LessThanOrEqual, d.high), func(v valueinterfaces.Value) bool { return nativeLess(v, d.middle) || nativeEqual(v, d.middle) }},
					scalarCase{"open lower", cmp(foundation.GreaterThan, d.middle), cmp(foundation.GreaterThanOrEqual, d.middle), func(v valueinterfaces.Value) bool { return nativeLess(d.middle, v) }},
					scalarCase{"closed lower", cmp(foundation.GreaterThanOrEqual, d.middle), logical(foundation.Not, cmp(foundation.LessThan, d.middle)), func(v valueinterfaces.Value) bool { return nativeLess(d.middle, v) || nativeEqual(v, d.middle) }},
				)
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					source := logical(foundation.And, tc.guard, tc.target)
					stages := rewriteScalarFixture(t, provider, source, tc.guard)
					if !stages[1].EqualsTo(logical(foundation.And, tc.guard, booleanLiteral(foundation.True, foundation.NonNull))) {
						t.Fatal("missing contextual proof before cleanup")
					}
					rows := make([]contextualRow, len(d.samples))
					truths := make([]foundation.TruthValue, len(rows))
					for j, v := range d.samples {
						rows[j] = contextualRow{{x, v}}
						truths[j] = truthOf(tc.expected(v))
					}
					assertScalarRows(t, provider, stages, rows, truths)
				})
			}
		})
	}
}

func TestScalarDomainsNullableGuardsAndTargets(t *testing.T) {
	for _, d := range scalarDomains() {
		t.Run(d.name, func(t *testing.T) {
			provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			a := reference("a", must(types.NewBoolType(foundation.MayNull)))
			for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				x := reference("x", must(d.typ.WithNullability(nullability)))
				guard := scalarMembership(x, d.low)
				impossible := scalarComparison(provider, foundation.Equal, x, d.high)
				source := logical(foundation.And, guard, logical(foundation.Not, logical(foundation.And, a, impossible)))
				var want expressions.Expression = source
				if nullability == foundation.NonNull {
					want = logical(foundation.And, guard, booleanLiteral(foundation.True, foundation.MayNull))
				}
				stages := rewriteScalarFixture(t, provider, source, want)
				samples := append([]valueinterfaces.Value(nil), d.samples...)
				if nullability == foundation.MayNull {
					samples = append(samples, must(testbedvalues.NewNull(x.Type())))
				}
				var rows []contextualRow
				var truths []foundation.TruthValue
				for _, v := range samples {
					for _, av := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
						rows = append(rows, contextualRow{{x, must(v.WithNullability(nullability))}, {a, booleanLiteral(av, foundation.MayNull).Value()}})
						expected := foundation.Null
						if !v.IsNull() {
							expected = truthOf(nativeEqual(v, d.low))
						}
						truths = append(truths, expected)
					}
				}
				assertScalarRows(t, provider, stages, rows, truths)
			}
		})
	}
}

func TestFloatUnorderedComparisonsRemainDistinct(t *testing.T) {
	for _, d := range scalarDomains()[3:] {
		t.Run(d.name, func(t *testing.T) {
			provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			x := reference("x", d.typ)
			cmp := func(op foundation.OperatorType) expressions.Expression {
				return scalarComparison(provider, op, x, d.middle)
			}
			// NaN != 0 is true, but both ordered comparisons are false. Replacing
			// their disjunction by true under this guard would therefore be unsound.
			source := logical(foundation.And, cmp(foundation.NotEqual), logical(foundation.Or, cmp(foundation.LessThan), cmp(foundation.GreaterThan)))
			stages := rewriteScalarFixture(t, provider, source, nil)
			rows := make([]contextualRow, len(d.samples))
			truths := make([]foundation.TruthValue, len(rows))
			for j, v := range d.samples {
				rows[j] = contextualRow{{x, v}}
				truths[j] = truthOf(nativeLess(v, d.middle) || nativeLess(d.middle, v))
			}
			assertScalarRows(t, provider, stages, rows, truths)
			// A NaN candidate never matches; either signed zero does match zero.
			nan := must(testbedvalues.NewFloat(d.typ.(*types.FloatType), math.NaN()))
			guard := scalarMembership(x, nan, d.middle)
			stages = rewriteScalarFixture(t, provider, logical(foundation.And, guard, cmp(foundation.Equal)), guard)
			for j, v := range d.samples {
				truths[j] = truthOf(nativeEqual(v, d.middle))
			}
			assertScalarRows(t, provider, stages, rows, truths)
		})
	}
}
