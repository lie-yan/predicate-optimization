package scoped_reasoning_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func reference(typ types.Type) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(testbed.NewColumnID("t", "x")), typ, "x"))
}

func condition() expressions.Expression {
	return reference(must(types.NewBoolType(foundation.MayNull)))
}

func expectAssertion(t *testing.T, call func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected assertion")
		}
	}()
	call()
}

func TestScopedOccurrenceIdentity(t *testing.T) {
	context := scoped_reasoning.NewScopedFactContext()
	scope, other := context.NewScope(), context.NewScope()
	foreign := scoped_reasoning.NewScopedFactContext().NewScope()
	if !scope.EqualsTo(scope) || scope.EqualsTo(other) || scope.EqualsTo(foreign) {
		t.Fatal("scope identity mismatch")
	}
	expression := condition()
	first := must(context.RegisterPremise(scope, expression))
	second := must(context.RegisterPremise(scope, expression))
	third := must(context.RegisterPremise(other, expression))
	if first.EqualsTo(second) || first.EqualsTo(third) || !first.EqualsTo(first) {
		t.Fatal("shared expression collapsed occurrence identity")
	}
	if first.Expression() != expression || first.Scope() != scope {
		t.Fatal("immutable expression/scope not shared")
	}
	scopeCopy, premiseCopy := *scope, *first
	if !scope.EqualsTo(&scopeCopy) || !first.EqualsTo(&premiseCopy) {
		t.Fatal("token copying changed identity")
	}
	if _, err := context.RegisterPremise(foreign, expression); err == nil {
		t.Fatal("foreign context accepted")
	}
	if _, err := context.RegisterPremise(scope, reference(must(types.NewIntType(types.Int64, foundation.NonNull)))); err == nil {
		t.Fatal("non-Boolean premise accepted")
	}
}

func TestInputDomainOwnershipAndScope(t *testing.T) {
	context := scoped_reasoning.NewScopedFactContext()
	scope := context.NewScope()
	typ := must(types.NewIntType(types.Int64, foundation.MayNull))
	operand := reference(typ)
	possible := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{must(testbedvalues.NewInteger(typ, 1)), must(testbedvalues.NewInteger(typ, 3))}))
	expression := condition()
	first, second := must(context.RegisterPremise(scope, expression)), must(context.RegisterPremise(scope, expression))
	firstCopy := *first
	dependencies := []*scoped_reasoning.Premise{first, second, &firstCopy}
	domain := must(scoped_reasoning.NewInputDomain(scope, operand, possible, true, dependencies))
	dependencies[0] = nil
	stored := domain.Premises()
	if len(stored) != 2 || stored[0] != first || stored[1] != second {
		t.Fatal("identity deduplication or dependency order mismatch")
	}
	stored[1] = nil
	if domain.Premises()[1] != second {
		t.Fatal("dependency slice escaped")
	}
	if domain.Scope() != scope || domain.Operand() != operand || domain.PossibleValues() != possible || !domain.NullPossible() || operand.Type() != typ {
		t.Fatal("immutable inputs or nullability changed")
	}
	for _, bad := range []*scoped_reasoning.Premise{
		must(context.RegisterPremise(context.NewScope(), expression)),
		func() *scoped_reasoning.Premise {
			c := scoped_reasoning.NewScopedFactContext()
			return must(c.RegisterPremise(c.NewScope(), expression))
		}(),
	} {
		if _, err := scoped_reasoning.NewInputDomain(scope, operand, possible, false, []*scoped_reasoning.Premise{bad}); err == nil {
			t.Fatal("foreign scope/context premise accepted")
		}
	}
	// Scope survives even without premises; producer-established restrictions
	// are represented without claiming that construction proves them.
	independent := must(scoped_reasoning.NewInputDomain(scope, operand, possible, false, nil))
	if independent.Scope() != scope || len(independent.Premises()) != 0 || independent.NullPossible() {
		t.Fatal("premise-free restriction changed")
	}
}

