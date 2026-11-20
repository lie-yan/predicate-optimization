package reconstruction_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func comparison(context functioninterfaces.FunctionContext, op foundation.OperatorType, ref *expressions.ReferenceExpression, value valueinterfaces.Value) *expressions.CallExpression {
	return must(expressions.NewCallExpression(context, must(context.ComparisonFunction(op, ref.Type(), value.Type())), []expressions.Expression{ref, literal(value)}))
}

func pointPredicate(context functioninterfaces.FunctionContext, ref *expressions.ReferenceExpression, value valueinterfaces.Value) expressions.Expression {
	return logical(foundation.And, comparison(context, foundation.GreaterThanOrEqual, ref, value), comparison(context, foundation.LessThanOrEqual, ref, value))
}

func TestRangeReconstructionMatchesInterpreter(t *testing.T) {
	sourceContext, target := must(testfunctions.NewStandardContext()), must(testfunctions.NewStandardContext())
	reconstructor := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, target)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	var ints []valueinterfaces.Value
	for _, v := range []int64{math.MinInt64, math.MinInt64 + 1, -1, 0, 1, 3, 9, 10, 11, math.MaxInt64 - 1, math.MaxInt64} {
		ints = append(ints, must(testbedvalues.NewInteger(integer, v)))
	}
	var strings []valueinterfaces.Value
	for _, v := range []string{"", "\x00", "a", "a\x00", "a\x00\x00", "b", "\xff"} {
		strings = append(strings, must(testbedvalues.NewVarChar(text, v)))
	}
	domains := [][]valueinterfaces.Value{ints, strings}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		tiny, before, after := math.SmallestNonzeroFloat64, math.Nextafter(1, 0), math.Nextafter(1, 2)
		if width == types.Float32 {
			tiny, before, after = math.SmallestNonzeroFloat32, float64(math.Nextafter32(1, 0)), float64(math.Nextafter32(1, 2))
		}
		var floats []valueinterfaces.Value
		for _, v := range []float64{math.Inf(-1), -1, -tiny, math.Copysign(0, -1), 0, tiny, before, 1, after, 3, math.Inf(1), math.NaN()} {
			floats = append(floats, must(testbedvalues.NewFloat(typ, v)))
		}
		domains = append(domains, floats)
	}
	yes := literal(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true)))
	no := literal(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), false)))
	for _, domain := range domains {
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			typ := must(domain[0].Type().WithNullability(nullability))
			t.Run(typ.String(), func(t *testing.T) {
				ref := reference(typ)
				var inputs []valueinterfaces.Value
				for _, v := range domain {
					inputs = append(inputs, must(v.WithNullability(nullability)))
				}
				if nullability == foundation.MayNull {
					inputs = append(inputs, must(testbedvalues.NewNull(typ)))
				}
				var predicates []expressions.Expression
				for _, value := range domain {
					for _, op := range []foundation.OperatorType{foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual} {
						atom := comparison(sourceContext, op, ref, value)
						predicates = append(predicates, atom, logical(foundation.Not, atom), logical(foundation.And, atom, no), logical(foundation.Or, atom, yes))
					}
					predicates = append(predicates, pointPredicate(sourceContext, ref, value))
				}
				// Mix open and closed intervals, disconnected regions, points, and holes.
				for i := 0; i+1 < len(domain); i++ {
					lower, upper := domain[i], domain[i+1]
					for _, lowOp := range []foundation.OperatorType{foundation.GreaterThan, foundation.GreaterThanOrEqual} {
						for _, highOp := range []foundation.OperatorType{foundation.LessThan, foundation.LessThanOrEqual} {
							interval := logical(foundation.And, comparison(sourceContext, lowOp, ref, lower), comparison(sourceContext, highOp, ref, upper))
							predicates = append(predicates, interval, logical(foundation.Not, interval))
						}
					}
					disjunction := logical(foundation.Or, pointPredicate(sourceContext, ref, lower), pointPredicate(sourceContext, ref, upper))
					predicates = append(predicates, disjunction, logical(foundation.Not, disjunction))
				}
				for _, source := range predicates {
					summary := summaryOf(sourceContext, source)
					// Constant composition can select a discrete representation; convert
					// it exactly so this suite exercises the range reconstruction path.
					if discrete, ok := summary.TrueSet().(*finite.EquatableValueSet); ok {
						summary = must(facts.NewPredicateSummary(summary.Operand(), must(discrete.ToSortedRangeSet()), summary.NullTruth(), source))
					}
					candidate, present := must(reconstructor.Reconstruct(summary)).Get()
					empty, universal := must(summary.TrueSet().IsEmpty()), must(summary.TrueSet().IsUniversal())
					nullTruth, nullable := summary.NullTruth().Get()
					constantTruth := foundation.False
					if universal {
						constantTruth = foundation.True
					}
					expectAbsent := (empty || universal) && nullable && nullTruth != constantTruth
					if present == expectAbsent {
						t.Fatalf("unexpected presence for %s: %s", source, summary.TrueSet())
					}
					if !present {
						continue
					}
					checkEquivalent(t, target, source, candidate, inputs)
					if _, constant := candidate.(*expressions.LiteralExpression); !constant {
						roundTrip := summaryOf(target, candidate)
						if !must(summary.TrueSet().EqualsTo(roundTrip.TrueSet())) || summary.NullTruth() != roundTrip.NullTruth() {
							t.Fatalf("range round trip changed: %s -> %s", source, candidate)
						}
					}
					if summary.Source() != source {
						t.Fatal("source alternative changed")
					}
				}
			})
		}
	}
}

