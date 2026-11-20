package array_exclusion_grouping_rewriter

import (
	"reflect"

	"github.com/lie-yan/predicate-optimization/expressions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
)

// exclusionGroup contains at least two compatible occurrences in source order.
// Its first member anchors placement, array identity and the paired provider.
type exclusionGroup struct {
	// Owned member slice; each occurrence shares its immutable expression and Values.
	members []exclusionOccurrence
}

// partitionKey narrows compatibility checks without hashing Types or handles.
// Context is admitted only after dynamic comparability has been established.
type partitionKey struct {
	// Exact provider identity, independent of its handles/catalog.
	context functioninterfaces.FunctionContext
	// Canonical identity for a reference; literals use their node identity instead.
	column string
	// Non-nil only for a literal array operand.
	literal *expressions.LiteralExpression
}

// groupExclusions partitions an ordered occurrence list without provider queries.
// First-seen group order and member order are preserved; singleton partitions and
// non-comparable contexts remain outside the returned replacement groups.
func groupExclusions(occurrences []exclusionOccurrence) []exclusionGroup {
	var partitions []exclusionGroup
	index := make(map[partitionKey][]int)
	for _, occurrence := range occurrences {
		if !reflect.ValueOf(occurrence.context).Comparable() {
			continue
		}
		key := partitionKey{context: occurrence.context}
		switch array := occurrence.array.(type) {
		case *expressions.ReferenceExpression:
			key.column = array.ID().Key()
		case *expressions.LiteralExpression:
			key.literal = array
		}
		found := false
		// Keep full-Type/identity verification inside the matching bucket. The
		// index never conflates differing nullability or nested constraints.
		for _, i := range index[key] {
			anchor := partitions[i].members[0]
			if sameArrayOperand(anchor.array, occurrence.array) {
				partitions[i].members = append(partitions[i].members, occurrence)
				found = true
				break
			}
		}
		if !found {
			index[key] = append(index[key], len(partitions))
			partitions = append(partitions, exclusionGroup{members: []exclusionOccurrence{occurrence}})
		}
	}
	var groups []exclusionGroup
	for _, partition := range partitions {
		if len(partition.members) >= 2 {
			groups = append(groups, partition)
		}
	}
	return groups
}

// sameArrayOperand respects full declared Types and column identity. Literal
// matching requires the same node, not merely equal-looking provider input Values.
func sameArrayOperand(left, right expressions.Expression) bool {
	if !left.Type().EqualsTo(right.Type()) {
		return false
	}
	switch operand := left.(type) {
	case *expressions.ReferenceExpression:
		other, ok := right.(*expressions.ReferenceExpression)
		return ok && operand.ID().EqualsTo(other.ID())
	case *expressions.LiteralExpression:
		other, ok := right.(*expressions.LiteralExpression)
		return ok && operand == other
	default:
		return false
	}
}
