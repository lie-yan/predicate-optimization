package fact_extraction_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
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

func membership(ref *expressions.ReferenceExpression, candidates ...expressions.Expression) *expressions.SpecialFormExpression {
	n := ref.Type().Nullability()
	for _, candidate := range candidates {
		if candidate.Type().Nullability() == foundation.MayNull {
			n = foundation.MayNull
		}
	}
	return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(n)), append([]expressions.Expression{ref}, candidates...)))
}

func TestExtractMatchesInterpreter(t *testing.T) {
	context := must(testbed.NewStandardContext())
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, context)
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	domains := [][]valueinterfaces.Value{
		{must(testbedvalues.NewBool(boolean, false)), must(testbedvalues.NewBool(boolean, true))},
		{must(testbedvalues.NewInteger(integer, math.MinInt64)), must(testbedvalues.NewInteger(integer, -1)), must(testbedvalues.NewInteger(integer, 0)), must(testbedvalues.NewInteger(integer, 3)), must(testbedvalues.NewInteger(integer, math.MaxInt64))},
		{must(testbedvalues.NewVarChar(text, "")), must(testbedvalues.NewVarChar(text, "a")), must(testbedvalues.NewVarChar(text, "a\x00")), must(testbedvalues.NewVarChar(text, "\xff"))},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		domain := []valueinterfaces.Value{}
		smallest := math.SmallestNonzeroFloat64
		if width == types.Float32 {
			smallest = math.SmallestNonzeroFloat32
		}
		for _, v := range []float64{math.Inf(-1), -1, -smallest, math.Copysign(0, -1), 0, smallest, 1, math.Inf(1), math.NaN()} {
			domain = append(domain, must(testbedvalues.NewFloat(typ, v)))
		}
		domains = append(domains, domain)
	}
	for _, domain := range domains {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			typ := must(domain[0].Type().WithNullability(n))
			ref := reference(typ, "t", "x")
			t.Run(typ.String(), func(t *testing.T) {
				var predicates []expressions.Expression
				for _, candidate := range domain {
					for _, op := range []foundation.OperatorType{foundation.Equal, foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual} {
						if _, ok := typ.(*types.BoolType); ok && op != foundation.Equal {
							continue
						}
						predicates = append(predicates, comparison(context, op, ref, literal(candidate)), comparison(context, op, literal(candidate), ref))
					}
					predicates = append(predicates, membership(ref, literal(candidate), literal(candidate)))
				}
				all := make([]expressions.Expression, len(domain))
				for i, v := range domain {
					all[i] = literal(v)
				}
				predicates = append(predicates, membership(ref, all...))
				inputs := make([]valueinterfaces.Value, 0, len(domain)+1)
				for _, v := range domain {
					inputs = append(inputs, must(v.WithNullability(n)))
				}
				if n == foundation.MayNull {
					inputs = append(inputs, must(testbedvalues.NewNull(typ)))
				}
				for _, predicate := range predicates {
					result := must(extractor.Extract(predicate))
					summary, present := result.Summary().Get()
					if result.Status() != fact_extraction.Exact || !present || result.Source() != predicate || result.Type() != predicate.Type() || summary.Operand() != ref {
						t.Fatal("exact source/type/operand not retained")
					}
					for _, input := range inputs {
						expected := must(evaluator.Evaluate(predicate, scalarEnvironment{input}))
						var truth foundation.TruthValue
						if input.IsNull() {
							var present bool
							truth, present = summary.NullTruth().Get()
							if !present {
								t.Fatal("missing null outcome")
							}
						} else {
							truth = foundation.False
							if must(summary.TrueSet().Contains(input)) {
								truth = foundation.True
							}
						}
						if expected.IsNull() {
							if truth != foundation.Null {
								t.Fatal("NULL mismatch")
							}
						} else if (truth == foundation.True) != expected.(*testbedvalues.BoolValue).Bool() || truth == foundation.Null {
							t.Fatalf("predicate %s input %s: truth %v, expected %s", predicate, input, truth, expected)
						}
					}
					if _, present := summary.NullTruth().Get(); present != (n == foundation.MayNull) {
						t.Fatal("null outcome presence mismatch")
					}
				}
			})
		}
	}
}

