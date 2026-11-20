package reconstruction_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

type externalFactory struct{ testbedvalues.ValueFactory }

func (f externalFactory) NewBool(typ *types.BoolType, b bool) (valueinterfaces.Value, error) {
	return fixtures.WrapScalar(must(f.ValueFactory.NewBool(typ, b))), nil
}

func (f externalFactory) NewFloat(typ *types.FloatType, n float64) (valueinterfaces.Value, error) {
	return fixtures.WrapScalar(must(f.ValueFactory.NewFloat(typ, n))), nil
}

func (f externalFactory) NewInteger(typ *types.IntType, n int64) (valueinterfaces.Value, error) {
	return fixtures.WrapScalar(must(f.ValueFactory.NewInteger(typ, n))), nil
}

func TestExternalExtractionReconstruction(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	extractionFactory := &fixtures.RecordingFactory{Delegate: externalFactory{}}
	reconstructionFactory := &fixtures.RecordingFactory{Delegate: externalFactory{}}
	extractor := fact_extraction.NewScalarExtractor(extractionFactory, context)
	reconstructor := reconstruction.NewScalarReconstructor(reconstructionFactory, context)
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	domains := [][]valueinterfaces.Value{
		{must(testbedvalues.NewBool(boolean, false)), must(testbedvalues.NewBool(boolean, true))},
		{must(testbedvalues.NewInteger(integer, -1)), must(testbedvalues.NewInteger(integer, 0)), must(testbedvalues.NewInteger(integer, 2))},
		{must(testbedvalues.NewVarChar(text, "")), must(testbedvalues.NewVarChar(text, "a\x00")), must(testbedvalues.NewVarChar(text, "z"))},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		var domain []valueinterfaces.Value
		for _, n := range []float64{math.Inf(-1), -1, math.Copysign(0, -1), 0, 1, math.Inf(1), math.NaN()} {
			domain = append(domain, must(testbedvalues.NewFloat(typ, n)))
		}
		domains = append(domains, domain)
	}
	for _, domain := range domains {
		for i, v := range domain {
			domain[i] = fixtures.WrapScalar(v)
		}
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			ref := reference(must(domain[0].Type().WithNullability(nullability)))
			var inputs []valueinterfaces.Value
			for _, v := range domain {
				inputs = append(inputs, must(v.WithNullability(nullability)))
			}
			if nullability == foundation.MayNull {
				inputs = append(inputs, fixtures.WrapScalar(must(testbedvalues.NewNull(ref.Type()))))
			}
			sources := []expressions.Expression{membership(ref, domain...)}
			for _, v := range domain {
				ops := []foundation.OperatorType{foundation.Equal}
				if ref.Type().IsOrderable() {
					ops = append(ops, foundation.NotEqual, foundation.LessThan, foundation.GreaterThanOrEqual)
				}
				for _, op := range ops {
					sources = append(sources, comparison(context, op, ref, v))
				}
			}
			// External Boolean literals must acquire the same domain from the sibling.
			sources = append(sources, logical(foundation.And, sources[0], literal(fixtures.WrapScalar(must(testbedvalues.NewBool(boolean, true))))))
			for _, source := range sources {
				result := must(extractor.Extract(source))
				summary, present := result.Summary().Get()
				if !present || summary.Source() != source || !summary.Type().EqualsTo(source.Type()) {
					t.Fatal("lost exactness or source type")
				}
				candidate, present := must(reconstructor.Reconstruct(summary)).Get()
				if !present { // Nullable empty/universal predicates cannot always become literals.
					if nullability == foundation.NonNull {
						t.Fatalf("unexpected absence for %s", source)
					}
					continue
				}
				checkEquivalent(t, context, source, candidate, inputs)
			}
		}
	}
	if len(extractionFactory.Requests) == 0 || len(reconstructionFactory.Requests) == 0 {
		t.Fatal("factories were bypassed")
	}
}

