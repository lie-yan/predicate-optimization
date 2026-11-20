package fact_extraction_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Metadata-only fixtures supply overloads absent from the standard testbed.
func orderedProvider(typ types.Type, op foundation.OperatorType) *testbed.Context {
	boolean := must(types.NewBoolType(foundation.NonNull))
	signature := must(functions.NewFunctionSignature([]types.Type{typ, typ}, boolean))
	metadata := must(functions.NewFunctionMetadata(signature, "ordered", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Some(op), []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}))
	return testbed.NewContext(must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "ordered", Metadata: metadata}})))
}

func TestOrderedExtractionDiscreteDomains(t *testing.T) {
	int8 := must(types.NewIntType(types.Int8, foundation.NonNull))
	intDomain := make([]valueinterfaces.Value, 256)
	for i := range intDomain {
		intDomain[i] = must(testbedvalues.NewInteger(int8, int64(i-128)))
	}
	bound := uint64(1)
	text := must(types.NewVarCharType(&bound, foundation.NonNull))
	textDomain := []valueinterfaces.Value{must(testbedvalues.NewVarChar(text, ""))}
	for i := 0; i < 256; i++ {
		textDomain = append(textDomain, must(testbedvalues.NewVarChar(text, string([]byte{byte(i)}))))
	}
	// Enumeration is in semantic order, providing an independent rank oracle.
	for _, domain := range [][]valueinterfaces.Value{intDomain, textDomain} {
		typ := domain[0].Type()
		for _, op := range []foundation.OperatorType{foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual} {
			provider := orderedProvider(typ, op)
			extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, provider)
			ref := reference(must(typ.WithNullability(foundation.MayNull)), "t", "x")
			for _, boundary := range []int{0, 1, 10, 127, len(domain) - 2, len(domain) - 1} {
				for _, literalLeft := range []bool{false, true} {
					var left, right expressions.Expression = ref, literal(domain[boundary])
					if literalLeft {
						left, right = right, left
					}
					call := comparison(provider, op, left, right)
					result := must(extractor.Extract(call))
					summary, present := result.Summary().Get()
					if !present {
						t.Fatal("ordered overload unsupported")
					}
					if _, ok := summary.TrueSet().(*sets.SortedRangeSet); !ok {
						t.Fatal("expected range representation")
					}
					if truth, present := summary.NullTruth().Get(); !present || truth != foundation.Null {
						t.Fatal("null outcome lost")
					}
					for rank, input := range domain {
						a, b := rank, boundary
						if literalLeft {
							a, b = b, a
						}
						var expected bool
						switch op {
						case foundation.LessThan:
							expected = a < b
						case foundation.LessThanOrEqual:
							expected = a <= b
						case foundation.GreaterThan:
							expected = a > b
						case foundation.GreaterThanOrEqual:
							expected = a >= b
						}
						if got := must(summary.TrueSet().Contains(input)); got != expected {
							t.Fatalf("type %s op %v boundary %d rank %d literalLeft %v", typ, op, boundary, rank, literalLeft)
						}
					}
				}
			}
		}
	}
}

func TestOrderedExtractionRejectsBooleanAndCompositeDomains(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	array := must(types.NewArrayType(boolean, foundation.NonNull))
	structure := must(types.NewStructType(nil, foundation.NonNull))
	for _, typ := range []types.Type{boolean, array, structure} {
		provider := orderedProvider(typ, foundation.LessThan)
		ref := reference(typ, "t", "x")
		// Domain admission precedes operand-pattern extraction.
		call := comparison(provider, foundation.LessThan, ref, ref)
		result := must(fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, provider).Extract(call))
		if result.Status() != fact_extraction.Unsupported || result.Source() != call || result.Type() != call.Type() {
			t.Fatal("unsupported ordering domain admitted")
		}
	}
}