func TestExtractUnsupportedPreservesSource(t *testing.T) {
	context := must(testbed.NewStandardContext())
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ, "t", "x")
	one := literal(must(testbedvalues.NewInteger(typ, 1)))
	nullable := must(typ.WithNullability(foundation.MayNull))
	maybe := literal(must(testbedvalues.NewInteger(nullable.(*types.IntType), 1)))
	null := literal(must(testbedvalues.NewNull(nullable)))
	predicates := []expressions.Expression{
		membership(ref), membership(ref, one, reference(typ, "t", "y")), membership(ref, one, maybe), membership(ref, null),
		comparison(context, foundation.Equal, ref, maybe), comparison(context, foundation.Equal, ref, null),
		comparison(context, foundation.Equal, ref, reference(typ, "t", "y")), comparison(context, foundation.Equal, one, one),
		boolLiteral(foundation.NonNull, true),
	}
	for _, unsupported := range []types.Type{must(types.NewIntType(types.Int8, foundation.NonNull)), must(types.NewVarCharType(new(uint64), foundation.NonNull))} {
		r := reference(unsupported, "t", "x")
		var v valueinterfaces.Value
		switch typ := unsupported.(type) {
		case *types.IntType:
			v = must(testbedvalues.NewInteger(typ, 0))
		case *types.VarCharType:
			v = must(testbedvalues.NewVarChar(typ, ""))
		}
		predicates = append(predicates, membership(r, literal(v)))
	}
	for _, predicate := range predicates {
		result := must(extractor.Extract(predicate))
		if result.Status() != fact_extraction.Unsupported || result.Source() != predicate || result.Type() != predicate.Type() {
			t.Fatalf("unsupported source lost: %s", predicate)
		}
		if _, present := result.Summary().Get(); present {
			t.Fatal("unsupported acquired summary")
		}
	}
	if _, err := extractor.Extract(one); !errors.Is(err, value_set.ErrInvalidInput) {
		t.Fatalf("non-predicate: %v", err)
	}
	expectAssertion(t, func() { fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, nil) })
	expectAssertion(t, func() { extractor.Extract(nil) })
	expectAssertion(t, func() { new(fact_extraction.ScalarExtractor).Extract(comparison(context, foundation.Equal, ref, one)) })
}

// Lookup errors must survive extraction; Evaluate is never used by analysis.
type extractionContext struct {
	functioninterfaces.FunctionContext
	lookupError   error
	metadataError error
}

func (c *extractionContext) ComparisonFunction(op foundation.OperatorType, l, r types.Type) (functioninterfaces.FunctionHandle, error) {
	if c.lookupError != nil {
		return nil, c.lookupError
	}
	return c.FunctionContext.ComparisonFunction(op, l, r)
}

func (c *extractionContext) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if c.metadataError != nil {
		return nil, c.metadataError
	}
	return c.FunctionContext.Metadata(h)
}

func (*extractionContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("extraction must not evaluate")
}

func TestExtractProviderFailures(t *testing.T) {
	base := must(testbed.NewStandardContext())
	context := &extractionContext{FunctionContext: base}
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ, "t", "x")
	one := literal(must(testbedvalues.NewInteger(typ, 1)))
	call := comparison(context, foundation.Equal, ref, one)
	extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	for _, predicate := range []expressions.Expression{call, membership(ref, one)} {
		if must(extractor.Extract(predicate)).Status() != fact_extraction.Exact {
			t.Fatal("metadata-only extraction failed")
		}
	}
	for _, cause := range []error{functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation, errors.Join(functions.ErrContractViolation, value_set.ErrUnsupported)} {
		context.lookupError = cause
		if _, err := extractor.Extract(membership(ref, one)); !errors.Is(err, cause) {
			t.Fatalf("lookup failure hidden: %v", err)
		}
		context.metadataError = cause
		if _, err := extractor.Extract(call); !errors.Is(err, cause) {
			t.Fatalf("metadata failure hidden: %v", err)
		}
		context.metadataError = nil
	}
	context.lookupError = functions.ErrUnsupportedLookup
	if must(extractor.Extract(membership(ref, one))).Status() != fact_extraction.Unsupported {
		t.Fatal("unavailable overload is not unsupported")
	}
	// The membership context cannot resolve anything, but calls use their own context.
	separate := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
	if must(separate.Extract(comparison(base, foundation.Equal, ref, one))).Status() != fact_extraction.Exact {
		t.Fatal("call used membership context")
	}
}

