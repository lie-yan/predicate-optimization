package fact_extraction_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func logical(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, children))
}

func TestLogicalExtractionMatchesInterpreter(t *testing.T) {
	context := must(testbed.NewStandardContext())
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, context)
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	domains := [][]valueinterfaces.Value{
		{must(testbedvalues.NewBool(boolean, false)), must(testbedvalues.NewBool(boolean, true))},
		{must(testbedvalues.NewInteger(integer, 3)), must(testbedvalues.NewInteger(integer, 10)), must(testbedvalues.NewInteger(integer, 9)), must(testbedvalues.NewInteger(integer, 11)), must(testbedvalues.NewInteger(integer, math.MinInt64)), must(testbedvalues.NewInteger(integer, math.MaxInt64))},
		{must(testbedvalues.NewVarChar(text, "a")), must(testbedvalues.NewVarChar(text, "b")), must(testbedvalues.NewVarChar(text, "")), must(testbedvalues.NewVarChar(text, "a\x00")), must(testbedvalues.NewVarChar(text, "\xff"))},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		var domain []valueinterfaces.Value
		for _, v := range []float64{3, 10, 9, 11, math.NaN(), math.Inf(-1), math.Inf(1), math.Copysign(0, -1), 0} {
			domain = append(domain, must(testbedvalues.NewFloat(typ, v)))
		}
		domains = append(domains, domain)
	}
	for _, domain := range domains {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			typ := must(domain[0].Type().WithNullability(n))
			t.Run(typ.String(), func(t *testing.T) {
				ref := reference(typ, "t", "x")
				p := comparison(context, foundation.Equal, ref, literal(domain[0]))
				// Separately allocated equivalent references must be one analysis operand.
				q := comparison(context, foundation.Equal, reference(typ, "t", "x"), literal(domain[1]))
				yes, no := boolLiteral(foundation.MayNull, true), boolLiteral(foundation.NonNull, false)
				atoms := []expressions.Expression{p, q, membership(ref, literal(domain[0]), literal(domain[1])), logical(foundation.Not, p), logical(foundation.And, p, no), logical(foundation.Or, p, yes), yes, no}
				var predicates []expressions.Expression
				for i, a := range atoms {
					for j, b := range atoms {
						if i >= len(atoms)-2 && j >= len(atoms)-2 {
							continue
						}
						for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
							node := logical(op, a, b)
							predicates = append(predicates, node, logical(foundation.Not, node), logical(op, no, node, yes))
						}
					}
				}
				if typ.IsOrderable() {
					lt := comparison(context, foundation.LessThan, ref, literal(domain[1]))
					ne := comparison(context, foundation.NotEqual, ref, literal(domain[0]))
					predicates = append(predicates, logical(foundation.And, lt, ne), logical(foundation.Not, lt), logical(foundation.Or, lt, logical(foundation.Not, lt)), logical(foundation.And, lt, membership(ref, literal(domain[0]), literal(domain[1]))))
				}
				predicates = append(predicates, logical(foundation.Not, logical(foundation.Not, p)), logical(foundation.And, membership(ref, literal(domain[0])), membership(ref, literal(domain[1]))))
				var inputs []valueinterfaces.Value
				for _, v := range domain {
					inputs = append(inputs, must(v.WithNullability(n)))
				}
				if n == foundation.MayNull {
					inputs = append(inputs, must(testbedvalues.NewNull(typ)))
				}
				for _, predicate := range predicates {
					result := must(extractor.Extract(predicate))
					summary, present := result.Summary().Get()
					if !present || result.Status() != fact_extraction.Exact || result.Source() != predicate || result.Type() != predicate.Type() || !summary.Operand().EqualsTo(ref) {
						t.Fatal("logical summary/source/type mismatch")
					}
					if _, present := summary.NullTruth().Get(); present != (n == foundation.MayNull) {
						t.Fatal("null presence mismatch")
					}
					for _, input := range inputs {
						expected := must(evaluator.Evaluate(predicate, scalarEnvironment{input}))
						truth := foundation.False
						if input.IsNull() {
							truth, _ = summary.NullTruth().Get()
						} else if must(summary.TrueSet().Contains(input)) {
							truth = foundation.True
						}
						if expected.IsNull() {
							if truth != foundation.Null {
								t.Fatal("NULL mismatch")
							}
						} else if truth == foundation.Null || (truth == foundation.True) != expected.(*testbedvalues.BoolValue).Bool() {
							t.Fatalf("truth mismatch: %s input %s", predicate, input)
						}
					}
				}
			})
		}
	}
}

func TestLogicalExtractionRetainsUnsupportedGroups(t *testing.T) {
	context := must(testbed.NewStandardContext())
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ, "t", "x")
	one := literal(must(testbedvalues.NewInteger(typ, 1)))
	p := comparison(context, foundation.Equal, ref, one)
	q := comparison(context, foundation.Equal, reference(typ, "t", "y"), one)
	otherNullability := comparison(context, foundation.Equal, reference(must(typ.WithNullability(foundation.MayNull)), "t", "x"), one)
	unsupported := comparison(context, foundation.Equal, ref, ref)
	yes, no := boolLiteral(foundation.NonNull, true), boolLiteral(foundation.NonNull, false)
	null := literal(must(testbedvalues.NewNull(must(types.NewBoolType(foundation.MayNull)))))
	for _, predicate := range []expressions.Expression{
		logical(foundation.And, p, q), logical(foundation.Or, p, q), logical(foundation.And, p, otherNullability),
		logical(foundation.And, no, unsupported), logical(foundation.Or, yes, unsupported),
		logical(foundation.And, p, null), logical(foundation.Not, null), logical(foundation.Not, yes),
		logical(foundation.And, yes, no), logical(foundation.Or, no, logical(foundation.And, p, q)),
	} {
		result := must(extractor.Extract(predicate))
		if result.Status() != fact_extraction.Unsupported || result.Source() != predicate || result.Type() != predicate.Type() {
			t.Fatal("unsupported group lost")
		}
		if _, present := result.Summary().Get(); present {
			t.Fatal("partial group presented as exact")
		}
	}
	// Independent child analysis remains available after rejecting a mixed group.
	if must(extractor.Extract(p)).Status() != fact_extraction.Exact {
		t.Fatal("supported child lost")
	}
}

func TestLogicalExtractionDoesNotSuppressProviderErrors(t *testing.T) {
	base := must(testbed.NewStandardContext())
	context := &extractionContext{FunctionContext: base}
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ, "t", "x")
	one := literal(must(testbedvalues.NewInteger(typ, 1)))
	bad := comparison(context, foundation.Equal, ref, one)
	unsupported := comparison(base, foundation.Equal, ref, ref)
	context.metadataError = functions.ErrContractViolation
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, base)
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		for _, first := range []expressions.Expression{boolLiteral(foundation.NonNull, false), boolLiteral(foundation.NonNull, true), unsupported} {
			if _, err := extractor.Extract(logical(op, first, bad)); !errors.Is(err, functions.ErrContractViolation) {
				t.Fatalf("hidden provider error: %v", err)
			}
		}
	}
}
