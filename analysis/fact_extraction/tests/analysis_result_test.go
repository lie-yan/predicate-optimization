package fact_extraction_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/facts"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestAnalysisResultDistinguishesExactEmptyNullAndUnsupported(t *testing.T) {
	typ := must(types.NewBoolType(foundation.MayNull))
	ref := reference(typ, "t", "x")
	not := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{ref}))
	source := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{ref, not}))
	empty := must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	summary := must(facts.NewPredicateSummary(ref, empty, foundation.Some(foundation.Null), source))
	exact := fact_extraction.NewExactAnalysisResult(summary)
	if exact.Status() != fact_extraction.Exact || exact.Source() != source || exact.Type() != source.Type() {
		t.Fatal("exact result lost status/source/type")
	}
	got, present := exact.Summary().Get()
	if !present || got != summary || !must(got.TrueSet().IsEmpty()) {
		t.Fatal("exact empty region confused with absent summary")
	}
	if truth, present := got.NullTruth().Get(); !present || truth != foundation.Null {
		t.Fatal("known NULL confused with unavailable analysis")
	}
	if _, present := exact.Reason().Get(); present {
		t.Fatal("exact result acquired unsupported reason")
	}
	unsupported := must(fact_extraction.NewUnsupportedAnalysisResult(source, "operator semantics not admitted"))
	if unsupported.Status() != fact_extraction.Unsupported || unsupported.Source() != source || unsupported.Type() != source.Type() {
		t.Fatal("unsupported result lost source/type")
	}
	if _, present := unsupported.Summary().Get(); present {
		t.Fatal("unsupported result acquired exact summary")
	}
	if reason, present := unsupported.Reason().Get(); !present || reason != "operator semantics not admitted" {
		t.Fatal("unsupported reason lost")
	}
}

func TestAnalysisResultRejectsInvalidConstruction(t *testing.T) {
	source := boolLiteral(foundation.NonNull, true)
	number := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(must(types.NewIntType(types.Int64, foundation.NonNull)), 1))))
	if _, err := fact_extraction.NewUnsupportedAnalysisResult(source, ""); err == nil {
		t.Fatal("empty unsupported reason accepted")
	}
	if _, err := fact_extraction.NewUnsupportedAnalysisResult(number, "not Boolean"); err == nil {
		t.Fatal("non-predicate source accepted")
	}
	expectAssertion(t, func() { fact_extraction.NewExactAnalysisResult(nil) })
	expectAssertion(t, func() { fact_extraction.NewExactAnalysisResult(&facts.PredicateSummary{}) })
	expectAssertion(t, func() { _, _ = fact_extraction.NewUnsupportedAnalysisResult(nil, "missing") })
	expectAssertion(t, func() {
		_, _ = fact_extraction.NewUnsupportedAnalysisResult((*expressions.ReferenceExpression)(nil), "missing")
	})
	var zero fact_extraction.AnalysisResult
	for _, call := range []func(){func() { zero.Status() }, func() { zero.Source() }, func() { zero.Type() }, func() { zero.Summary() }, func() { zero.Reason() }} {
		expectAssertion(t, call)
	}
}