func TestExtractDomainCompatibility(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	int8 := must(types.NewIntType(types.Int8, foundation.NonNull))
	int64 := must(types.NewIntType(types.Int64, foundation.NonNull))
	a, b := uint64(2), uint64(4)
	short, long := must(types.NewVarCharType(&a, foundation.NonNull)), must(types.NewVarCharType(&b, foundation.NonNull))
	for _, tc := range []struct {
		left  types.Type
		right valueinterfaces.Value
		exact bool
	}{
		{int8, must(testbedvalues.NewInteger(int8, 1)), true},
		{short, must(testbedvalues.NewVarChar(short, "a")), true},
		{int8, must(testbedvalues.NewInteger(int64, 1)), false},
		{short, must(testbedvalues.NewVarChar(long, "a")), false},
	} {
		signature := must(functions.NewFunctionSignature([]types.Type{tc.left, tc.right.Type()}, boolean))
		metadata := must(functions.NewFunctionMetadata(signature, "equal", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}))
		catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "equal", Metadata: metadata}}))
		provider := testbed.NewContext(catalog)
		ref := reference(tc.left, "t", "x")
		result := must(fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, provider).Extract(comparison(provider, foundation.Equal, ref, literal(tc.right))))
		if (result.Status() == fact_extraction.Exact) != tc.exact {
			t.Fatal("domain admission mismatch")
		}
		if tc.exact {
			summary, _ := result.Summary().Get()
			if !must(summary.TrueSet().Contains(tc.right)) {
				t.Fatal("supported overload lost its equality region")
			}
			// No evaluator is installed: equality resolution and metadata suffice.
			if must(fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, provider).Extract(membership(ref, literal(tc.right)))).Status() != fact_extraction.Exact {
				t.Fatal("custom membership overload unsupported")
			}
		} else {
			// Unlike a general call, an IN node rejects mixed domains at construction.
			if _, err := expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{ref, literal(tc.right)}); err == nil {
				t.Fatal("mixed-domain IN constructed")
			}
		}
	}
}

func TestMembershipNaNCompactionPreservesSourceAndAdmission(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		ref := reference(typ, "t", "x")
		context := &extractionContext{FunctionContext: must(testbed.NewStandardContext())}
		extractor := fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context)
		value := func(v float64) *expressions.LiteralExpression { return literal(must(testbedvalues.NewFloat(typ, v))) }
		nan, zero, one := value(math.NaN()), value(math.Copysign(0, -1)), value(1)
		source := membership(ref, nan, one, nan, zero, one, nan)
		children := source.Children()
		for range 2 {
			result := must(extractor.Extract(source))
			summary, present := result.Summary().Get()
			if !present || result.Source() != source || !must(summary.TrueSet().Contains(one.Value())) || !must(summary.TrueSet().Contains(value(0).Value())) || must(summary.TrueSet().Contains(nan.Value())) || must(summary.TrueSet().Contains(value(2).Value())) {
				t.Fatal("compaction changed membership")
			}
			for i, child := range children {
				if source.ChildAt(i) != child {
					t.Fatal("compaction mutated source children")
				}
			}
			if !math.IsNaN(nan.Value().(valueinterfaces.FloatValue).Float()) || !math.Signbit(zero.Value().(valueinterfaces.FloatValue).Float()) {
				t.Fatal("set normalization mutated source payloads")
			}
		}
		allNaN := membership(ref, nan, nan)
		summary, present := must(extractor.Extract(allNaN)).Summary().Get()
		if !present || !must(summary.TrueSet().IsEmpty()) {
			t.Fatal("all-NaN list must produce an empty true set")
		}
		failure := errors.New("equality lookup failed")
		context.lookupError = failure
		if _, err := extractor.Extract(allNaN); !errors.Is(err, failure) {
			t.Fatalf("all-NaN filtering bypassed provider failure: %v", err)
		}
		// Invalid candidates must still be rejected before querying the provider.
		invalid := membership(ref, nan, reference(typ, "t", "other"))
		result, err := extractor.Extract(invalid)
		if err != nil || result.Status() != fact_extraction.Unsupported {
			t.Fatalf("candidate validation moved after lookup: %v", err)
		}
	}
}
