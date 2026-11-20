package scoped_reasoning_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testexpressions "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type scalarEnvironment struct{ value valueinterfaces.Value }

func (e scalarEnvironment) Lookup(expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	return foundation.Some(e.value), nil
}

func literal(value valueinterfaces.Value) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(value))
}

func comparison(context functioninterfaces.FunctionContext, op foundation.OperatorType, left, right expressions.Expression) *expressions.CallExpression {
	return must(expressions.NewCallExpression(context, must(context.ComparisonFunction(op, left.Type(), right.Type())), []expressions.Expression{left, right}))
}

func logical(op foundation.LogicalOperator, args ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, args))
}

func summaryOf(context functioninterfaces.FunctionContext, source expressions.Expression) *facts.PredicateSummary {
	result := must(fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context).Extract(source))
	summary, present := result.Summary().Get()
	if !present {
		panic("expected exact summary")
	}
	return summary
}

func truthOf(value valueinterfaces.Value) foundation.TruthValue {
	if value.IsNull() {
		return foundation.Null
	}
	if value.(*testbedvalues.BoolValue).Bool() {
		return foundation.True
	}
	return foundation.False
}

// checkQueries compares proofs and restrictions against independent interpretation
// over a complete enumeration of the supplied input domain.
func checkQueries(t *testing.T, context functioninterfaces.FunctionContext, summary *facts.PredicateSummary, input *scoped_reasoning.InputDomain, premise *scoped_reasoning.Premise, inputs []valueinterfaces.Value) {
	t.Helper()
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, context)
	outcomes := make(map[foundation.TruthValue]bool)
	expectedPassing := make([]bool, len(inputs))
	passing := 0
	for i, input := range inputs {
		truth := truthOf(must(evaluator.Evaluate(summary.Source(), scalarEnvironment{input})))
		outcomes[truth] = true
		expectedPassing[i] = truth == foundation.True
		if expectedPassing[i] {
			passing++
		}
	}
	proof, present := must(scoped_reasoning.ProveTruth(summary, input)).Get()
	if present != (len(outcomes) <= 1) {
		t.Fatalf("uniform proof presence mismatch for %s on %s + null=%t", summary.Source(), input.PossibleValues(), input.NullPossible())
	}
	if present {
		if proof.Summary() != summary || proof.InputDomain() != input || proof.Scope() != input.Scope() {
			t.Fatal("proof lost contextual evidence")
		}
		if len(proof.Premises()) != len(input.Premises()) {
			t.Fatal("proof acquired condition dependency")
		}
		if _, ok := proof.RestrictedDomain().Get(); ok {
			t.Fatal("proof acquired restricted domain")
		}
		truth, known := proof.Truth().Get()
		if len(outcomes) == 0 {
			if proof.Kind() != scoped_reasoning.Unreachable || known {
				t.Fatal("empty input yielded vacuous truth")
			}
		} else if proof.Kind() != scoped_reasoning.KnownTruth || !known || !outcomes[truth] {
			t.Fatal("wrong known truth")
		}
	}
	result, present := must(scoped_reasoning.RestrictToTrue(summary, input, premise)).Get()
	if !present || result.Summary() != summary || result.InputDomain() != input || result.Scope() != input.Scope() {
		t.Fatal("restriction lost context")
	}
	if _, known := result.Truth().Get(); known {
		t.Fatal("restriction claimed uniform truth")
	}
	deps := result.Premises()
	if len(deps) != len(input.Premises())+1 || deps[len(deps)-1] != premise {
		t.Fatal("condition dependency lost")
	}
	deps[len(deps)-1] = nil
	if result.Premises()[len(deps)-1] != premise {
		t.Fatal("mutable conclusion dependencies")
	}
	restricted, present := result.RestrictedDomain().Get()
	if passing == 0 {
		if result.Kind() != scoped_reasoning.Unreachable || present {
			t.Fatal("empty restriction not unreachable")
		}
		return
	}
	if result.Kind() != scoped_reasoning.RestrictedDomain || !present {
		t.Fatal("missing restricted domain")
	}
	if restricted.Operand() != input.Operand() || restricted.Scope() != input.Scope() || len(restricted.Premises()) != len(result.Premises()) {
		t.Fatal("restriction changed operand or dependencies")
	}
	for i, value := range inputs {
		contains := restricted.NullPossible()
		if !value.IsNull() {
			contains = must(restricted.PossibleValues().Contains(value))
		}
		if contains != expectedPassing[i] {
			t.Fatalf("wrong restriction for %s at %s", summary.Source(), value)
		}
	}
	if !must(restricted.PossibleValues().IsSubsetOf(input.PossibleValues())) || (restricted.NullPossible() && !input.NullPossible()) {
		t.Fatal("restriction expanded input")
	}
}

