package expressions_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// All operations except Metadata panic through the embedded nil interface:
// construction/reconstruction must neither resolve nor evaluate functions.
// Tests deliberately break provider responses to exercise the boundary; these
// mutations are not examples of the required stable production metadata contract.
type callBoundaryContext struct {
	functioninterfaces.FunctionContext
	metadata *functions.FunctionMetadata
	failure  error
	queries  int
}

func (c *callBoundaryContext) Metadata(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.queries++
	return c.metadata, c.failure
}

func TestCallConstructionChecksHandleMetadataAgreement(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	narrow := fixture(types.NewIntType(types.Int32, foundation.NonNull))
	nullable := fixture(integer.WithNullability(foundation.MayNull))
	one := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	handle := fixture(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer}))
	for _, tc := range []struct {
		name       string
		parameters []types.Type
		arguments  []expressions.Expression
	}{
		{"arity", []types.Type{integer}, []expressions.Expression{one}},
		{"width", []types.Type{integer, narrow}, []expressions.Expression{one, one}},
		{"implementation nullability", []types.Type{nullable, integer}, []expressions.Expression{one, one}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := fixture(functions.NewFunctionMetadata(fixture(functions.NewFunctionSignature(tc.parameters, integer)), "custom", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, make([]functions.NullPolicy, len(tc.parameters))))
			context := &callBoundaryContext{metadata: metadata}
			call, err := expressions.NewCallExpression(context, handle, tc.arguments)
			if call != nil || !errors.Is(err, functions.ErrContractViolation) || context.queries != 1 {
				t.Fatalf("inconsistent handle/metadata not rejected before binding: %v", err)
			}
		})
	}
}

func TestCallBoundaryFailuresAndReconstruction(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := fixture(types.NewIntType(types.Int64, foundation.MayNull))
	one := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	two := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 2))))
	maybe := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(nullable, 3))))
	handle := fixture(provider.ArithmeticFunction(foundation.Add, []types.Type{nullable, integer}))
	metadata := fixture(provider.Metadata(handle))
	context := &callBoundaryContext{metadata: metadata}
	arguments := []expressions.Expression{maybe, one}
	call := fixture(expressions.NewCallExpression(context, handle, arguments))
	if call.Context() != context || call.Handle() != handle || !call.Type().EqualsTo(nullable) || metadata.Signature().ResultType().Nullability() != foundation.NonNull {
		t.Fatal("construction lost pairing or confused implementation and effective Types")
	}
	before := expressions.PrettyPrint(call)
	sentinel := errors.New("metadata unavailable")
	badSignature := fixture(functions.NewFunctionSignature([]types.Type{integer}, integer))
	badMetadata := fixture(functions.NewFunctionMetadata(badSignature, "wrong arity", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.PropagateNull}))
	for _, tc := range []struct {
		name     string
		metadata *functions.FunctionMetadata
		failure  error
		want     error
	}{
		{"missing metadata", nil, nil, functions.ErrContractViolation},
		{"mismatched metadata", badMetadata, nil, functions.ErrContractViolation},
		{"provider failure", metadata, sentinel, sentinel},
		{"wrapped unsupported failure", metadata, errors.Join(sentinel, functions.ErrUnsupportedLookup), sentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context.metadata, context.failure = tc.metadata, tc.failure
			context.queries = 0
			if got := fixture(call.WithChildren(call.Children())); got != call || context.queries != 0 {
				t.Fatal("unchanged call should retain already constructed identity")
			}
			created, err := expressions.NewCallExpression(context, handle, arguments)
			if created != nil || !errors.Is(err, tc.want) {
				t.Fatalf("construction failure lost: %v", err)
			}
			rebuilt, err := call.WithChildren([]expressions.Expression{maybe, two})
			if rebuilt != nil || !errors.Is(err, tc.want) || context.queries != 2 {
				t.Fatalf("reconstruction bypassed inspection: %v", err)
			}
			if errors.Is(tc.failure, functions.ErrUnsupportedLookup) && !errors.Is(err, functions.ErrUnsupportedLookup) {
				t.Fatal("wrapped unsupported cause lost")
			}
			if expressions.PrettyPrint(call) != before {
				t.Fatal("failed construction changed source")
			}
			context.metadata, context.failure = metadata, nil
			retry := fixture(call.WithChildren([]expressions.Expression{maybe, two})).(*expressions.CallExpression)
			if retry.Context() != context || retry.Handle() != handle || !retry.Type().EqualsTo(call.Type()) || retry.Children()[1] != two {
				t.Fatal("retry lost pairing, effective Type or retained child")
			}
		})
	}
	// Valid handle/metadata with incompatible actual arguments is an ordinary
	// binding failure, rather than a provider consistency failure.
	for _, args := range [][]expressions.Expression{{one}, {one, logicalLiteral(foundation.NonNull, true)}} {
		for _, construct := range []func() (expressions.Expression, error){
			func() (expressions.Expression, error) {
				result, err := expressions.NewCallExpression(context, handle, args)
				if err != nil {
					return nil, err
				}
				return result, nil
			},
			func() (expressions.Expression, error) { return call.WithChildren(args) },
		} {
			got, err := construct()
			if got != nil || !errors.Is(err, functions.ErrUnsupportedLookup) || errors.Is(err, functions.ErrContractViolation) {
				t.Fatalf("ordinary binding failure changed category: %v", err)
			}
		}
	}
}
