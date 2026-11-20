package functions_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func evalFixture(t *testing.T, policies []shared.NullPolicy) (*testbed.Catalog, *testbed.Handle, *shared.FunctionMetadata) {
	t.Helper()
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	params := make([]types.Type, len(policies))
	for i, policy := range policies {
		params[i] = integer
		if policy == shared.PassNull {
			params[i], err = integer.WithNullability(foundation.MayNull)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	def := lookupDefinition(t, "evaluate", params, integer, foundation.Optional[shared.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, policies)
	catalog, err := testbed.NewCatalog([]testbed.Definition{def})
	if err != nil {
		t.Fatal(err)
	}
	return catalog, catalog.Handles()[0], def.Metadata
}

func evalInteger(t *testing.T, typ types.Type, number int64) valueinterfaces.Value {
	t.Helper()
	value, err := testbedvalues.NewInteger(typ.(*types.IntType), number)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func evalSignature(t *testing.T, metadata *shared.FunctionMetadata, args ...valueinterfaces.Value) *shared.FunctionSignature {
	t.Helper()
	types := make([]types.Type, len(args))
	for i, value := range args {
		types[i] = value.Type()
	}
	signature, err := shared.BindFunctionSignature(metadata, types)
	if err != nil {
		t.Fatal(err)
	}
	return signature
}

func TestEvaluateNullPropagationAndPolicy(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.PropagateNull})
	nullable, err := metadata.Signature().ArgumentTypes()[0].WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	null, err := testbedvalues.NewNull(nullable)
	if err != nil {
		t.Fatal(err)
	}
	signature := evalSignature(t, metadata, null)
	calls := 0
	provider, err := testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) {
		calls++
		return nil, errors.New("must not execute")
	}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []*testbed.Context{provider, testbed.NewContext(catalog)} {
		result, err := ctx.Evaluate(handle, signature, []valueinterfaces.Value{null})
		if err != nil || !result.IsNull() || !result.Type().EqualsTo(signature.ResultType()) {
			t.Fatalf("null propagation failed: %v", err)
		}
	}
	if calls != 0 {
		t.Fatal("null propagation invoked evaluator")
	}
	disallowed, err := testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Disallowed: true}})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := disallowed.Evaluate(handle, signature, []valueinterfaces.Value{null}); result != nil || !errors.Is(err, shared.ErrEvaluationDisallowed) {
		t.Fatal("policy bypassed by null propagation")
	}
}

func TestEvaluateAdaptationAndPassingNull(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.PropagateNull, shared.PassNull})
	params := metadata.Signature().ArgumentTypes()
	nullable, err := params[0].WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	first := evalInteger(t, nullable, 8)
	second, err := testbedvalues.NewNull(params[1])
	if err != nil {
		t.Fatal(err)
	}
	args := []valueinterfaces.Value{first, second}
	calls := 0
	bindings := []testbed.EvaluationBinding{{Handle: handle, Evaluate: func(got []valueinterfaces.Value) (valueinterfaces.Value, error) {
		calls++
		if !got[0].Type().EqualsTo(params[0]) || !got[1].IsNull() || !got[1].Type().EqualsTo(params[1]) {
			t.Fatal("incorrect implementation arguments")
		}
		result := got[0]
		got[0] = nil // Checked invocation isolates its caller's argument slice.
		return result, nil
	}}}
	provider, err := testbed.NewContextWithEvaluators(catalog, bindings)
	if err != nil {
		t.Fatal(err)
	}
	bindings[0].Evaluate = nil
	signature := evalSignature(t, metadata, args...)
	result, err := provider.Evaluate(handle, signature, args)
	if err != nil || result.(*testbedvalues.IntegerValue).Integer() != 8 || !result.Type().EqualsTo(signature.ResultType()) {
		t.Fatalf("result adaptation failed: %v", err)
	}
	if calls != 1 || args[0] != first || first.Type().Nullability() != foundation.MayNull {
		t.Fatal("caller state mutated")
	}
}

