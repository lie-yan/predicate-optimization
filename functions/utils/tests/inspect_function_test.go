package utils_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/functions/utils"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// The embedded nil interface makes any operation except Metadata fail. Inspection
// must not perform lookup, classification or evaluation through the provider.
type inspectionContext struct {
	functioninterfaces.FunctionContext
	metadata *functions.FunctionMetadata
	failure  error
	queries  int
}

func (c *inspectionContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.queries++
	h, ok := handle.(inspectionHandle)
	if !ok || h.owner != c {
		return nil, functions.ErrIncompatibleHandle
	}
	if h.id != 1 {
		return nil, functions.ErrUnknownHandle
	}
	return c.metadata, c.failure
}

// A slice makes the value handle non-comparable. Full identity belongs to its
// owner; neither pointer identity nor a Go map key is required by inspection.
type inspectionHandle struct {
	owner     *inspectionContext
	id        int
	arguments []types.Type
}

func (h inspectionHandle) Namespace() string { return "inspection" }
func (h inspectionHandle) Name() string      { return "custom" }
func (h inspectionHandle) ArgumentTypes() []types.Type {
	return append([]types.Type(nil), h.arguments...)
}

func (h inspectionHandle) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	o, ok := other.(inspectionHandle)
	return ok && h.owner == o.owner && h.id == o.id
}

func inspectionMetadata(arguments []types.Type) *functions.FunctionMetadata {
	policies := make([]functions.NullPolicy, len(arguments))
	for i, argument := range arguments {
		if argument.Nullability() == foundation.MayNull {
			policies[i] = functions.PassNull
		}
	}
	// Deliberately unclassified, nullable and non-Boolean. Validity inspection
	// must not impose the narrower semantics of scalar or cleanup admission.
	result := must(types.NewVarCharType(nil, foundation.MayNull))
	return must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(arguments, result)), "custom function", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, policies))
}

func TestInspectFunctionAcceptsValidMetadata(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := must(integer.WithNullability(foundation.MayNull))
	nested := must(types.NewArrayType(nullable, foundation.NonNull))
	for _, tc := range []struct {
		name      string
		arguments []types.Type
	}{
		{"zero arguments", nil},
		{"scalar", []types.Type{integer}},
		{"nullable and composite", []types.Type{nullable, nested}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := inspectionMetadata(tc.arguments)
			context := &inspectionContext{metadata: metadata}
			handle := inspectionHandle{context, 1, tc.arguments}
			if reflect.TypeOf(handle).Comparable() {
				t.Fatal("fixture should use a non-comparable handle")
			}
			for range 2 {
				got, err := utils.InspectFunction(context, handle)
				if err != nil || got != metadata {
					t.Fatalf("inspection did not return shared metadata: %v", err)
				}
			}
			if context.queries != 2 {
				t.Fatal("inspection should query once per call without caching")
			}
			for i, argument := range tc.arguments {
				if argument != handle.arguments[i] || !argument.EqualsTo(metadata.Signature().ArgumentTypes()[i]) {
					t.Fatal("inspection mutated signature inputs")
				}
			}
		})
	}
}

