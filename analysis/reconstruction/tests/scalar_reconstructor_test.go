package reconstruction_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testexpressions "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func checkEquivalent(t *testing.T, target functioninterfaces.FunctionContext, source, candidate expressions.Expression, inputs []valueinterfaces.Value) {
	t.Helper()
	if !candidate.Type().EqualsTo(source.Type()) {
		t.Fatal("result Type changed")
	}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, target)
	for _, input := range inputs {
		want := must(evaluator.Evaluate(source, scalarEnvironment{input}))
		got := must(evaluator.Evaluate(candidate, scalarEnvironment{input}))
		if !want.EqualsTo(got) {
			t.Fatalf("%s -> %s differs at %s: %s != %s", source, candidate, input, want, got)
		}
	}
}

func TestDiscreteReconstructionMatchesInterpreter(t *testing.T) {
	sourceContext, targetContext := must(testfunctions.NewStandardContext()), must(testfunctions.NewStandardContext())
	reconstructor := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, targetContext)
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	domains := [][]valueinterfaces.Value{
		{must(testbedvalues.NewBool(boolean, false)), must(testbedvalues.NewBool(boolean, true))},
		{must(testbedvalues.NewInteger(integer, 3)), must(testbedvalues.NewInteger(integer, -1)), must(testbedvalues.NewInteger(integer, math.MinInt64)), must(testbedvalues.NewInteger(integer, math.MaxInt64))},
		{must(testbedvalues.NewVarChar(text, "z")), must(testbedvalues.NewVarChar(text, "a")), must(testbedvalues.NewVarChar(text, "")), must(testbedvalues.NewVarChar(text, "a\x00")), must(testbedvalues.NewVarChar(text, "\xff"))},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		var domain []valueinterfaces.Value
		for _, n := range []float64{3, -1, math.Inf(-1), math.Inf(1), math.Copysign(0, -1), 0, math.NaN()} {
			domain = append(domain, must(testbedvalues.NewFloat(typ, n)))
		}
		domains = append(domains, domain)
	}
	for _, domain := range domains {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			typ := must(domain[0].Type().WithNullability(n))
			t.Run(typ.String(), func(t *testing.T) {
				ref := reference(typ)
				var inputs []valueinterfaces.Value
				for _, value := range domain {
					inputs = append(inputs, must(value.WithNullability(n)))
				}
				if n == foundation.MayNull {
					inputs = append(inputs, must(testbedvalues.NewNull(typ)))
				}
				lists := [][]valueinterfaces.Value{{domain[0], domain[0]}}
				if typ.IsOrderable() {
					lists = append(lists, []valueinterfaces.Value{domain[0], domain[1], domain[0]})
				}
				if _, floating := typ.(*types.FloatType); floating {
					// NaN list entries never match; signed zeros match each other.
					lists = append(lists, []valueinterfaces.Value{domain[4], domain[5], domain[6]}, []valueinterfaces.Value{domain[2], domain[3]})
				}
				for _, list := range lists {
					for _, exclude := range []bool{false, true} {
						var source expressions.Expression = membership(ref, list...)
						if exclude {
							source = logical(foundation.Not, source)
						}
						summary := summaryOf(sourceContext, source)
						candidate, present := must(reconstructor.Reconstruct(summary)).Get()
						if !present {
							t.Fatal("missing discrete candidate")
						}
						checkEquivalent(t, targetContext, source, candidate, inputs)
						// Re-extraction verifies set/null meaning, independently of candidate syntax.
						roundTrip := summaryOf(targetContext, candidate)
						if !must(summary.TrueSet().EqualsTo(roundTrip.TrueSet())) || summary.NullTruth() != roundTrip.NullTruth() {
							t.Fatal("round-trip summary changed")
						}
						entries := summary.TrueSet().(*finite.EquatableValueSet).Values()
						core := candidate
						if negated, ok := core.(*expressions.LogicalExpression); ok {
							if negated.Operator() != foundation.Not {
								t.Fatal("unexpected logical candidate")
							}
							core = negated.Children()[0]
						}
						if len(entries) == 1 {
							call, ok := core.(*expressions.CallExpression)
							if !ok || call.Context() != targetContext || call.Arguments()[0] != ref {
								t.Fatal("singleton did not use target comparison and shared operand")
							}
							expected := foundation.Equal
							if exclude {
								if _, boolean := typ.(*types.BoolType); !boolean {
									expected = foundation.NotEqual
								}
							}
							metadata := must(targetContext.Metadata(call.Handle()))
							if op, _ := metadata.Operator().Get(); op != expected {
								t.Fatal("wrong comparison operator")
							}
							if call.Arguments()[1].(*expressions.LiteralExpression).Value() != entries[0] {
								t.Fatal("immutable literal was not shared")
							}
						} else {
							in, ok := core.(*expressions.SpecialFormExpression)
							if !ok || in.Form() != foundation.SpecialFormIn || in.Arguments()[0] != ref || len(in.Arguments()) != len(entries)+1 {
								t.Fatal("wrong membership candidate")
							}
							for i, entry := range entries {
								if in.Arguments()[i+1].(*expressions.LiteralExpression).Value() != entry {
									t.Fatal("stored candidate order or sharing changed")
								}
							}
						}
					}
				}
			})
		}
	}
}

