package functions

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// Resolves declaration parameters from arguments only, producing a concrete implementation signature.
// Domain/arity mismatches return ErrUnsupportedLookup. Null policies are checked separately by
// constructing FunctionMetadata and calling BindFunctionSignature with the original argument Types.
func BindDeclarationSignature(declaration *DeclarationSignature, arguments []types.Type) (*FunctionSignature, error) {
	assertions.NotNilPointer(declaration)
	bindings, matched := types.BindTypePatterns(declaration.arguments, arguments)
	if !matched {
		return nil, fmt.Errorf("%w: declaration argument domains or arity do not match", ErrUnsupportedLookup)
	}
	parameters := make([]types.Type, len(declaration.arguments))
	for i, pattern := range declaration.arguments {
		typ, err := bindings.Resolve(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: declaration argument %d: %w", ErrContractViolation, i, err)
		}
		parameters[i] = typ
	}
	result, err := bindings.Resolve(declaration.result)
	if err != nil {
		return nil, fmt.Errorf("%w: declaration result: %w", ErrContractViolation, err)
	}
	return NewFunctionSignature(parameters, result)
}
