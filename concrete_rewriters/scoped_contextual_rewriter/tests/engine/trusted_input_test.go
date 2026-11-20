package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

type cleanupObservedHandle struct {
	functioninterfaces.FunctionHandle
	argumentQueries int
}

func (h *cleanupObservedHandle) ArgumentTypes() []types.Type {
	h.argumentQueries++
	return h.FunctionHandle.ArgumentTypes()
}

func (h *cleanupObservedHandle) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	o, ok := other.(*cleanupObservedHandle)
	return ok && h == o
}

func TestCleanupTrustsStoredCallValidity(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	originalHandle := must(provider.ComparisonFunction(foundation.Equal, integer, integer))
	handle := &cleanupObservedHandle{FunctionHandle: originalHandle}
	context := &eligibilityContext{FunctionContext: provider}
	context.metadata = func(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
		if h != handle {
			return nil, functions.ErrIncompatibleHandle
		}
		return provider.Metadata(originalHandle)
	}
	ref := eligibilityReference(integer)
	one := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integer, 1))))
	two := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integer, 2))))
	call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{ref, one}))
	if err := utils.ValidateForOptimization(call); err != nil {
		t.Fatal(err)
	}
	handle.argumentQueries, context.queries = 0, 0
	cleaner := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	if admitted, err := cleaner.CanOmit(call); err != nil || !admitted || context.queries != 1 || handle.argumentQueries != 0 {
		t.Fatalf("omission repeated stored-call validity checks: %v", err)
	}
	context.queries = 0
	if admitted, err := cleaner.CanChangeChildren(call, []expressions.Expression{ref, two}); err != nil || !admitted || context.queries != 1 || handle.argumentQueries != 0 {
		t.Fatalf("changed-child admission repeated stored-call validity checks: %v", err)
	}
	// Exact per-position Types remain a candidate-acceptance obligation even
	// though the original stored call is already valid.
	maybe := must(expressions.NewLiteralExpression(must(two.Value().WithNullability(foundation.MayNull))))
	for _, children := range [][]expressions.Expression{{ref}, {ref, maybe}} {
		if admitted, err := cleaner.CanChangeChildren(call, children); admitted || !errors.Is(err, rewriter.ErrInvalidReplacement) {
			t.Fatalf("changed shape/Type escaped acceptance checks: %v", err)
		}
	}
	if call.Arguments()[1] != one {
		t.Fatal("eligibility changed the source")
	}
}

func TestCleanupInspectsNewMembershipEquality(t *testing.T) {
	context := membershipEligibilityProvider()
	provider := context.FunctionContext
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	originalHandle := must(provider.ComparisonFunction(foundation.Equal, integer, integer))
	handle := &cleanupObservedHandle{FunctionHandle: originalHandle}
	lookups := 0
	context.lookup = func(operator foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
		lookups++
		if operator != foundation.Equal || !left.EqualsTo(integer) || !right.EqualsTo(integer) {
			t.Fatal("wrong membership equality resolution")
		}
		return handle, nil
	}
	context.metadata = func(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
		if h != handle {
			return nil, functions.ErrIncompatibleHandle
		}
		return provider.Metadata(originalHandle)
	}
	source := membershipExpression(eligibilityReference(integer), membershipCandidate(integer))
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	if lookups != 0 || handle.argumentQueries != 0 {
		t.Fatal("input validation should not resolve IN equality")
	}
	if admitted, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); err != nil || !admitted || lookups != 1 || handle.argumentQueries != 1 {
		t.Fatalf("new equality invocation skipped its inspection boundary: %v", err)
	}
}