func TestInspectFunctionChecksCompleteImplementationTypes(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	width := must(types.NewIntType(types.Int32, foundation.NonNull))
	nullable := must(integer.WithNullability(foundation.MayNull))
	bound8, bound16 := uint64(8), uint64(16)
	text8 := must(types.NewVarCharType(&bound8, foundation.NonNull))
	text16 := must(types.NewVarCharType(&bound16, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	nullableElements := must(types.NewArrayType(nullable, foundation.NonNull))
	var typedNil *types.IntType
	for _, tc := range []struct {
		name       string
		arguments  []types.Type
		parameters []types.Type
	}{
		{"arity", nil, []types.Type{integer}},
		{"nil argument", []types.Type{nil}, []types.Type{integer}},
		{"typed nil argument", []types.Type{typedNil}, []types.Type{integer}},
		{"integer width", []types.Type{width}, []types.Type{integer}},
		{"outer nullability", []types.Type{nullable}, []types.Type{integer}},
		{"varchar bound", []types.Type{text8}, []types.Type{text16}},
		{"nested nullability", []types.Type{array}, []types.Type{nullableElements}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context := &inspectionContext{metadata: inspectionMetadata(tc.parameters)}
			handle := inspectionHandle{context, 1, tc.arguments}
			got, err := utils.InspectFunction(context, handle)
			if got != nil || !errors.Is(err, functions.ErrContractViolation) || context.queries != 1 {
				t.Fatalf("expected checked inconsistency, got %v", err)
			}
		})
	}
}

func TestInspectFunctionRejectsMissingHandleOrMetadata(t *testing.T) {
	var typedNilHandle *inspectionHandle
	for _, handle := range []functioninterfaces.FunctionHandle{nil, typedNilHandle} {
		context := &inspectionContext{metadata: inspectionMetadata(nil)}
		got, err := utils.InspectFunction(context, handle)
		if got != nil || !errors.Is(err, functions.ErrContractViolation) || context.queries != 0 {
			t.Fatalf("missing handle should fail before querying provider: %v", err)
		}
	}
	// Metadata has a concrete pointer return type, so a typed nil pointer is
	// exactly nil here; it cannot be hidden inside a non-nil interface.
	var typedNilMetadata *functions.FunctionMetadata
	context := &inspectionContext{metadata: typedNilMetadata}
	got, err := utils.InspectFunction(context, inspectionHandle{owner: context, id: 1})
	if got != nil || !errors.Is(err, functions.ErrContractViolation) || context.queries != 1 {
		t.Fatalf("expected missing metadata failure: %v", err)
	}
}

func TestInspectFunctionPreservesProviderFailuresAndRetry(t *testing.T) {
	sentinel := errors.New("metadata provider failed")
	for _, cause := range []error{
		sentinel, functions.ErrIncompatibleHandle, functions.ErrUnknownHandle,
		fmt.Errorf("%w: %w", functions.ErrUnsupportedLookup, sentinel),
		errors.Join(functions.ErrUnsupportedLookup, functions.ErrContractViolation),
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			// Even a provider returning metadata alongside an error has failed.
			context := &inspectionContext{metadata: inspectionMetadata(nil), failure: cause}
			handle := inspectionHandle{owner: context, id: 1}
			got, err := utils.InspectFunction(context, handle)
			if got != nil || !errors.Is(err, cause) {
				t.Fatalf("provider cause lost: %v", err)
			}
			for _, category := range []error{functions.ErrUnsupportedLookup, functions.ErrContractViolation, sentinel} {
				if errors.Is(cause, category) && !errors.Is(err, category) {
					t.Fatalf("joined cause lost: %v", category)
				}
			}
			context.failure = nil
			if retry := must(utils.InspectFunction(context, handle)); retry != context.metadata || context.queries != 2 {
				t.Fatal("clean retry failed")
			}
		})
	}
}

func TestInspectFunctionUsesPairedContext(t *testing.T) {
	first, second := &inspectionContext{metadata: inspectionMetadata(nil)}, &inspectionContext{metadata: inspectionMetadata(nil)}
	for _, tc := range []struct {
		handle inspectionHandle
		cause  error
	}{
		{inspectionHandle{owner: first, id: 1}, functions.ErrIncompatibleHandle},
		{inspectionHandle{owner: second, id: 2}, functions.ErrUnknownHandle},
	} {
		got, err := utils.InspectFunction(second, tc.handle)
		if got != nil || !errors.Is(err, tc.cause) {
			t.Fatalf("paired-context failure lost: %v", err)
		}
	}
	provider := must(testbed.NewStandardContext())
	integer := must(types.NewIntType(types.Int64, foundation.MayNull))
	handle := must(provider.ComparisonFunction(foundation.Equal, integer, integer))
	metadata := must(utils.InspectFunction(provider, handle))
	if metadata != must(provider.Metadata(handle)) || metadata.Signature().ArgumentTypes()[0].Nullability() != foundation.NonNull {
		t.Fatal("inspection should compare implementation Types, not nullable call Types")
	}
	foreign := must(testbed.NewStandardContext())
	if got, err := utils.InspectFunction(foreign, handle); got != nil || !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatalf("foreign catalog accepted: %v", err)
	}
}
