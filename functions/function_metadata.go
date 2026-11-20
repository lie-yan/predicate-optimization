package functions

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
)

// FunctionMetadata describes a resolved function independently of execution.
// Instances are immutable; the zero value is invalid. Use NewFunctionMetadata.
type FunctionMetadata struct {
	signature    *FunctionSignature
	displayName  string
	builtInName  foundation.Optional[BuiltInFunctionName]
	operator     foundation.Optional[foundation.OperatorType]
	nullPolicies []NullPolicy
}

// NewFunctionMetadata shares signature and copies nullPolicies. Zero-value
// optionals mean absent classification; present empty names or zero operators
// are invalid. Built-in names and operator identities may describe the same
// function. The provider ensures their consistency with the signature and behavior.
// Every parameter requires an explicit policy, which may be unspecified.
func NewFunctionMetadata(
	signature *FunctionSignature,
	displayName string,
	builtInName foundation.Optional[BuiltInFunctionName],
	operator foundation.Optional[foundation.OperatorType],
	nullPolicies []NullPolicy,
) (*FunctionMetadata, error) {
	if signature == nil || signature.resultType == nil {
		return nil, fmt.Errorf("invalid function signature")
	}
	assertions.NotNil(signature.resultType)
	for _, t := range signature.argumentTypes {
		assertions.NotNil(t)
	}
	name, hasName := builtInName.Get()
	if hasName && name == "" {
		return nil, fmt.Errorf("empty built-in name")
	}
	op, hasOperator := operator.Get()
	if hasOperator && !op.IsArithmeticOperator() && !op.IsComparisonOperator() {
		return nil, fmt.Errorf("invalid operator: %d", op)
	}
	if len(nullPolicies) != len(signature.argumentTypes) {
		return nil, fmt.Errorf("null policy count %d does not match arity %d", len(nullPolicies), len(signature.argumentTypes))
	}
	for i, policy := range nullPolicies {
		if !policy.IsValid() {
			return nil, fmt.Errorf("invalid null policy at position %d", i)
		}
		if policy == PassNull && signature.argumentTypes[i].Nullability() != foundation.MayNull {
			return nil, fmt.Errorf("PassNull requires a nullable parameter at position %d", i)
		}
	}
	return &FunctionMetadata{
		signature: signature, displayName: displayName,
		builtInName: builtInName, operator: operator,
		nullPolicies: append([]NullPolicy(nil), nullPolicies...),
	}, nil
}

// Signature returns the shared immutable implementation signature.
func (m *FunctionMetadata) Signature() *FunctionSignature {
	assertions.NotNilPointer(m)
	return m.signature
}

// DisplayName returns diagnostic text, which may be empty and is not identity.
func (m *FunctionMetadata) DisplayName() string {
	assertions.NotNilPointer(m)
	return m.displayName
}

// BuiltInName returns the standard built-in identity, if established.
func (m *FunctionMetadata) BuiltInName() foundation.Optional[BuiltInFunctionName] {
	assertions.NotNilPointer(m)
	return m.builtInName
}

// Operator returns the scalar operator identity, if established.
func (m *FunctionMetadata) Operator() foundation.Optional[foundation.OperatorType] {
	assertions.NotNilPointer(m)
	return m.operator
}

// NullPolicies returns a copy in implementation-parameter order.
func (m *FunctionMetadata) NullPolicies() []NullPolicy {
	assertions.NotNilPointer(m)
	return append([]NullPolicy(nil), m.nullPolicies...)
}

// NullPolicyAt returns the policy at a zero-based implementation-parameter index
// without allocating. The receiver must be valid and non-nil. Indexes outside
// [0, Signature().ArgumentCount()) panic. Ordering matches NullPolicies().
func (m *FunctionMetadata) NullPolicyAt(index int) NullPolicy {
	assertions.NotNilPointer(m)
	return m.nullPolicies[index]
}
