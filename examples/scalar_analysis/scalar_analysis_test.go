package scalar_analysis_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func Example_scalarStatusIntersection() {
	context := fixtures.Must(functions.NewStandardContext())
	text := fixtures.Must(types.NewVarCharType(nil, foundation.NonNull))
	status := fixtures.ScalarReference("status", text)
	newStatus, ready, done := fixtures.Must(testbedvalues.NewVarChar(text, "new")), fixtures.Must(testbedvalues.NewVarChar(text, "ready")), fixtures.Must(testbedvalues.NewVarChar(text, "done"))
	// status IN ["new", "ready"] AND status IN ["ready", "done"].
	source := fixtures.ScalarLogical(foundation.And, fixtures.ScalarMembership(status, newStatus, ready), fixtures.ScalarMembership(status, ready, done))
	// One entry check covers extraction and subsequent checked reconstruction.
	if err := utils.ValidateForOptimization(source); err != nil {
		panic(err)
	}
	summary := fixtures.ScalarSummary(context, source)
	fmt.Println("exact true set:", summary.TrueSet())
	candidate, present := fixtures.Must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context).Reconstruct(summary)).Get()
	if !present {
		panic("expected equality candidate")
	}
	call := candidate.(*expressions.CallExpression)
	op, _ := fixtures.Must(context.Metadata(call.Handle())).Operator().Get()
	fmt.Println("equality candidate:", op == foundation.Equal)
	fmt.Println("result Type preserved:", candidate.Type().EqualsTo(source.Type()))
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, context)
	for _, input := range []valueinterfaces.Value{newStatus, ready, done} {
		bindings := fixtures.Environment{{ID: status.ID(), Value: input}}
		original := fixtures.Must(evaluator.Evaluate(source, bindings))
		rewritten := fixtures.Must(evaluator.Evaluate(candidate, bindings))
		fmt.Printf("%s: %t, matches source: %t\n", input.(*testbedvalues.VarCharValue).VarChar(), rewritten.(*testbedvalues.BoolValue).Bool(), rewritten.EqualsTo(original))
	}
	// Output:
	// exact true set: {"ready"}
	// equality candidate: true
	// result Type preserved: true
	// new: false, matches source: true
	// ready: true, matches source: true
	// done: false, matches source: true
}

func Example_scalarNullableContradiction() {
	context := fixtures.Must(functions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.MayNull))
	literalType := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x := fixtures.ScalarReference("x", integer)
	atom := fixtures.ScalarMembership(x, fixtures.Must(testbedvalues.NewInteger(literalType, 3)))
	// x IN [3] AND NOT(x IN [3]) is false on non-null inputs, NULL at NULL.
	source := fixtures.ScalarLogical(foundation.And, atom, fixtures.ScalarLogical(foundation.Not, atom))
	summary := fixtures.ScalarSummary(context, source)
	nullTruth, present := summary.NullTruth().Get()
	fmt.Println("exact true set:", summary.TrueSet())
	fmt.Println("known NULL at null input:", present && nullTruth == foundation.Null)
	_, candidatePresent := fixtures.Must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context).Reconstruct(summary)).Get()
	fmt.Println("constant replacement available:", candidatePresent)
	fmt.Println("source retained:", summary.Source() == source)

	// No passing rows does not make this predicate uniformly False.
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, context)
	result := fixtures.Must(evaluator.Evaluate(source, fixtures.Environment{{ID: x.ID(), Value: fixtures.Must(testbedvalues.NewNull(integer))}}))
	fmt.Println("evaluation at NULL is NULL:", result.IsNull())
	scope := scoped_reasoning.NewScope()
	nullOnly := fixtures.Must(scoped_reasoning.NewInputDomain(scope, x, summary.TrueSet(), true, nil))
	proof, hasProof := fixtures.Must(scoped_reasoning.ProveTruth(summary, nullOnly)).Get()
	truth, hasTruth := proof.Truth().Get()
	fmt.Println("null-only proof is known NULL:", hasProof && proof.Kind() == scoped_reasoning.KnownTruth && hasTruth && truth == foundation.Null)
	// Output:
	// exact true set: {}
	// known NULL at null input: true
	// constant replacement available: false
	// source retained: true
	// evaluation at NULL is NULL: true
	// null-only proof is known NULL: true
}

