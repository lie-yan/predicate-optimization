package functions_test

import (
	"errors"
	"testing"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestContextInspection(t *testing.T) {
	base := definition(t, types.Int64, foundation.NonNull, shared.PropagateNull)
	defs := []testbed.Definition{base}
	for _, operator := range []foundation.OperatorType{foundation.Negate, foundation.Equal} {
		signature := base.Metadata.Signature()
		policies := base.Metadata.NullPolicies()
		name := "negate"
		if operator == foundation.Equal {
			name = "equal"
			boolean, err := types.NewBoolType(foundation.NonNull)
			if err != nil {
				t.Fatal(err)
			}
			argument := signature.ArgumentTypes()[0]
			signature, err = shared.NewFunctionSignature([]types.Type{argument, argument}, boolean)
			if err != nil {
				t.Fatal(err)
			}
			policies = []shared.NullPolicy{shared.PropagateNull, shared.PropagateNull}
		}
		metadata, err := shared.NewFunctionMetadata(signature, name, foundation.Optional[shared.BuiltInFunctionName]{}, foundation.Some(operator), policies)
		if err != nil {
			t.Fatal(err)
		}
		defs = append(defs, testbed.Definition{Namespace: "fixture", Name: name, Metadata: metadata})
	}
	catalog, err := testbed.NewCatalog(defs)
	if err != nil {
		t.Fatal(err)
	}
	provider := testbed.NewContext(catalog)
	for i, handle := range catalog.Handles() {
		metadata, err := provider.Metadata(handle)
		if err != nil || metadata != defs[i].Metadata {
			t.Fatalf("metadata not shared: %v", err)
		}
		arithmetic, err := provider.IsArithmeticFunction(handle)
		if err != nil || arithmetic != (i == 1) {
			t.Fatalf("incorrect arithmetic recognition: %v", err)
		}
		comparison, err := provider.IsComparisonFunction(handle)
		if err != nil || comparison != (i == 2) {
			t.Fatalf("incorrect comparison recognition: %v", err)
		}
		if _, err := testbed.NewContext(catalog).Metadata(handle); err != nil {
			t.Fatal("contexts sharing a catalog should accept the same handles")
		}
	}
}

type foreignHandle struct{}

func (foreignHandle) Namespace() string                       { return "fixture" }
func (foreignHandle) Name() string                            { return "identity" }
func (foreignHandle) Signature() *shared.FunctionSignature { return nil }
func (foreignHandle) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	_, ok := other.(foreignHandle)
	return ok
}

func TestContextRejectsForeignHandles(t *testing.T) {
	def := definition(t, types.Int64, foundation.NonNull, shared.PropagateNull)
	catalog, err := testbed.NewCatalog([]testbed.Definition{def})
	if err != nil {
		t.Fatal(err)
	}
	other, err := testbed.NewCatalog([]testbed.Definition{def})
	if err != nil {
		t.Fatal(err)
	}
	provider := testbed.NewContext(catalog)
	input := contextArguments(t, def)
	for _, handle := range []functioninterfaces.FunctionHandle{other.Handles()[0], foreignHandle{}} {
		if _, err := provider.Metadata(handle); !errors.Is(err, shared.ErrIncompatibleHandle) {
			t.Fatalf("foreign metadata lookup: %v", err)
		}
		if _, err := provider.IsArithmeticFunction(handle); !errors.Is(err, shared.ErrIncompatibleHandle) {
			t.Fatalf("foreign arithmetic recognition: %v", err)
		}
		if _, err := provider.IsComparisonFunction(handle); !errors.Is(err, shared.ErrIncompatibleHandle) {
			t.Fatalf("foreign comparison recognition: %v", err)
		}
		if value, err := provider.Evaluate(handle, def.Metadata.Signature(), input); value != nil || !errors.Is(err, shared.ErrIncompatibleHandle) {
			t.Fatalf("foreign evaluation: %v", err)
		}
	}
}

func TestContextUnavailableOperations(t *testing.T) {
	def := definition(t, types.Int64, foundation.NonNull, shared.PropagateNull)
	catalog, err := testbed.NewCatalog([]testbed.Definition{def})
	if err != nil {
		t.Fatal(err)
	}
	provider := testbed.NewContext(catalog)
	args := def.Metadata.Signature().ArgumentTypes()
	if _, err := provider.LookupBuiltInFunction(shared.BuiltInNot, args); !errors.Is(err, shared.ErrUnsupportedLookup) {
		t.Fatal(err)
	}
	if _, err := provider.ArithmeticFunction(foundation.Negate, args); !errors.Is(err, shared.ErrUnsupportedLookup) {
		t.Fatal(err)
	}
	if _, err := provider.ComparisonFunction(foundation.Equal, args[0], args[0]); !errors.Is(err, shared.ErrUnsupportedLookup) {
		t.Fatal(err)
	}
	handle := catalog.Handles()[0]
	input := contextArguments(t, def)
	if result, err := provider.Evaluate(handle, def.Metadata.Signature(), input); result != nil || !errors.Is(err, shared.ErrEvaluationUnavailable) {
		t.Fatalf("expected unavailable: %v", err)
	}
}

func contextArguments(t *testing.T, def testbed.Definition) []valueinterfaces.Value {
	t.Helper()
	argument := def.Metadata.Signature().ArgumentTypes()[0].(*types.IntType)
	value, err := testbedvalues.NewInteger(argument, 1)
	if err != nil {
		t.Fatal(err)
	}
	return []valueinterfaces.Value{value}
}