func TestRangeCandidateForms(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ)
	one, ten := must(testbedvalues.NewInteger(typ, 1)), must(testbedvalues.NewInteger(typ, 10))
	reconstruct := func(source expressions.Expression) expressions.Expression {
		candidate, present := must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context).Reconstruct(summaryOf(context, source))).Get()
		if !present {
			t.Fatal("expected range candidate")
		}
		return candidate
	}
	point := pointPredicate(context, ref, one)
	candidate := reconstruct(point)
	call, ok := candidate.(*expressions.CallExpression)
	if !ok {
		t.Fatal("point was not equality")
	}
	if op, _ := must(context.Metadata(call.Handle())).Operator().Get(); op != foundation.Equal {
		t.Fatal("wrong point operator")
	}
	excluded := reconstruct(logical(foundation.Not, point)).(*expressions.CallExpression)
	if op, _ := must(context.Metadata(excluded.Handle())).Operator().Get(); op != foundation.NotEqual {
		t.Fatal("isolated hole was not inequality")
	}
	points := logical(foundation.Or, point, pointPredicate(context, ref, ten))
	if _, ok := reconstruct(points).(*expressions.SpecialFormExpression); !ok {
		t.Fatal("points were not grouped into IN")
	}
	negated := reconstruct(logical(foundation.Not, points)).(*expressions.LogicalExpression)
	if _, ok := negated.Children()[0].(*expressions.SpecialFormExpression); negated.Operator() != foundation.Not || !ok {
		t.Fatal("isolated exclusions were not NOT IN")
	}
	bounded := reconstruct(logical(foundation.And, comparison(context, foundation.GreaterThanOrEqual, ref, one), comparison(context, foundation.LessThan, ref, ten))).(*expressions.LogicalExpression)
	if bounded.Operator() != foundation.And {
		t.Fatal("bounded range missing AND")
	}
	disconnected := reconstruct(logical(foundation.Or, comparison(context, foundation.LessThan, ref, one), comparison(context, foundation.GreaterThan, ref, ten))).(*expressions.LogicalExpression)
	if disconnected.Operator() != foundation.Or {
		t.Fatal("disconnected ranges missing OR")
	}
}

func TestRangeSizesAndSourceAlternative(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ)
	var entries []valueinterfaces.Value
	var points []*ranges.Range
	for _, n := range []int64{1, 2, 10, 11} {
		v := must(testbedvalues.NewInteger(typ, n))
		entries = append(entries, v)
		endpoint := must(ranges.NewRangeEndpoint(v, true))
		point, _ := must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, endpoint, endpoint)).Get()
		points = append(points, point)
	}
	source := membership(ref, entries...)
	// This exact range representation merges adjacent points into two intervals.
	set := must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, points, false))
	summary := must(facts.NewPredicateSummary(ref, set, foundation.Optional[foundation.TruthValue]{}, source))
	candidate, present := must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context).Reconstruct(summary)).Get()
	if !present {
		t.Fatal("range candidate missing")
	}
	sourceSize, candidateSize := reconstruction.MeasureExpression(source), reconstruction.MeasureExpression(candidate)
	if sourceSize != (reconstruction.ExpressionSize{NodeCount: 6, LiteralOccurrences: 4, MembershipEntries: 4}) {
		t.Fatalf("source size: %+v", sourceSize)
	}
	if candidateSize != (reconstruction.ExpressionSize{NodeCount: 15, LiteralOccurrences: 4}) {
		t.Fatalf("candidate size: %+v", candidateSize)
	}
	if summary.Source() != source || candidateSize.NodeCount <= sourceSize.NodeCount {
		t.Fatal("larger candidate displaced source alternative")
	}
	var inputs []valueinterfaces.Value
	for n := int64(0); n <= 12; n++ {
		inputs = append(inputs, must(testbedvalues.NewInteger(typ, n)))
	}
	checkEquivalent(t, context, source, candidate, inputs)
	// A shared subtree is counted once per occurrence, not once per pointer.
	shared := logical(foundation.And, source, source)
	if got := reconstruction.MeasureExpression(shared); got != (reconstruction.ExpressionSize{NodeCount: 13, LiteralOccurrences: 8, MembershipEntries: 8}) {
		t.Fatalf("shared occurrences: %+v", got)
	}
	expectAssertion(t, func() { reconstruction.MeasureExpression(nil) })
	expectAssertion(t, func() { reconstruction.MeasureExpression((*expressions.ReferenceExpression)(nil)) })
}
