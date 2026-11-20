package array_membership_normalization_rewriter

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/array_membership"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

// normalizeSingleton returns a local replacement only after both invocation
// contracts and the effective result Type agree. It never evaluates arguments.
func normalizeSingleton(call *expressions.CallExpression) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	arguments := call.Arguments()
	element, present := singletonCandidate(arguments).Get()
	if !present {
		return absent, nil
	}
	context := call.Context()
	source, err := context.Metadata(call.Handle())
	if err != nil {
		return absent, fmt.Errorf("normalization source metadata: %w", err)
	}
	if !array_membership.RecognizeMetadata(source, functions.BuiltInArrayContainsAny) {
		return absent, nil
	}

	// Unsupported optimization is benign only at explicit target lookup.
	// Metadata and final construction errors remain provider/contract failures.
	argumentTypes := []types.Type{arguments[0].Type(), element.Type()}
	handle, err := context.LookupBuiltInFunction(functions.BuiltInArrayContains, argumentTypes)
	if err != nil {
		if array_membership.IsUnsupportedTargetLookup(err) {
			return absent, nil
		}
		return absent, fmt.Errorf("normalization target lookup: %w", err)
	}
	target, err := context.Metadata(handle)
	if err != nil {
		return absent, fmt.Errorf("normalization target metadata: %w", err)
	}
	if !array_membership.RecognizeMetadata(target, functions.BuiltInArrayContains) {
		return absent, nil
	}

	// Share the input expression and sole immutable element Value. Construction
	// is the sole binding authority, using the ordinary API and its metadata query.
	literal, err := expressions.NewLiteralExpression(element)
	if err != nil {
		return absent, err
	}
	replacement, err := expressions.NewCallExpression(context, handle, []expressions.Expression{arguments[0], literal})
	if err != nil {
		return absent, fmt.Errorf("normalization target construction: %w", err)
	}
	if !replacement.Type().EqualsTo(call.Type()) {
		return absent, nil
	}
	return foundation.Some[expressions.Expression](replacement), nil
}
