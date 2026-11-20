package array_exclusion_grouping_rewriter

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/array_membership"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// exclusionOccurrence describes one eligible direct AND operand; compatibility
// between occurrences and safe publication are separate later steps.
type exclusionOccurrence struct {
	// Position in the rebuilt AND's direct child list.
	index int
	// Original NOT wrapper for this occurrence.
	exclusion *expressions.LogicalExpression
	// Recognized membership invocation beneath NOT.
	call *expressions.CallExpression
	// Reference or literal array retained by a future group.
	array expressions.Expression
	// Provider paired with this call's handle.
	context functioninterfaces.FunctionContext
	// Owned ordered slice sharing immutable candidate Values.
	candidates []valueinterfaces.Value
}

// collectExclusions inspects direct AND children only, preserving occurrence order.
// It neither descends into nested groups nor publishes partial analysis on error.
func collectExclusions(group *expressions.LogicalExpression) ([]exclusionOccurrence, error) {
	if group.Operator() != foundation.And {
		return nil, nil
	}
	var occurrences []exclusionOccurrence
	for index, child := range group.Children() {
		occurrence, err := recognizeExclusion(index, child)
		if err != nil {
			return nil, fmt.Errorf("array exclusion operand %d: %w", index, err)
		}
		if value, present := occurrence.Get(); present {
			occurrences = append(occurrences, value)
		}
	}
	return occurrences, nil
}

// recognizeExclusion trusts valid stored calls. Literal/leaf shape misses avoid
// provider queries; recognized shapes inspect semantic metadata exactly once.
func recognizeExclusion(index int, node expressions.Expression) (foundation.Optional[exclusionOccurrence], error) {
	absent := foundation.Optional[exclusionOccurrence]{}
	exclusion, ok := node.(*expressions.LogicalExpression)
	if !ok || exclusion.Operator() != foundation.Not {
		return absent, nil
	}
	call, ok := exclusion.Children()[0].(*expressions.CallExpression)
	if !ok {
		return absent, nil
	}
	args := call.Arguments()
	if len(args) != 2 {
		return absent, nil
	}
	switch args[0].(type) {
	case *expressions.ReferenceExpression, *expressions.LiteralExpression:
		// Stable reference lookup or immutable literal execution is the caller's
		// grouping contract. Shared literal identity is checked between members later.
	default:
		return absent, nil
	}
	arrayType, ok := args[0].Type().(*types.ArrayType)
	if !ok || arrayType.Element().Nullability() != foundation.NonNull {
		return absent, nil
	}
	literal, ok := args[1].(*expressions.LiteralExpression)
	if !ok || literal.Type().Nullability() != foundation.NonNull || literal.Value().IsNull() {
		return absent, nil
	}

	var candidates []valueinterfaces.Value
	name := functions.BuiltInArrayContains
	if literal.Type().EqualsTo(arrayType.Element()) {
		// E may itself be an array or struct: a literal of E is one candidate,
		// even when its payload happens to be an empty composite collection.
		candidates = []valueinterfaces.Value{literal.Value()}
	} else {
		candidateType, ok := literal.Type().(*types.ArrayType)
		if !ok || !candidateType.Element().EqualsTo(arrayType.Element()) {
			return absent, nil
		}
		candidates = literal.Value().(valueinterfaces.ArrayValue).Elements()
		if len(candidates) == 0 {
			return absent, nil
		}
		for _, value := range candidates {
			if value.IsNull() || !value.Type().EqualsTo(arrayType.Element()) {
				return absent, nil
			}
		}
		name = functions.BuiltInArrayContainsAny
	}
	context := call.Context()
	metadata, err := context.Metadata(call.Handle())
	if err != nil {
		return absent, fmt.Errorf("membership source metadata: %w", err)
	}
	if !array_membership.RecognizeMetadata(metadata, name) {
		return absent, nil
	}
	return foundation.Some(exclusionOccurrence{index: index, exclusion: exclusion, call: call, array: args[0], context: context, candidates: candidates}), nil
}
