package facts_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/facts"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func boolLiteral(n foundation.Nullability, value bool) expressions.Expression {
	typ := must(types.NewBoolType(n))
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(typ, value))))
}

func TestPredicateSummarySharesInputsAndPreservesType(t *testing.T) {
	limit := uint64(8)
	for _, typ := range []types.Type{
		must(types.NewBoolType(foundation.NonNull)),
		must(types.NewIntType(types.Int8, foundation.NonNull)),
		must(types.NewIntType(types.Int64, foundation.NonNull)),
		must(types.NewFloatType(types.Float32, foundation.NonNull)),
		must(types.NewFloatType(types.Float64, foundation.NonNull)),
		must(types.NewVarCharType(&limit, foundation.NonNull)),
	} {
		t.Run(typ.String(), func(t *testing.T) {
			operand := reference(typ, "t", "x")
			set := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
			// A conservatively nullable result Type is retained even though this
			// summary describes a false result on every admitted non-null input.
			source := boolLiteral(foundation.MayNull, false)
			summary := must(facts.NewPredicateSummary(operand, set, foundation.Optional[foundation.TruthValue]{}, source))
			if summary.Operand() != operand || summary.TrueSet() != set || summary.Source() != source || summary.Type() != source.Type() {
				t.Fatal("immutable payload or original result Type lost")
			}
			if _, present := summary.NullTruth().Get(); present {
				t.Fatal("non-nullable operand acquired a null-input case")
			}
		})
	}
	// Summary depends on ValueSet, not on finite storage alone.
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ranges := must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, nil, false))
	summary := must(facts.NewPredicateSummary(reference(typ, "t", "x"), ranges, foundation.Optional[foundation.TruthValue]{}, boolLiteral(foundation.NonNull, false)))
	if summary.TrueSet() != ranges {
		t.Fatal("range representation was replaced")
	}
}

func TestPredicateSummaryNullOutcomes(t *testing.T) {
	typ := must(types.NewBoolType(foundation.MayNull))
	ref := reference(typ, "t", "x")
	operand := ref
	empty := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	universal := must(empty.Complement())
	yes := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{must(testbedvalues.NewBool(typ, true))}))
	for _, tc := range []struct {
		truth  foundation.TruthValue
		set    value_set.ValueSet
		source expressions.Expression
	}{
		{foundation.False, empty, boolLiteral(foundation.NonNull, false)},
		{foundation.True, universal, boolLiteral(foundation.NonNull, true)},
		{foundation.Null, yes, ref},
	} {
		summary := must(facts.NewPredicateSummary(operand, tc.set, foundation.Some(tc.truth), tc.source))
		got, present := summary.NullTruth().Get()
		if !present || got != tc.truth || summary.Type() != tc.source.Type() {
			t.Fatal("null outcome or original result Type lost")
		}
	}
}

func TestPredicateSummaryRejectsInconsistentInputs(t *testing.T) {
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := must(typ.WithNullability(foundation.MayNull))
	operand := reference(typ, "t", "x")
	nullableOperand := reference(nullable, "t", "x")
	empty := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	absent := foundation.Optional[foundation.TruthValue]{}
	yes := boolLiteral(foundation.NonNull, true)
	maybe := boolLiteral(foundation.MayNull, true)
	for _, tc := range []struct {
		operand *expressions.ReferenceExpression
		truth   foundation.Optional[foundation.TruthValue]
		source  expressions.Expression
	}{
		{operand, foundation.Some(foundation.False), yes},
		{nullableOperand, absent, maybe},
		{nullableOperand, foundation.Some(foundation.TruthValue(255)), maybe},
		{nullableOperand, foundation.Some(foundation.Null), yes},
		{operand, absent, must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(typ, 1))))},
	} {
		if result, err := facts.NewPredicateSummary(tc.operand, empty, tc.truth, tc.source); err == nil || result != nil {
			t.Fatal("inconsistent summary accepted")
		}
	}
	for _, otherType := range []types.Type{must(types.NewIntType(types.Int8, foundation.NonNull)), must(types.NewFloatType(types.Float64, foundation.NonNull))} {
		other := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, otherType, nil))
		if summary, err := facts.NewPredicateSummary(operand, other, absent, yes); summary != nil || !errors.Is(err, value_set.ErrIncompatibleTypes) {
			t.Fatal("domain mismatch accepted", err)
		}
	}
	one, two := uint64(1), uint64(2)
	text := must(types.NewVarCharType(&one, foundation.MayNull))
	otherText := must(types.NewVarCharType(&two, foundation.NonNull))
	if _, err := facts.NewPredicateSummary(reference(text, "t", "s"), must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, otherText, nil)), foundation.Some(foundation.Null), maybe); !errors.Is(err, value_set.ErrIncompatibleTypes) {
		t.Fatal("VARCHAR bound mismatch accepted", err)
	}
}

func TestPredicateSummaryPreconditions(t *testing.T) {
	typ := must(types.NewBoolType(foundation.NonNull))
	operand := reference(typ, "t", "x")
	empty := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	source := boolLiteral(foundation.NonNull, false)
	absent := foundation.Optional[foundation.TruthValue]{}
	for _, call := range []func(){
		func() { _, _ = facts.NewPredicateSummary(nil, empty, absent, source) },
		func() { _, _ = facts.NewPredicateSummary(operand, nil, absent, source) },
		func() {
			_, _ = facts.NewPredicateSummary(operand, (*finite.EquatableValueSet)(nil), absent, source)
		},
		func() { _, _ = facts.NewPredicateSummary(operand, empty, absent, nil) },
		func() {
			_, _ = facts.NewPredicateSummary(operand, empty, absent, (*expressions.ReferenceExpression)(nil))
		},
	} {
		expectAssertion(t, call)
	}
	for _, bad := range []*facts.PredicateSummary{nil, {}} {
		expectAssertion(t, func() { bad.Operand() })
		expectAssertion(t, func() { bad.TrueSet() })
		expectAssertion(t, func() { bad.NullTruth() })
		expectAssertion(t, func() { bad.Type() })
		expectAssertion(t, func() { bad.Source() })
	}
}