func TestConstantReconstructionAndNullableFallback(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	reconstructor := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	yes, no := literal(must(testbedvalues.NewBool(boolean, true))), literal(must(testbedvalues.NewBool(boolean, false)))
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		typ := must(integer.WithNullability(n))
		ref := reference(typ)
		atom := membership(ref, must(testbedvalues.NewInteger(integer, 3)))
		inputs := []valueinterfaces.Value{must(testbedvalues.NewInteger(integer, 3)), must(testbedvalues.NewInteger(integer, 9))}
		for i, v := range inputs {
			inputs[i] = must(v.WithNullability(n))
		}
		if n == foundation.MayNull {
			inputs = append(inputs, must(testbedvalues.NewNull(typ)))
		}
		for _, source := range []expressions.Expression{logical(foundation.And, atom, no), logical(foundation.Or, atom, yes)} {
			candidate, present := must(reconstructor.Reconstruct(summaryOf(context, source))).Get()
			if !present {
				t.Fatal("uniform constant missing")
			}
			if _, ok := candidate.(*expressions.LiteralExpression); !ok {
				t.Fatal("expected typed Boolean constant")
			}
			checkEquivalent(t, context, source, candidate, inputs)
		}
		for _, source := range []expressions.Expression{logical(foundation.And, atom, logical(foundation.Not, atom)), logical(foundation.Or, atom, logical(foundation.Not, atom))} {
			result := must(reconstructor.Reconstruct(summaryOf(context, source)))
			candidate, present := result.Get()
			if present != (n == foundation.NonNull) {
				t.Fatal("nullable empty/universal region lost null result")
			}
			if present {
				checkEquivalent(t, context, source, candidate, inputs)
			}
		}
	}
	// Non-null input can still have a conservatively nullable source result Type.
	ref := reference(integer)
	atom := membership(ref, must(testbedvalues.NewInteger(integer, 3)))
	nullableTrue := literal(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.MayNull)), true)))
	var source expressions.Expression = logical(foundation.And, atom, nullableTrue)
	if _, present := must(reconstructor.Reconstruct(summaryOf(context, source))).Get(); present {
		t.Fatal("candidate narrowed declared result nullability")
	}
	// A complete Boolean universe is recognized without constructing IN [].
	boolRef := reference(boolean)
	source = membership(boolRef, must(testbedvalues.NewBool(boolean, true)), must(testbedvalues.NewBool(boolean, false)))
	candidate, present := must(reconstructor.Reconstruct(summaryOf(context, source))).Get()
	if !present {
		t.Fatal("Boolean universe not reconstructed")
	}
	if value, ok := candidate.(*expressions.LiteralExpression); !ok || !value.Value().(*testbedvalues.BoolValue).Bool() {
		t.Fatal("Boolean universe did not become true")
	}
}

func TestReconstructionDeclinesUnrepresentableSummaries(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	reconstructor := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context)
	boolType := must(types.NewBoolType(foundation.MayNull))
	// Opaque sources stand for adapter-supplied exact predicates beyond extraction's
	// vocabulary; the reconstructor must not invent equality semantics for them.
	opaque := must(expressions.NewOpaqueExpression(must(testexpressions.NewOpaqueID("fixture", "source")), boolType, "source"))
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.MayNull))
		nan := must(testbedvalues.NewFloat(typ, math.NaN()))
		for _, included := range []bool{true, false} {
			var set *finite.EquatableValueSet
			if included {
				set = must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{nan}))
			} else {
				set = must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{nan}))
			}
			summary := must(facts.NewPredicateSummary(reference(typ), set, foundation.Some(foundation.Null), opaque))
			if _, present := must(reconstructor.Reconstruct(summary)).Get(); present {
				t.Fatal("NaN set identity became predicate equality")
			}
		}
	}
	typ := must(types.NewIntType(types.Int64, foundation.MayNull))
	set := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{must(testbedvalues.NewInteger(typ, 3))}))
	for _, nullTruth := range []foundation.TruthValue{foundation.False, foundation.True} {
		summary := must(facts.NewPredicateSummary(reference(typ), set, foundation.Some(nullTruth), opaque))
		if _, present := must(reconstructor.Reconstruct(summary)).Get(); present {
			t.Fatal("non-propagating null outcome lost")
		}
	}
}