func TestInputDomainUniversesAndNullability(t *testing.T) {
	scope := scoped_reasoning.NewScopedFactContext().NewScope()
	bound := uint64(4)
	for _, typ := range []types.Type{
		must(types.NewBoolType(foundation.MayNull)), must(types.NewIntType(types.Int8, foundation.MayNull)),
		must(types.NewIntType(types.Int64, foundation.MayNull)), must(types.NewFloatType(types.Float32, foundation.MayNull)),
		must(types.NewFloatType(types.Float64, foundation.MayNull)), must(types.NewVarCharType(&bound, foundation.MayNull)),
	} {
		operand := reference(typ)
		empty := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
		universe := must(empty.Complement())
		for _, possible := range []value_set.ValueSet{empty, universe} {
			for _, nullPossible := range []bool{false, true} {
				domain := must(scoped_reasoning.NewInputDomain(scope, operand, possible, nullPossible, nil))
				if domain.PossibleValues() != possible || domain.NullPossible() != nullPossible {
					t.Fatal("null-only/empty/universal representation changed")
				}
			}
		}
		nonNull := reference(must(typ.WithNullability(foundation.NonNull)))
		if _, err := scoped_reasoning.NewInputDomain(scope, nonNull, empty, true, nil); err == nil {
			t.Fatal("null admitted for non-nullable operand")
		}
		_ = must(scoped_reasoning.NewInputDomain(scope, nonNull, empty, false, nil))
	}
	integer := must(types.NewIntType(types.Int64, foundation.MayNull))
	upper := must(ranges.NewRangeEndpoint(must(testbedvalues.NewInteger(integer, 10)), false))
	interval, present := must(ranges.NewRange(testbedvalues.ValueFactory{}, integer, ranges.RangeEndpoint{}, upper)).Get()
	if !present {
		t.Fatal("expected interval")
	}
	possible := must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, integer, []*ranges.Range{interval}, false))
	domain := must(scoped_reasoning.NewInputDomain(scope, reference(integer), possible, true, nil))
	if domain.PossibleValues() != possible {
		t.Fatal("range set replaced")
	}
}

func TestInputDomainTypeCompatibility(t *testing.T) {
	scope := scoped_reasoning.NewScopedFactContext().NewScope()
	one, two := uint64(1), uint64(2)
	for _, pair := range [][2]types.Type{
		{must(types.NewIntType(types.Int8, foundation.MayNull)), must(types.NewIntType(types.Int64, foundation.NonNull))},
		{must(types.NewFloatType(types.Float32, foundation.MayNull)), must(types.NewFloatType(types.Float64, foundation.NonNull))},
		{must(types.NewVarCharType(&one, foundation.MayNull)), must(types.NewVarCharType(&two, foundation.NonNull))},
	} {
		possible := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, pair[1], nil))
		if _, err := scoped_reasoning.NewInputDomain(scope, reference(pair[0]), possible, false, nil); !errors.Is(err, value_set.ErrIncompatibleTypes) {
			t.Fatalf("domain mismatch: %v", err)
		}
	}
}

func TestScopedRepresentationsPreconditions(t *testing.T) {
	context := scoped_reasoning.NewScopedFactContext()
	scope := context.NewScope()
	operand := reference(must(types.NewBoolType(foundation.NonNull)))
	possible := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, operand.Type(), nil))
	for _, bad := range []*scoped_reasoning.ScopedFactContext{nil, {}} {
		expectAssertion(t, func() { bad.NewScope() })
		expectAssertion(t, func() { _, _ = bad.RegisterPremise(scope, condition()) })
	}
	for _, bad := range []*scoped_reasoning.Scope{nil, {}} {
		expectAssertion(t, func() { scope.EqualsTo(bad) })
		expectAssertion(t, func() { _, _ = context.RegisterPremise(bad, condition()) })
		expectAssertion(t, func() { _, _ = scoped_reasoning.NewInputDomain(bad, operand, possible, false, nil) })
	}
	for _, bad := range []*scoped_reasoning.Premise{nil, {}} {
		expectAssertion(t, func() { bad.Scope() })
		expectAssertion(t, func() { bad.Expression() })
		expectAssertion(t, func() {
			_, _ = scoped_reasoning.NewInputDomain(scope, operand, possible, false, []*scoped_reasoning.Premise{bad})
		})
	}
	expectAssertion(t, func() { _, _ = context.RegisterPremise(scope, nil) })
	expectAssertion(t, func() { _, _ = context.RegisterPremise(scope, (*expressions.ReferenceExpression)(nil)) })
	expectAssertion(t, func() { _, _ = scoped_reasoning.NewInputDomain(scope, nil, possible, false, nil) })
	expectAssertion(t, func() { _, _ = scoped_reasoning.NewInputDomain(scope, operand, nil, false, nil) })
	expectAssertion(t, func() {
		_, _ = scoped_reasoning.NewInputDomain(scope, operand, (*finite.EquatableValueSet)(nil), false, nil)
	})
	for _, bad := range []*scoped_reasoning.InputDomain{nil, {}} {
		for _, call := range []func(){func() { bad.Scope() }, func() { bad.Operand() }, func() { bad.PossibleValues() }, func() { bad.NullPossible() }, func() { bad.Premises() }} {
			expectAssertion(t, call)
		}
	}
}
