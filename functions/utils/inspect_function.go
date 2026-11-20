// Package utils supplies function-boundary inspection and semantic recognition.
package utils

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// InspectFunction queries a handle's paired context and checks agreement between
// its implementation argument Types and the returned metadata. It returns the
// original immutable metadata; it neither binds actual arguments nor recognizes
// optimization semantics, resolves overloads or evaluates anything.
//
// The context must be valid and non-nil. Handles and metadata must otherwise obey
// their immutable object contracts. Missing handles/metadata and inconsistent
// argument Types return ErrContractViolation; provider errors retain their causes.
// In particular, a nil handle returned by an earlier lookup is diagnosed here.
func InspectFunction(context functioninterfaces.FunctionContext, handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	assertions.NotNil(context)
	if assertions.IsNil(handle) {
		return nil, fmt.Errorf("%w: missing function handle", functions.ErrContractViolation)
	}
	metadata, err := context.Metadata(handle)
	if err != nil {
		return nil, fmt.Errorf("function metadata: %w", err)
	}
	if metadata == nil {
		return nil, fmt.Errorf("%w: missing function metadata", functions.ErrContractViolation)
	}
	arguments := handle.ArgumentTypes()
	signature := metadata.Signature()
	if len(arguments) != signature.ArgumentCount() {
		return nil, fmt.Errorf("%w: handle/metadata arity mismatch", functions.ErrContractViolation)
	}
	for i, argument := range arguments {
		if assertions.IsNil(argument) || !argument.EqualsTo(signature.ArgumentTypeAt(i)) {
			return nil, fmt.Errorf("%w: handle/metadata argument %d mismatch", functions.ErrContractViolation, i)
		}
	}
	return metadata, nil
}