func TestQueriesMatchInterpreterOnFiniteDomains(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	domains := [][]valueinterfaces.Value{
		{must(testbedvalues.NewBool(boolean, false)), must(testbedvalues.NewBool(boolean, true))},
		{must(testbedvalues.NewInteger(integer, -1)), must(testbedvalues.NewInteger(integer, 0)), must(testbedvalues.NewInteger(integer, 3)), must(testbedvalues.NewInteger(integer, 10))},
		{must(testbedvalues.NewVarChar(text, "")), must(testbedvalues.NewVarChar(text, "a")), must(testbedvalues.NewVarChar(text, "a\x00")), must(testbedvalues.NewVarChar(text, "\xff"))},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		var domain []valueinterfaces.Value
		for _, v := range []float64{math.Inf(-1), 0, 3, math.Inf(1), math.NaN()} {
			domain = append(domain, must(testbedvalues.NewFloat(typ, v)))
		}
		domains = append(domains, domain)
	}
	yes, no := literal(must(testbedvalues.NewBool(boolean, true))), literal(must(testbedvalues.NewBool(boolean, false)))
	for _, domain := range domains {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			typ := must(domain[0].Type().WithNullability(n))
			t.Run(typ.String(), func(t *testing.T) {
				ref := reference(typ)
				eq := comparison(context, foundation.Equal, ref, literal(domain[0]))
				in := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(n)), []expressions.Expression{ref, literal(domain[0]), literal(domain[1])}))
				predicates := []expressions.Expression{eq, in, logical(foundation.Not, in), logical(foundation.And, eq, no), logical(foundation.Or, eq, yes)}
				if typ.IsOrderable() {
					predicates = append(predicates, comparison(context, foundation.LessThan, ref, literal(domain[1])))
				}
				factsContext := scoped_reasoning.NewScopedFactContext()
				scope := factsContext.NewScope()
				for _, predicate := range predicates {
					summary := summaryOf(context, predicate)
					premise := must(factsContext.RegisterPremise(scope, predicate))
					for mask := 0; mask < 1<<len(domain); mask++ {
						var selected []valueinterfaces.Value
						for i, value := range domain {
							if mask&(1<<i) != 0 {
								selected = append(selected, value)
							}
						}
						possible := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, selected))
						for _, nullPossible := range []bool{false, true} {
							if nullPossible && n == foundation.NonNull {
								continue
							}
							// A separately allocated equivalent reference must be admitted.
							input := must(scoped_reasoning.NewInputDomain(scope, reference(typ), possible, nullPossible, nil))
							var inputs []valueinterfaces.Value
							for _, value := range selected {
								inputs = append(inputs, must(value.WithNullability(n)))
							}
							if nullPossible {
								inputs = append(inputs, must(testbedvalues.NewNull(typ)))
							}
							checkQueries(t, context, summary, input, premise, inputs)
						}
					}
				}
			})
		}
	}
}

