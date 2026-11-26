package functions

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// Immutable function declaration with scoped literal parameters and no executable unresolved Types.
// Use the constructor; the zero value is invalid. Binding to concrete Types is a separate operation.
type DeclarationSignature struct {
	// Ordered immutable argument patterns, privately owned as a slice.
	arguments []types.TypePattern
	// Immutable result pattern whose parameters must occur in arguments.
	result types.TypePattern
}

// Copies arguments and rejects result parameters that cannot be inferred from arguments.
// Nil patterns assert; fixed zero-argument declarations are valid.
func NewDeclarationSignature(arguments []types.TypePattern, result types.TypePattern) (*DeclarationSignature, error) {
	assertions.NotNil(result)
	parameters := make(map[string]bool)
	for _, arg := range arguments {
		assertions.NotNil(arg)
		for _, name := range arg.Parameters() {
			parameters[name] = true
		}
	}
	for _, name := range result.Parameters() {
		if !parameters[name] {
			return nil, fmt.Errorf("result parameter %q does not occur in an argument", name)
		}
	}
	return &DeclarationSignature{arguments: append([]types.TypePattern(nil), arguments...), result: result}, nil
}

// Adapts a concrete signature without copying or changing any Type descriptor.
func DeclarationFromSignature(signature *FunctionSignature) *DeclarationSignature {
	assertions.NotNilPointer(signature)
	arguments := make([]types.TypePattern, len(signature.argumentTypes))
	for i, typ := range signature.argumentTypes {
		arguments[i] = types.NewConcreteTypePattern(typ)
	}
	return &DeclarationSignature{arguments: arguments, result: types.NewConcreteTypePattern(signature.resultType)}
}

// Returns an independent argument slice sharing immutable patterns.
func (s *DeclarationSignature) Arguments() []types.TypePattern {
	assertions.NotNilPointer(s)
	return append([]types.TypePattern(nil), s.arguments...)
}

// Returns the fixed argument arity without allocation.
func (s *DeclarationSignature) ArgumentCount() int {
	assertions.NotNilPointer(s)
	return len(s.arguments)
}

// Returns an immutable argument pattern; out-of-range indexes panic.
func (s *DeclarationSignature) ArgumentAt(index int) types.TypePattern {
	assertions.NotNilPointer(s)
	return s.arguments[index]
}

// Returns the shared immutable result pattern.
func (s *DeclarationSignature) Result() types.TypePattern {
	assertions.NotNilPointer(s)
	return s.result
}

// Compares declarations modulo consistent parameter renaming across arguments and result.
func (s *DeclarationSignature) EqualsTo(other *DeclarationSignature) bool {
	assertions.NotNilPointer(s)
	assertions.NotNilPointer(other)
	if s == other {
		return true
	}
	if len(s.arguments) != len(other.arguments) {
		return false
	}
	left := append(s.Arguments(), s.result)
	right := append(other.Arguments(), other.result)
	return types.EqualTypePatterns(left, right)
}
