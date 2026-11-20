package functions

import (
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// FunctionSignature describes ordered argument types and a result type.
// Instances are immutable and share immutable Type descriptors. The zero value
// is invalid; use NewFunctionSignature. A signature can describe either an
// implementation or an effective call; binding determines their relationship.
type FunctionSignature struct {
	argumentTypes []types.Type
	resultType    types.Type
}

// NewFunctionSignature shares valid Types and copies argumentTypes. Nil Types assert.
// Nil and empty argument lists both describe a zero-argument function.
func NewFunctionSignature(argumentTypes []types.Type, resultType types.Type) (*FunctionSignature, error) {
	for _, t := range argumentTypes {
		assertions.NotNil(t)
	}
	assertions.NotNil(resultType)
	return &FunctionSignature{
		argumentTypes: append([]types.Type(nil), argumentTypes...),
		resultType:    resultType,
	}, nil
}

// ArgumentTypes returns a copy of the ordered argument-type slice.
// The receiver must be valid and non-nil; immutable Type descriptors are shared.
func (s *FunctionSignature) ArgumentTypes() []types.Type {
	assertions.NotNilPointer(s)
	return append([]types.Type(nil), s.argumentTypes...)
}

// ArgumentCount returns the number of ordered argument types without allocating.
// The receiver must be valid and non-nil.
func (s *FunctionSignature) ArgumentCount() int {
	assertions.NotNilPointer(s)
	return len(s.argumentTypes)
}

// ArgumentTypeAt returns the shared immutable Type at a zero-based index without
// allocating. The receiver must be valid and non-nil. Indexes outside
// [0, ArgumentCount()) panic. Ordering matches ArgumentTypes().
func (s *FunctionSignature) ArgumentTypeAt(index int) types.Type {
	assertions.NotNilPointer(s)
	return s.argumentTypes[index]
}

// ResultType returns the shared immutable result type.
// The receiver must be valid and non-nil.
func (s *FunctionSignature) ResultType() types.Type {
	assertions.NotNilPointer(s)
	return s.resultType
}
