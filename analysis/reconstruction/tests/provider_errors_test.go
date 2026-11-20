package reconstruction_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type provider struct {
	functioninterfaces.FunctionContext
	lookup   func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error)
	metadata func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error)
}

func (c *provider) ComparisonFunction(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
	if c.lookup != nil {
		return c.lookup(op, left, right)
	}
	return c.FunctionContext.ComparisonFunction(op, left, right)
}

func (c *provider) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if c.metadata != nil {
		return c.metadata(handle)
	}
	return c.FunctionContext.Metadata(handle)
}

func (*provider) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("reconstruction must not evaluate")
}

func TestReconstructionPreservesProviderFailures(t *testing.T) {
	base := must(testfunctions.NewStandardContext())
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := reference(typ)
	one, two := must(testbedvalues.NewInteger(typ, 1)), must(testbedvalues.NewInteger(typ, 2))
	summaries := []*facts.PredicateSummary{
		summaryOf(base, membership(ref, one)),
		summaryOf(base, logical(foundation.Not, membership(ref, one))),
		summaryOf(base, membership(ref, one, two)),
		summaryOf(base, logical(foundation.Not, membership(ref, one, two))),
		summaryOf(base, comparison(base, foundation.LessThan, ref, one)),
		summaryOf(base, logical(foundation.And, comparison(base, foundation.GreaterThanOrEqual, ref, one), comparison(base, foundation.LessThanOrEqual, ref, two))),
		summaryOf(base, logical(foundation.Or, comparison(base, foundation.LessThan, ref, one), comparison(base, foundation.GreaterThan, ref, two))),
	}
	for _, summary := range summaries {
		// A metadata-only context can reconstruct without an executable evaluator.
		if _, present := must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, &provider{FunctionContext: base}).Reconstruct(summary)).Get(); !present {
			t.Fatal("metadata-only reconstruction failed")
		}
		for _, cause := range []error{functions.ErrUnsupportedLookup, functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation, errors.New("provider failure"), errors.Join(functions.ErrUnsupportedLookup, functions.ErrContractViolation)} {
			c := &provider{FunctionContext: base, lookup: func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
				return nil, fmt.Errorf("lookup: %w", cause)
			}}
			result, err := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, c).Reconstruct(summary)
			if _, present := result.Get(); present || !errors.Is(err, cause) {
				t.Fatalf("lookup error hidden: %v", err)
			}
			c = &provider{FunctionContext: base, metadata: func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
				return nil, fmt.Errorf("metadata: %w", cause)
			}}
			result, err = reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, c).Reconstruct(summary)
			if _, present := result.Get(); present || !errors.Is(err, cause) {
				t.Fatalf("metadata error hidden: %v", err)
			}
		}
		for _, c := range []*provider{
			{FunctionContext: base, lookup: func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
				return nil, nil
			}},
			{FunctionContext: base, metadata: func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, nil }},
			{FunctionContext: base, lookup: func(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
				if op == foundation.Equal {
					return base.ComparisonFunction(foundation.LessThan, left, right)
				}
				return base.ComparisonFunction(foundation.Equal, left, right)
			}},
		} {
			result, err := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, c).Reconstruct(summary)
			if _, present := result.Get(); present || !errors.Is(err, functions.ErrContractViolation) {
				t.Fatalf("provider contract violation hidden: %v", err)
			}
		}
		// Admit only comparisons with the established non-null result and propagation contract.
		for _, change := range []string{"arguments", "nullable result", "null policy"} {
			c := &provider{FunctionContext: base, metadata: func(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
				metadata := must(base.Metadata(handle))
				args, result := metadata.Signature().ArgumentTypes(), metadata.Signature().ResultType()
				policies := metadata.NullPolicies()
				switch change {
				case "arguments":
					args[0] = must(types.NewIntType(types.Int32, foundation.NonNull))
				case "nullable result":
					result = must(result.WithNullability(foundation.MayNull))
				case "null policy":
					policies[0] = functions.NullPolicyUnspecified
				}
				return functions.NewFunctionMetadata(must(functions.NewFunctionSignature(args, result)), "fixture", foundation.Optional[functions.BuiltInFunctionName]{}, metadata.Operator(), policies)
			}}
			expected := value_set.ErrUnsupported
			if change == "arguments" {
				expected = functions.ErrContractViolation
			}
			result, err := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, c).Reconstruct(summary)
			if _, present := result.Get(); present || !errors.Is(err, expected) {
				t.Fatalf("unadmitted %s produced candidate: %v", change, err)
			}
		}
	}
}

func expectAssertion(t *testing.T, call func()) {
	t.Helper()
	if !assertions.Enabled {
		return
	}
	defer func() {
		if recover() == nil {
			t.Error("expected assertion")
		}
	}()
	call()
}

func TestReconstructorPreconditions(t *testing.T) {
	base := must(testfunctions.NewStandardContext())
	ref := reference(must(types.NewIntType(types.Int64, foundation.NonNull)))
	source := membership(ref, must(testbedvalues.NewInteger(ref.Type().(*types.IntType), 1)))
	summary := summaryOf(base, source)
	reconstructor := reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, base)
	expectAssertion(t, func() { reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, nil) })
	expectAssertion(t, func() { reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, (*provider)(nil)) })
	for _, bad := range []*reconstruction.ScalarReconstructor{nil, {}} {
		expectAssertion(t, func() { _, _ = bad.Reconstruct(summary) })
	}
	for _, bad := range []*facts.PredicateSummary{nil, {}} {
		expectAssertion(t, func() { _, _ = reconstructor.Reconstruct(bad) })
	}
}
