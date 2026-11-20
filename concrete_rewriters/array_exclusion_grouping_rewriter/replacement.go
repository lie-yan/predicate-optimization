package array_exclusion_grouping_rewriter

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/array_membership"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// buildGroupReplacement requires an admitted compatible group with at least two
// members. It constructs a complete exclusion without publishing a changed region.
func buildGroupReplacement(factory valueinterfaces.ValueFactory, group exclusionGroup) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	anchor := group.members[0]
	count := 0
	for _, member := range group.members {
		count += len(member.candidates)
	}
	// Allocate the aggregate once; constructors own their copied containers while
	// all candidate Values and the anchor's array expression remain shared.
	candidates := make([]valueinterfaces.Value, 0, count)
	for _, member := range group.members {
		candidates = append(candidates, member.candidates...)
	}
	candidateType, err := types.NewArrayType(anchor.array.Type().(*types.ArrayType).Element(), foundation.NonNull)
	if err != nil {
		return absent, fmt.Errorf("group candidate type: %w", err)
	}
	candidateValue, err := factory.NewArray(candidateType, candidates)
	if err != nil {
		return absent, fmt.Errorf("group candidate array: %w", err)
	}
	literal, err := expressions.NewLiteralExpression(candidateValue)
	if err != nil {
		return absent, fmt.Errorf("group candidate literal: %w", err)
	}

	context := anchor.context
	handle, err := context.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{anchor.array.Type(), literal.Type()})
	if err != nil {
		if array_membership.IsUnsupportedTargetLookup(err) {
			return absent, nil
		}
		return absent, fmt.Errorf("group target lookup: %w", err)
	}
	metadata, err := context.Metadata(handle)
	if err != nil {
		return absent, fmt.Errorf("group target metadata: %w", err)
	}
	if !array_membership.RecognizeMetadata(metadata, functions.BuiltInArrayContainsAny) {
		return absent, nil
	}
	// Ordinary checked construction is the sole binding authority for the new
	// invocation. Incompatible returned overloads are errors, not lookup misses.
	call, err := expressions.NewCallExpression(context, handle, []expressions.Expression{anchor.array, literal})
	if err != nil {
		return absent, fmt.Errorf("group target construction: %w", err)
	}
	replacement, err := expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{call})
	if err != nil {
		return absent, fmt.Errorf("group negation construction: %w", err)
	}
	for _, member := range group.members {
		if !replacement.Type().EqualsTo(member.exclusion.Type()) {
			return absent, nil
		}
	}
	return foundation.Some[expressions.Expression](replacement), nil
}
