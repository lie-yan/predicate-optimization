package expressions_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Only Metadata is implemented; lookup, classification and evaluation would
// panic through the nil embedded interface. The delegate supplies paired identity.
type optimizationContext struct {
	functioninterfaces.FunctionContext
	delegate functioninterfaces.FunctionContext
	queries  int
}

func (c *optimizationContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.queries++
	return c.delegate.Metadata(handle)
}

func TestValidateForOptimizationUsesEachCallContext(t *testing.T) {
	first, second := fixture(testbed.NewStandardContext()), fixture(testbed.NewStandardContext())
	leftContext, rightContext := &optimizationContext{delegate: first}, &optimizationContext{delegate: second}
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	literal := logicalLiteral(foundation.NonNull, true)
	leftHandle := fixture(first.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{boolean}))
	rightHandle := fixture(second.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{boolean}))
	left := fixture(expressions.NewCallExpression(leftContext, leftHandle, []expressions.Expression{literal}))
	// The outer call and its argument call belong to independent providers.
	right := fixture(expressions.NewCallExpression(rightContext, rightHandle, []expressions.Expression{left}))
	root := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{left, right, right}))
	leftContext.queries, rightContext.queries = 0, 0
	before := expressions.PrettyPrint(root)
	if err := utils.Validate(root); err != nil || leftContext.queries != 0 || rightContext.queries != 0 {
		t.Fatalf("structural validation queried a provider: %v", err)
	}
	if err := utils.ValidateForOptimization(root); err != nil {
		t.Fatal(err)
	}
	if leftContext.queries != 1 || rightContext.queries != 1 || expressions.PrettyPrint(root) != before {
		t.Fatal("shared calls should be inspected once without mutation")
	}
	// A provider deliberately loses the pairing after valid construction. Entry
	// validation must diagnose it rather than borrow the other call's context.
	rightContext.delegate = first
	if err := utils.ValidateForOptimization(root); !errors.Is(err, functions.ErrIncompatibleHandle) || !strings.Contains(err.Error(), "child 1") {
		t.Fatalf("pairing failure or node location lost: %v", err)
	}
	rightContext.delegate = second
	if err := utils.ValidateForOptimization(root); err != nil {
		t.Fatalf("fresh validation after restored pairing failed: %v", err)
	}
}

func TestValidateForOptimizationAcceptsUnsupportedRegions(t *testing.T) {
	boolean := fixture(types.NewBoolType(foundation.MayNull))
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	metadata := fixture(functions.NewFunctionMetadata(fixture(functions.NewFunctionSignature([]types.Type{integer}, boolean)), "custom", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.NullPolicyUnspecified}))
	catalog := fixture(testbed.NewCatalog([]testbed.Definition{{Namespace: "custom", Name: "unclassified", Metadata: metadata}}))
	context := &optimizationContext{delegate: testbed.NewContext(catalog)}
	ref := fixture(expressions.NewReferenceExpression(externalColumnID{path: []string{"table", "x"}}, integer, "x"))
	call := fixture(expressions.NewCallExpression(context, catalog.Handles()[0], []expressions.Expression{ref}))
	opaque := fixture(expressions.NewOpaqueExpression(fixture(ids.NewOpaqueID("adapter", "unknown")), boolean, "opaque"))
	root := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{call, opaque}))
	context.queries = 0
	if err := utils.ValidateForOptimization(root); err != nil || context.queries != 1 {
		t.Fatalf("valid unsupported graph rejected: %v", err)
	}
	// No environment exists in this fixture: x has no runtime binding, and the
	// metadata-only function has no executable implementation to probe.
}

func TestValidateForOptimizationChecksStoredBindingAndProviderFailures(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	handle := fixture(provider.ComparisonFunction(foundation.Equal, integer, integer))
	metadata := fixture(provider.Metadata(handle))
	context := &callBoundaryContext{metadata: metadata}
	one := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	call := fixture(expressions.NewCallExpression(context, handle, []expressions.Expression{one, one}))
	root := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{call}))
	alternative := func(args []types.Type, result types.Type) *functions.FunctionMetadata {
		policies := make([]functions.NullPolicy, len(args))
		for i := range policies {
			policies[i] = functions.PropagateNull
		}
		return fixture(functions.NewFunctionMetadata(fixture(functions.NewFunctionSignature(args, result)), "comparison", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Some(foundation.Equal), policies))
	}
	sentinel := errors.New("entry metadata failed")
	for _, tc := range []struct {
		name     string
		metadata *functions.FunctionMetadata
		failure  error
		want     error
	}{
		{"missing metadata", nil, nil, functions.ErrContractViolation},
		{"handle arity", alternative([]types.Type{integer}, boolean), nil, functions.ErrContractViolation},
		{"handle Types", alternative([]types.Type{boolean, boolean}, boolean), nil, functions.ErrContractViolation},
		{"stored result", alternative([]types.Type{integer, integer}, fixture(boolean.WithNullability(foundation.MayNull))), nil, functions.ErrContractViolation},
		{"provider failure", metadata, sentinel, sentinel},
		{"joined failure", metadata, errors.Join(sentinel, functions.ErrUnsupportedLookup, functions.ErrContractViolation), sentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Immutable nodes stay unchanged. This deliberately broken provider
			// exposes inconsistencies after construction without unsafe node edits.
			context.metadata, context.failure = tc.metadata, tc.failure
			context.queries = 0
			if err := utils.Validate(root); err != nil || context.queries != 0 {
				t.Fatalf("structural mode consulted changed provider: %v", err)
			}
			err := utils.ValidateForOptimization(root)
			if !errors.Is(err, tc.want) || context.queries != 1 || !strings.Contains(err.Error(), "child 0") {
				t.Fatalf("entry error cause/location lost: %v", err)
			}
			if tc.name == "joined failure" && (!errors.Is(err, functions.ErrContractViolation) || !errors.Is(err, functions.ErrUnsupportedLookup)) {
				t.Fatal("joined categories lost")
			}
			context.metadata, context.failure = metadata, nil
			if err := utils.ValidateForOptimization(root); err != nil {
				t.Fatalf("retry failed: %v", err)
			}
		})
	}
}

func TestValidateForOptimizationRejectsIdentityConflictsAndNil(t *testing.T) {
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	nullable := fixture(boolean.WithNullability(foundation.MayNull))
	ref := func(typ types.Type) expressions.Expression {
		return fixture(expressions.NewReferenceExpression(externalColumnID{path: []string{"table", "x"}}, typ, ""))
	}
	opaque := func(typ types.Type) expressions.Expression {
		return fixture(expressions.NewOpaqueExpression(fixture(ids.NewOpaqueID("adapter", "x")), typ, ""))
	}
	for _, node := range []func(types.Type) expressions.Expression{ref, opaque} {
		conflict := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{node(boolean), node(nullable)}))
		if err := utils.ValidateForOptimization(conflict); err == nil {
			t.Fatal("conflicting identity Types accepted")
		}
		consistent := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{node(boolean), node(boolean)}))
		if err := utils.ValidateForOptimization(consistent); err != nil {
			t.Fatal("equal identities with consistent Types rejected", err)
		}
	}
	var nilLiteral *expressions.LiteralExpression
	for _, root := range []expressions.Expression{nil, nilLiteral} {
		if err := utils.ValidateForOptimization(root); err == nil {
			t.Fatal("nil graph accepted")
		}
	}
}
