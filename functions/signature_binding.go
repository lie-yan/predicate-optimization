package functions

import (
	opterrors "github.com/lie-yan/predicate-optimization/errors"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// BindFunctionSignature derives an effective call signature from valid immutable
// metadata and argument-expression types. Only outer nullability may adapt;
// coercion, overload resolution and changes to nested constraints are excluded.
// Inputs must be valid and non-nil; nil inputs trigger assertions.
// Incompatible arity/types and nullable arguments with unspecified null handling
// return ErrUnsupportedLookup. No independent null-handling contract is assumed.
// Exact bindings share the immutable implementation signature; adapted bindings
// retain a private copy of the effective argument slice.
func BindFunctionSignature(metadata *FunctionMetadata, argumentTypes []types.Type) (*FunctionSignature, error) {
	resultType, err := bindResultType(metadata, argumentTypes)
	if err != nil {
		return nil, err
	}
	implementation := metadata.signature
	if resultType.EqualsTo(implementation.resultType) {
		exact := true
		for i, argument := range argumentTypes {
			if !argument.EqualsTo(implementation.argumentTypes[i]) {
				exact = false
				break
			}
		}
		if exact {
			return implementation, nil
		}
	}
	return NewFunctionSignature(argumentTypes, resultType)
}

// CheckFunctionBinding performs the same checks and result-nullability adaptation
// as BindFunctionSignature without constructing an effective signature or copying
// its argument slice. It preserves binding errors and input assertions. Successful
// checks can still allocate when adapting the result type's nullability.
// Use this when only bindability matters, such as overload candidate selection.
func CheckFunctionBinding(metadata *FunctionMetadata, argumentTypes []types.Type) error {
	_, err := bindResultType(metadata, argumentTypes)
	return err
}

// bindResultType checks the binding without constructing temporary argument
// descriptors or an effective signature. Only the result's nullability may adapt.
func bindResultType(metadata *FunctionMetadata, argumentTypes []types.Type) (types.Type, error) {
	result, check := probeResultType(metadata, argumentTypes)
	return result, check.Err()
}

func probeResultType(metadata *FunctionMetadata, argumentTypes []types.Type) (types.Type, BindingCheck) {
	assertions.NotNilPointer(metadata)
	for _, argument := range argumentTypes {
		assertions.NotNil(argument)
	}
	implementation := metadata.signature
	if len(argumentTypes) != len(implementation.argumentTypes) {
		return nil, BindingCheck{code: opterrors.CodeBindingArityMismatch, first: len(argumentTypes), second: len(implementation.argumentTypes)}
	}
	resultType := implementation.resultType
	for i, argument := range argumentTypes {
		parameter := implementation.argumentTypes[i]
		if !argument.EqualsToIgnoringNullability(parameter) {
			return nil, BindingCheck{code: opterrors.CodeBindingArgumentTypeMismatch, first: i}
		}
		if argument.Nullability() != foundation.MayNull {
			continue
		}
		switch metadata.nullPolicies[i] {
		case NullPolicyUnspecified:
			return nil, BindingCheck{code: opterrors.CodeBindingNullPolicyUnspecified, first: i}
		case PropagateNull:
			if resultType.Nullability() != foundation.MayNull {
				adapted, err := resultType.WithNullability(foundation.MayNull)
				if err != nil {
					return nil, BindingCheck{code: opterrors.CodeBindingResultNullability, cause: err}
				}
				resultType = adapted
			}
		case PassNull:
			// Metadata construction guarantees that the parameter admits null.
		}
	}
	return resultType, BindingCheck{}
}

// ValidateCallSignature checks that a valid supplied signature is exactly the
// signature derived from its argument types and metadata. A mismatch returns
// ErrContractViolation. Unsupported binding is retained as a wrapped cause.
// The caller separately checks concrete Values against these argument types.
// Nil metadata, signatures and argument types trigger assertions.
func ValidateCallSignature(metadata *FunctionMetadata, callSignature *FunctionSignature) error {
	assertions.NotNilPointer(metadata)
	if callSignature == nil {
		panic("functions: expected non-nil FunctionSignature")
	}
	assertions.NotNil(callSignature.resultType)
	expected, err := bindResultType(metadata, callSignature.argumentTypes)
	if err != nil {
		return newBindingError(opterrors.CodeInvalidCallBinding, err)
	}
	if !expected.EqualsTo(callSignature.resultType) {
		return newBindingError(opterrors.CodeBindingResultTypeMismatch, nil)
	}
	return nil
}