func TestQueriesOnRangeGuardAndRetainedDependencies(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	typ := must(types.NewIntType(types.Int64, foundation.MayNull))
	nonNull := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ)
	number := func(n int64) expressions.Expression { return literal(must(testbedvalues.NewInteger(nonNull, n))) }
	guard := logical(foundation.And, comparison(context, foundation.GreaterThanOrEqual, ref, number(-128)), comparison(context, foundation.LessThan, ref, number(10)))
	guardSummary := summaryOf(context, guard)
	factsContext := scoped_reasoning.NewScopedFactContext()
	scope := factsContext.NewScope()
	guardPremise := must(factsContext.RegisterPremise(scope, guard))
	// The caller establishes the guard's true-input restriction while retaining it.
	input := must(scoped_reasoning.NewInputDomain(scope, ref, guardSummary.TrueSet(), false, []*scoped_reasoning.Premise{guardPremise}))
	var inputs []valueinterfaces.Value
	for i := int64(-128); i < 10; i++ {
		inputs = append(inputs, must(testbedvalues.NewInteger(typ, i)))
	}
	for _, predicate := range []expressions.Expression{
		comparison(context, foundation.LessThan, ref, number(20)),
		comparison(context, foundation.GreaterThanOrEqual, ref, number(10)),
		comparison(context, foundation.Equal, ref, number(3)),
		comparison(context, foundation.NotEqual, ref, number(3)),
	} {
		premise := must(factsContext.RegisterPremise(scope, predicate))
		checkQueries(t, context, summaryOf(context, predicate), input, premise, inputs)
	}
	// Adding the same occurrence again deduplicates it; it never discharges it.
	result, _ := must(scoped_reasoning.RestrictToTrue(guardSummary, input, guardPremise)).Get()
	if len(result.Premises()) != 1 || result.Premises()[0] != guardPremise {
		t.Fatal("retained guard lost or duplicated")
	}
	restricted, _ := result.RestrictedDomain().Get()
	if !must(restricted.PossibleValues().EqualsTo(input.PossibleValues())) || ref.Type().Nullability() != foundation.MayNull {
		t.Fatal("guard changed domain or globally refined Type")
	}
	// The same expression registered again is a distinct dependency.
	otherOccurrence := must(factsContext.RegisterPremise(scope, guard))
	other, _ := must(scoped_reasoning.RestrictToTrue(guardSummary, input, otherOccurrence)).Get()
	if len(other.Premises()) != 2 {
		t.Fatal("distinct guard occurrences collapsed")
	}
}

func TestQueriesRejectMismatchedEvidence(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	typ := must(types.NewIntType(types.Int64, foundation.MayNull))
	ref := reference(typ)
	one := literal(must(testbedvalues.NewInteger(must(types.NewIntType(types.Int64, foundation.NonNull)), 1)))
	predicate := comparison(context, foundation.Equal, ref, one)
	summary := summaryOf(context, predicate)
	factsContext := scoped_reasoning.NewScopedFactContext()
	scope := factsContext.NewScope()
	premise := must(factsContext.RegisterPremise(scope, predicate))
	input := must(scoped_reasoning.NewInputDomain(scope, ref, summary.TrueSet(), false, nil))
	otherColumn := must(expressions.NewReferenceExpression(must(testexpressions.NewColumnID("t", "y")), typ, "x"))
	for _, operand := range []*expressions.ReferenceExpression{otherColumn, reference(must(typ.WithNullability(foundation.NonNull))), reference(must(types.NewIntType(types.Int32, foundation.MayNull)))} {
		possible := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, operand.Type(), nil))
		bad := must(scoped_reasoning.NewInputDomain(scope, operand, possible, false, nil))
		expected := value_set.ErrIncompatibleTypes
		if operand == otherColumn {
			expected = value_set.ErrInvalidInput
		}
		if result, err := scoped_reasoning.ProveTruth(summary, bad); !errors.Is(err, expected) {
			t.Fatalf("operand mismatch: %v, %v", result, err)
		}
		if _, err := scoped_reasoning.RestrictToTrue(summary, bad, premise); !errors.Is(err, expected) {
			t.Fatalf("restriction operand mismatch: %v", err)
		}
	}
	foreign := scoped_reasoning.NewScopedFactContext()
	for _, bad := range []*scoped_reasoning.Premise{
		must(factsContext.RegisterPremise(factsContext.NewScope(), predicate)),
		must(foreign.RegisterPremise(foreign.NewScope(), predicate)),
		must(factsContext.RegisterPremise(scope, logical(foundation.Not, predicate))),
	} {
		if _, err := scoped_reasoning.RestrictToTrue(summary, input, bad); !errors.Is(err, value_set.ErrInvalidInput) {
			t.Fatalf("bad condition accepted: %v", err)
		}
	}
	// Structural equality admits an independently built copy of the source.
	copy := comparison(context, foundation.Equal, reference(typ), one)
	if _, err := scoped_reasoning.RestrictToTrue(summary, input, must(factsContext.RegisterPremise(scope, copy))); err != nil {
		t.Fatal(err)
	}
}

