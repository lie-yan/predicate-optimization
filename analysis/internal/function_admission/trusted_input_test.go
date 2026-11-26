package function_admission

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

type observedHandle struct {
	functioninterfaces.FunctionHandle
	argumentQueries int
}

// Each wrapper owns a stable identity accepted by its paired fixture context.
func (h *observedHandle) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	o, ok := other.(*observedHandle)
	return ok && h == o
}

func (h *observedHandle) Signature() *functions.FunctionSignature {
	h.argumentQueries++
	return h.FunctionHandle.Signature()
}

func TestStoredComparisonTrustsValidatedBinding(t *testing.T) {
	base := admissionMust(testbed.NewStandardContext())
	integer := admissionMust(types.NewIntType(types.Int64, foundation.MayNull))
	underlying := admissionMust(base.ComparisonFunction(foundation.Equal, integer, integer))
	handle := &observedHandle{FunctionHandle: underlying}
	queries := 0
	context := &admissionContext{FunctionContext: base}
	context.metadata = func(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
		queries++
		if h != handle {
			return nil, functions.ErrIncompatibleHandle
		}
		return base.Metadata(underlying)
	}
	call := admissionMust(expressions.NewCallExpression(context, handle, []expressions.Expression{admissionReference(integer), admissionReference(integer)}))
	if err := utils.ValidateForOptimization(call); err != nil {
		t.Fatal(err)
	}
	handle.argumentQueries, queries = 0, 0
	context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		t.Fatal("stored comparison should not be resolved again")
		return nil, nil
	}
	operator, err := CheckComparison(call)
	if err != nil || operator != foundation.Equal || handle.argumentQueries != 0 || queries != 1 {
		t.Fatalf("stored call repeated validity checks or lost semantics: %v", err)
	}
	if call.Type().Nullability() != foundation.MayNull {
		t.Fatal("effective Type changed")
	}
}

func TestResolveComparisonConstructionFailureAndRetry(t *testing.T) {
	base := admissionMust(testbed.NewStandardContext())
	integer := admissionMust(types.NewIntType(types.Int64, foundation.MayNull))
	left, right := admissionReference(integer), admissionReference(integer)
	for _, cause := range []error{errors.New("metadata failure"), functions.ErrUnsupportedLookup, errors.Join(functions.ErrUnsupportedLookup, functions.ErrContractViolation)} {
		context := &admissionContext{FunctionContext: base}
		queries := 0
		context.metadata = func(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
			queries++
			if queries == 2 {
				return nil, cause // Checked construction's required metadata query.
			}
			return base.Metadata(handle)
		}
		candidate, err := ResolveComparison(context, foundation.Equal, left, right)
		if candidate != nil || !errors.Is(err, cause) || queries != 2 {
			t.Fatalf("construction failure was hidden or published a candidate: %v", err)
		}
		context.metadata = nil
		retry := admissionMust(ResolveComparison(context, foundation.Equal, left, right))
		if retry.Context() != context || retry.Type().Nullability() != foundation.MayNull || retry.Children()[0] != left || retry.Children()[1] != right {
			t.Fatal("clean retry lost context, effective Type or child identity")
		}
	}
}

func TestResolveComparisonRetainsConstructorBindingError(t *testing.T) {
	base := admissionMust(testbed.NewStandardContext())
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	floating := admissionMust(types.NewFloatType(types.Float64, foundation.NonNull))
	context := &admissionContext{FunctionContext: base}
	context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		// Valid handle/metadata, but a deliberately incorrect lookup result for
		// the actual argument Types. The constructor owns binding diagnosis.
		return base.ComparisonFunction(foundation.Equal, floating, floating)
	}
	candidate, err := ResolveComparison(context, foundation.Equal, admissionReference(integer), admissionReference(integer))
	if candidate != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatalf("constructor binding failure lost: %v", err)
	}
}
