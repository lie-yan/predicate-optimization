package fact_extraction_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

type diagnosticCase struct {
	name   string
	source expressions.Expression
	reason string
}

func diagnosticFixtures() (*fact_extraction.ScalarExtractor, []diagnosticCase) {
	context := must(testbed.NewStandardContext())
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	yes := boolLiteral(foundation.NonNull, true)
	no := boolLiteral(foundation.NonNull, false)
	handle := must(context.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{yes.Type()}))
	unsupported := must(expressions.NewCallExpression(context, handle, []expressions.Expression{yes}))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(integer, "t", "x")
	exact := comparison(context, foundation.Equal, ref, literal(must(testbedvalues.NewInteger(integer, 3))))
	return extractor, []diagnosticCase{
		{"ComparisonSemantics", unsupported, "scalar analysis: unsupported semantics: scalar comparison semantics not supported"},
		{"LogicalUnsupported", logical(foundation.And, unsupported, no), "scalar analysis: unsupported semantics: logical children are unsupported or refer to different operands"},
		{"ConstantOnly", logical(foundation.Or, yes, no), "scalar analysis: unsupported semantics: constant-only logical expression has no operand domain"},
		{"Exact", exact, ""},
	}
}

func TestFixedAnalysisDiagnostics(t *testing.T) {
	extractor, cases := diagnosticFixtures()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := expressions.PrettyPrint(tc.source)
			result := must(extractor.Extract(tc.source))
			if result.Source() != tc.source || result.Type() != tc.source.Type() {
				t.Fatal("result lost source or type identity")
			}
			_, summary := result.Summary().Get()
			if summary != (tc.reason == "") {
				t.Fatal("unexpected exact summary")
			}
			wantStatus := fact_extraction.Unsupported
			if tc.reason == "" {
				wantStatus = fact_extraction.Exact
			}
			if result.Status() != wantStatus {
				t.Fatal("unexpected status")
			}
			for range 3 {
				reason, present := result.Reason().Get()
				if reason != tc.reason || present != (tc.reason != "") {
					t.Fatalf("reason = %q, %v; want %q", reason, present, tc.reason)
				}
			}
			if expressions.PrettyPrint(tc.source) != before {
				t.Fatal("diagnostic access changed source")
			}
		})
	}
}

func TestFixedDiagnosticDoesNotHideJoinedFailure(t *testing.T) {
	base := must(testbed.NewStandardContext())
	context := &extractionContext{FunctionContext: base}
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	bad := comparison(context, foundation.Equal, reference(integer, "t", "x"), literal(must(testbedvalues.NewInteger(integer, 1))))
	extractor, cases := diagnosticFixtures()
	for _, category := range []error{functions.ErrContractViolation, functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrEvaluationFailure} {
		failure := errors.Join(value_set.ErrUnsupported, category)
		context.metadataError = failure
		for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
			for _, tc := range cases[:3] {
				_, err := extractor.Extract(logical(op, tc.source, bad))
				if !errors.Is(err, failure) || !errors.Is(err, category) {
					t.Fatalf("later provider failure hidden: %v", err)
				}
			}
		}
	}
}