func TestAnalysisFactoryOwnershipAndFailures(t *testing.T) {
	context := must(testfunctions.NewStandardContext())
	extractionFactory := &fixtures.RecordingFactory{Delegate: externalFactory{}}
	reconstructionFactory := &fixtures.RecordingFactory{Delegate: externalFactory{}}
	extractor := fact_extraction.NewScalarExtractor(extractionFactory, context)
	reconstructor := reconstruction.NewScalarReconstructor(reconstructionFactory, context)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	source := comparison(context, foundation.GreaterThan, reference(integer), fixtures.WrapScalar(must(testbedvalues.NewInteger(integer, 2))))
	summary, _ := must(extractor.Extract(source)).Summary().Get()
	if len(extractionFactory.Requests) != 1 || extractionFactory.Requests[0].Payload != int64(3) {
		t.Fatal("integer bound did not use extraction factory")
	}
	extractionFactory.Requests = nil
	must(summary.TrueSet().Complement())
	if len(extractionFactory.Requests) == 0 || len(reconstructionFactory.Requests) != 0 {
		t.Fatal("extracted set lost its factory")
	}
	sentinel := errors.New("factory failed")
	extractionFactory.Failure = sentinel
	if _, err := extractor.Extract(source); !errors.Is(err, sentinel) {
		t.Fatalf("lost extraction failure: %v", err)
	}
	extractionFactory.Failure = nil
	floating := must(types.NewFloatType(types.Float64, foundation.NonNull))
	ref := reference(floating)
	negzero := fixtures.WrapScalar(must(testbedvalues.NewFloat(floating, math.Copysign(0, -1))))
	for _, predicate := range []expressions.Expression{membership(ref, negzero), comparison(context, foundation.Equal, ref, negzero)} {
		extractionFactory.Requests = nil
		must(extractor.Extract(predicate))
		if len(extractionFactory.Requests) != 1 || extractionFactory.Requests[0].Payload != float64(0) {
			t.Fatal("finite normalization bypassed factory")
		}
		extractionFactory.Failure = sentinel
		if _, err := extractor.Extract(predicate); !errors.Is(err, sentinel) {
			t.Fatal("finite normalization failure lost")
		}
		extractionFactory.Failure = nil
	}
	// Ordered universe excludes NaN; reconstruction synthesizes -Inf. NaN
	// equality instead gives an empty region reconstructed as a Boolean literal.
	for _, tc := range []struct {
		op     foundation.OperatorType
		number float64
		kind   string
	}{
		{foundation.GreaterThanOrEqual, math.Inf(-1), "Float"}, {foundation.Equal, math.NaN(), "Bool"},
	} {
		source := comparison(context, tc.op, ref, fixtures.WrapScalar(must(testbedvalues.NewFloat(floating, tc.number))))
		summary, _ := must(extractor.Extract(source)).Summary().Get()
		reconstructionFactory.Requests = nil
		candidate, present := must(reconstructor.Reconstruct(summary)).Get()
		if !present || len(reconstructionFactory.Requests) != 1 || reconstructionFactory.Requests[0].Kind != tc.kind {
			t.Fatal("reconstruction bypassed factory")
		}
		var result valueinterfaces.Value
		if tc.kind == "Bool" {
			result = candidate.(*expressions.LiteralExpression).Value()
		} else {
			result = candidate.(*expressions.CallExpression).Arguments()[1].(*expressions.LiteralExpression).Value()
		}
		if _, ok := result.(fixtures.ScalarView); !ok {
			t.Fatal("factory output representation lost")
		}
		reconstructionFactory.Failure = sentinel
		failed, err := reconstructor.Reconstruct(summary)
		if _, present := failed.Get(); present || !errors.Is(err, sentinel) {
			t.Fatal("reconstruction failure became absence or partial output")
		}
		reconstructionFactory.Failure = nil
	}
	// Provider refusal stays distinct from factory construction failure.
	providerFailure := errors.New("provider failed")
	broken := &provider{FunctionContext: context, lookup: func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		return nil, providerFailure
	}}
	summary = summaryOf(context, comparison(context, foundation.Equal, reference(integer), fixtures.WrapScalar(must(testbedvalues.NewInteger(integer, 2)))))
	if _, err := reconstruction.NewScalarReconstructor(reconstructionFactory, broken).Reconstruct(summary); !errors.Is(err, providerFailure) {
		t.Fatal("provider failure lost")
	}
	if _, err := fact_extraction.NewScalarExtractor(extractionFactory, broken).Extract(membership(reference(integer), fixtures.WrapScalar(must(testbedvalues.NewInteger(integer, 2))))); !errors.Is(err, providerFailure) {
		t.Fatal("extraction provider failure lost")
	}
}

func TestAnalysisNilFactories(t *testing.T) {
	if !assertions.Enabled {
		return
	}
	context := must(testfunctions.NewStandardContext())
	for _, factory := range []valueinterfaces.ValueFactory{nil, (*testbedvalues.ValueFactory)(nil)} {
		for _, construct := range []func(){func() { fact_extraction.NewScalarExtractor(factory, context) }, func() { reconstruction.NewScalarReconstructor(factory, context) }} {
			func() {
				defer func() {
					if recover() == nil {
						t.Error("nil factory accepted")
					}
				}()
				construct()
			}()
		}
	}
}