func TestEvaluateRejectsInvalidCallsAndResults(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.PropagateNull})
	valid := evalInteger(t, metadata.Signature().ArgumentTypes()[0], 1)
	calls := 0
	provider, err := testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) { calls++; return valid, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	wrongType, err := types.NewIntType(types.Int8, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	wrong := evalInteger(t, wrongType, 1)
	for _, args := range [][]valueinterfaces.Value{nil, {wrong}} {
		if result, err := provider.Evaluate(handle, metadata.Signature(), args); result != nil || !errors.Is(err, shared.ErrContractViolation) {
			t.Fatalf("invalid call accepted: %v", err)
		}
	}
	forged, err := shared.NewFunctionSignature(metadata.Signature().ArgumentTypes(), wrongType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Evaluate(handle, forged, []valueinterfaces.Value{valid}); !errors.Is(err, shared.ErrContractViolation) {
		t.Fatal("forged result accepted")
	}
	if calls != 0 {
		t.Fatal("invalid call reached evaluator")
	}
	for _, result := range []valueinterfaces.Value{wrong} {
		bad, err := testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) { return result, nil }}})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := bad.Evaluate(handle, metadata.Signature(), []valueinterfaces.Value{valid}); got != nil || !errors.Is(err, shared.ErrContractViolation) {
			t.Fatalf("invalid result accepted: %v", err)
		}
	}
}

func TestEvaluateErrors(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.PropagateNull})
	argument := evalInteger(t, metadata.Signature().ArgumentTypes()[0], 1)
	cause := errors.New("division by zero fixture")
	for _, failure := range []error{cause, shared.ErrEvaluationDisallowed} {
		provider, err := testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) { return nil, failure }}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := provider.Evaluate(handle, metadata.Signature(), []valueinterfaces.Value{argument})
		if result != nil || !errors.Is(err, failure) {
			t.Fatalf("error cause lost: %v", err)
		}
		if errors.Is(err, shared.ErrEvaluationFailure) != (failure == cause) {
			t.Fatal("incorrect semantic failure classification")
		}
	}
}

func TestEvaluateUnspecifiedNullableContract(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.NullPolicyUnspecified})
	nullable, err := metadata.Signature().ArgumentTypes()[0].WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	null, err := testbedvalues.NewNull(nullable)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := shared.NewFunctionSignature([]types.Type{nullable}, nullable)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := testbed.NewContext(catalog).Evaluate(handle, signature, []valueinterfaces.Value{null}); result != nil || !errors.Is(err, shared.ErrContractViolation) {
		t.Fatal("unestablished nullable call contract accepted")
	}
}

func TestEvaluationBindingsRejectDuplicatesAndForeignHandles(t *testing.T) {
	catalog, handle, _ := evalFixture(t, []shared.NullPolicy{shared.PropagateNull})
	_, foreign, _ := evalFixture(t, []shared.NullPolicy{shared.PropagateNull})
	for _, tc := range []struct {
		bindings []testbed.EvaluationBinding
		want     error
	}{
		{[]testbed.EvaluationBinding{{Handle: handle}, {Handle: catalog.Handles()[0]}}, shared.ErrContractViolation},
		{[]testbed.EvaluationBinding{{Handle: foreign}}, shared.ErrIncompatibleHandle},
	} {
		if provider, err := testbed.NewContextWithEvaluators(catalog, tc.bindings); provider != nil || !errors.Is(err, tc.want) {
			t.Fatalf("invalid binding accepted: %v", err)
		}
	}
}

func TestNullPropagationValidatesAllArguments(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.PropagateNull, shared.PropagateNull})
	nullable, err := metadata.Signature().ArgumentTypes()[0].WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	null, err := testbedvalues.NewNull(nullable)
	if err != nil {
		t.Fatal(err)
	}
	valid := evalInteger(t, metadata.Signature().ArgumentTypes()[1], 2)
	signature := evalSignature(t, metadata, null, valid)
	if result, err := testbed.NewContext(catalog).Evaluate(handle, signature, []valueinterfaces.Value{null, null}); result != nil || !errors.Is(err, shared.ErrContractViolation) {
		t.Fatal("null propagation hid an invalid argument")
	}
}

func TestEvaluateNilResultAssertions(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.PropagateNull})
	valid := evalInteger(t, metadata.Signature().ArgumentTypes()[0], 1)
	for _, result := range []valueinterfaces.Value{nil, (*testbedvalues.IntegerValue)(nil)} {
		provider, err := testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) { return result, nil }}})
		if err != nil {
			t.Fatal(err)
		}
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("expected non-nil result assertion")
				}
			}()
			_, _ = provider.Evaluate(handle, metadata.Signature(), []valueinterfaces.Value{valid})
		}()
	}
}
