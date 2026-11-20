package functions_test

import (
	"errors"
	"fmt"
	"testing"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// unavailableContext is an external conformance fixture, not a production
// context: it rejects all handles and lookup requests and has no evaluator.
type unavailableContext struct{}

var _ functioninterfaces.FunctionContext = unavailableContext{}

func (unavailableContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, _ []types.Type) (functioninterfaces.FunctionHandle, error) {
	return nil, fmt.Errorf("%s: %w", name, functions.ErrUnsupportedLookup)
}

func (unavailableContext) ArithmeticFunction(_ foundation.OperatorType, _ []types.Type) (functioninterfaces.FunctionHandle, error) {
	return nil, functions.ErrUnsupportedLookup
}

func (unavailableContext) ComparisonFunction(_ foundation.OperatorType, _, _ types.Type) (functioninterfaces.FunctionHandle, error) {
	return nil, functions.ErrUnsupportedLookup
}

func (unavailableContext) IsArithmeticFunction(_ functioninterfaces.FunctionHandle) (bool, error) {
	return false, functions.ErrIncompatibleHandle
}

func (unavailableContext) IsComparisonFunction(_ functioninterfaces.FunctionHandle) (bool, error) {
	return false, functions.ErrIncompatibleHandle
}

func (unavailableContext) Metadata(_ functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	return nil, functions.ErrIncompatibleHandle
}

func (unavailableContext) Evaluate(_ functioninterfaces.FunctionHandle, _ *functions.FunctionSignature, _ []valueinterfaces.Value) (valueinterfaces.Value, error) {
	return nil, functions.ErrIncompatibleHandle
}

func TestExternalFunctionContext(t *testing.T) {
	var provider functioninterfaces.FunctionContext = unavailableContext{}
	if handle, err := provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{&types.BoolType{}}); handle != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatal("lookup failure category was lost")
	}
	handle := externalHandle{domain: "fixture", definition: "constant"}
	if metadata, err := provider.Metadata(handle); metadata != nil || !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("incompatible handle category was lost")
	}
	signature, err := functions.NewFunctionSignature(nil, &types.BoolType{})
	if err != nil {
		t.Fatal(err)
	}
	if value, err := provider.Evaluate(handle, signature, nil); value != nil || !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("evaluation failure category was lost")
	}
}

func TestFunctionErrorCategories(t *testing.T) {
	categories := []error{
		functions.ErrUnknownHandle, functions.ErrIncompatibleHandle,
		functions.ErrUnsupportedLookup, functions.ErrEvaluationUnavailable,
		functions.ErrEvaluationDisallowed, functions.ErrEvaluationFailure,
		functions.ErrContractViolation,
	}
	for _, category := range categories {
		cause := errors.New("provider detail")
		wrapped := fmt.Errorf("operation: %w", errors.Join(category, cause))
		if !errors.Is(wrapped, cause) {
			t.Fatal("provider cause was lost")
		}
		for _, target := range categories {
			if errors.Is(wrapped, target) != (category == target) {
				t.Fatalf("category %v confused with %v", category, target)
			}
		}
	}
}