// failingSet injects errors without changing the valid set universe.
type failingSet struct {
	value_set.ValueSet
	emptyError, subsetError, disjointError, intersectError error
	intersection                                           value_set.ValueSet
}

func (s *failingSet) IsEmpty() (bool, error) {
	if s.emptyError != nil {
		return false, s.emptyError
	}
	return s.ValueSet.IsEmpty()
}

func (s *failingSet) IsSubsetOf(other value_set.ValueSet) (bool, error) {
	if s.subsetError != nil {
		return false, s.subsetError
	}
	return s.ValueSet.IsSubsetOf(other)
}

func (s *failingSet) IsDisjointFrom(other value_set.ValueSet) (bool, error) {
	if s.disjointError != nil {
		return false, s.disjointError
	}
	return s.ValueSet.IsDisjointFrom(other)
}

func (s *failingSet) Intersect(other value_set.ValueSet) (value_set.ValueSet, error) {
	if s.intersectError != nil {
		return nil, s.intersectError
	}
	if s.intersection != nil {
		return s.intersection, nil
	}
	return s.ValueSet.Intersect(other)
}

func TestQueriesPropagateErrorsAndAssertPreconditions(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ)
	summary := summaryOf(context, comparison(context, foundation.Equal, ref, literal(must(testbedvalues.NewInteger(typ, 1)))))
	factsContext := scoped_reasoning.NewScopedFactContext()
	scope := factsContext.NewScope()
	premise := must(factsContext.RegisterPremise(scope, summary.Source()))
	possible := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{must(testbedvalues.NewInteger(typ, 1)), must(testbedvalues.NewInteger(typ, 2))}))
	input := must(scoped_reasoning.NewInputDomain(scope, ref, possible, false, nil))
	sentinel := errors.New("set failure")
	for _, cause := range []error{value_set.ErrUnsupported, sentinel} {
		for _, set := range []*failingSet{{ValueSet: possible, emptyError: cause}, {ValueSet: possible, subsetError: cause}, {ValueSet: possible, disjointError: cause}} {
			bad := must(scoped_reasoning.NewInputDomain(scope, ref, set, false, nil))
			result, err := scoped_reasoning.ProveTruth(summary, bad)
			if _, present := result.Get(); present || !errors.Is(err, cause) {
				t.Fatalf("proof suppressed error: %v", err)
			}
		}
		for _, set := range []*failingSet{{ValueSet: possible, intersectError: cause}, {ValueSet: possible, intersection: &failingSet{ValueSet: possible, emptyError: cause}}} {
			bad := must(scoped_reasoning.NewInputDomain(scope, ref, set, false, nil))
			result, err := scoped_reasoning.RestrictToTrue(summary, bad, premise)
			if _, present := result.Get(); present || !errors.Is(err, cause) {
				t.Fatalf("restriction suppressed error: %v", err)
			}
		}
	}
	for _, bad := range []*facts.PredicateSummary{nil, {}} {
		expectAssertion(t, func() { _, _ = scoped_reasoning.ProveTruth(bad, input) })
		expectAssertion(t, func() { _, _ = scoped_reasoning.RestrictToTrue(bad, input, premise) })
	}
	for _, bad := range []*scoped_reasoning.InputDomain{nil, {}} {
		expectAssertion(t, func() { _, _ = scoped_reasoning.ProveTruth(summary, bad) })
		expectAssertion(t, func() { _, _ = scoped_reasoning.RestrictToTrue(summary, bad, premise) })
	}
	for _, bad := range []*scoped_reasoning.Premise{nil, {}} {
		expectAssertion(t, func() { _, _ = scoped_reasoning.RestrictToTrue(summary, input, bad) })
	}
	var zero scoped_reasoning.ScopedConclusion
	for _, call := range []func(){func() { zero.Kind() }, func() { zero.Summary() }, func() { zero.InputDomain() }, func() { zero.Scope() }, func() { zero.Truth() }, func() { zero.RestrictedDomain() }, func() { zero.Premises() }} {
		expectAssertion(t, call)
	}
}