func Example_scalarGlobalRange() {
	context := fixtures.Must(functions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x := fixtures.ScalarReference("x", integer)
	number := func(n int64) expressions.Expression {
		return fixtures.ScalarLiteral(fixtures.Must(testbedvalues.NewInteger(integer, n)))
	}
	source := fixtures.ScalarLogical(foundation.And, fixtures.ScalarComparison(context, foundation.LessThan, x, number(10)), fixtures.ScalarComparison(context, foundation.NotEqual, x, number(3)))
	summary := fixtures.ScalarSummary(context, source)
	fmt.Println("exact true set:", summary.TrueSet())
	candidate, present := fixtures.Must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context).Reconstruct(summary)).Get()
	if !present {
		panic("expected range candidate")
	}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, context)
	for _, n := range []int64{2, 3, 9, 10} {
		bindings := fixtures.Environment{{ID: x.ID(), Value: fixtures.Must(testbedvalues.NewInteger(integer, n))}}
		original := fixtures.Must(evaluator.Evaluate(source, bindings))
		rebuilt := fixtures.Must(evaluator.Evaluate(candidate, bindings))
		fmt.Printf("x=%d: %t, matches source: %t\n", n, rebuilt.(*testbedvalues.BoolValue).Bool(), rebuilt.EqualsTo(original))
	}
	// Equivalence alone does not establish that range expansion is preferable.
	before, after := reconstruction.MeasureExpression(source), reconstruction.MeasureExpression(candidate)
	fmt.Printf("source/candidate occurrences: %d/%d\n", before.NodeCount, after.NodeCount)
	fmt.Println("smaller source remains available:", before.NodeCount < after.NodeCount && summary.Source() == source)
	// Output:
	// exact true set: (-∞,2] ∪ [4,9]
	// x=2: true, matches source: true
	// x=3: false, matches source: true
	// x=9: true, matches source: true
	// x=10: false, matches source: true
	// source/candidate occurrences: 7/11
	// smaller source remains available: true
}

func Example_scalarRangeGuard() {
	context := fixtures.Must(functions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.MayNull))
	literalType := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x := fixtures.ScalarReference("x", integer)
	number := func(n int64) expressions.Expression {
		return fixtures.ScalarLiteral(fixtures.Must(testbedvalues.NewInteger(literalType, n)))
	}
	// Retain guard: x >= 0 AND x < 10. Analyze target: x < 20.
	guard := fixtures.ScalarLogical(foundation.And, fixtures.ScalarComparison(context, foundation.GreaterThanOrEqual, x, number(0)), fixtures.ScalarComparison(context, foundation.LessThan, x, number(10)))
	target := fixtures.ScalarComparison(context, foundation.LessThan, x, number(20))
	guardSummary, targetSummary := fixtures.ScalarSummary(context, guard), fixtures.ScalarSummary(context, target)

	scope := scoped_reasoning.NewScope()
	universe := fixtures.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, integer, nil))
	input := fixtures.Must(scoped_reasoning.NewInputDomain(scope, x, universe, true, nil))
	_, globallyKnown := fixtures.Must(scoped_reasoning.ProveTruth(targetSummary, input)).Get()
	fmt.Println("uniform before guard:", globallyKnown)

	// Registration records an occurrence; restriction establishes its true-input
	// domain conditionally, retaining that occurrence as a dependency.
	premise := fixtures.Must(scope.RegisterPremise(guard))
	restriction, hasRestriction := fixtures.Must(scoped_reasoning.RestrictToTrue(guardSummary, input, premise)).Get()
	if !hasRestriction {
		panic("expected restriction")
	}
	restricted, hasDomain := restriction.RestrictedDomain().Get()
	if !hasDomain {
		panic("expected reachable restricted domain")
	}
	fmt.Println("guarded inputs:", restricted.PossibleValues())
	fmt.Println("NULL still possible:", restricted.NullPossible())
	proof, hasProof := fixtures.Must(scoped_reasoning.ProveTruth(targetSummary, restricted)).Get()
	if !hasProof {
		panic("expected conditional proof")
	}
	truth, hasTruth := proof.Truth().Get()
	fmt.Println("target is true under guard:", proof.Kind() == scoped_reasoning.KnownTruth && hasTruth && truth == foundation.True)
	dependencies := proof.Premises()
	fmt.Println("guard dependency retained:", len(dependencies) == 1 && dependencies[0].EqualsTo(premise))
	fmt.Println("reference remains nullable:", x.Type().Nullability() == foundation.MayNull)
	// The conditional proof does not authorize deleting the retained guard or
	// publishing a whole-expression replacement without its NULL/safety analysis.
	// Output:
	// uniform before guard: false
	// guarded inputs: [0,9]
	// NULL still possible: false
	// target is true under guard: true
	// guard dependency retained: true
	// reference remains nullable: true
}

func Example_scalarUnsupportedInput() {
	context := fixtures.Must(functions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x, y := fixtures.ScalarReference("x", integer), fixtures.ScalarReference("y", integer)
	supported := fixtures.ScalarMembership(x, fixtures.Must(testbedvalues.NewInteger(integer, 3)))
	// A reference/reference comparison needs reasoning beyond this fragment.
	unsupported := fixtures.ScalarComparison(context, foundation.Equal, x, y)
	source := fixtures.ScalarLogical(foundation.And, supported, unsupported)
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	result := fixtures.Must(extractor.Extract(source))
	_, hasSummary := result.Summary().Get()
	reason, hasReason := result.Reason().Get()
	fmt.Println("unsupported:", result.Status() == fact_extraction.Unsupported)
	fmt.Println("exact parent summary available:", hasSummary)
	fmt.Println("source retained:", result.Source() == source)
	fmt.Println("diagnostic available:", hasReason && reason != "")
	// Supported children can still be analyzed independently; their facts are
	// not substituted for the unsupported whole predicate.
	child := fixtures.Must(extractor.Extract(supported))
	fmt.Println("supported child is exact:", child.Status() == fact_extraction.Exact)
	// Output:
	// unsupported: true
	// exact parent summary available: false
	// source retained: true
	// diagnostic available: true
	// supported child is exact: true
}
