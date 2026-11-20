package array_exclusion_grouping_rewriter

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// rewriteConjunction prepares all local replacements before rebuilding the region.
// Local misses retain their members; any error discards all tentative local work.
func rewriteConjunction(factory valueinterfaces.ValueFactory, source *expressions.LogicalExpression) (expressions.Expression, error) {
	occurrences, err := collectExclusions(source)
	if err != nil {
		return nil, err
	}
	groups := groupExclusions(occurrences)
	if len(groups) == 0 {
		return source, nil
	}
	children := source.Children()
	replacements := make([]expressions.Expression, len(children))
	removed := make([]bool, len(children))
	changed := false
	beforeCandidates, afterCandidates := 0, 0
	for _, group := range groups {
		candidate, err := buildGroupReplacement(factory, group)
		if err != nil {
			return nil, err
		}
		replacement, present := candidate.Get()
		if !present {
			continue
		}
		changed = true
		replacements[group.members[0].index] = replacement
		for i, member := range group.members {
			beforeCandidates += len(member.candidates)
			if i != 0 {
				removed[member.index] = true
			}
		}
		// Only successful groups alter candidate payloads; all other subtrees are
		// retained. Counting their aggregate positions proves region-wide no growth
		// without querying metadata on unrelated expressions or flattening nested Values.
		call := replacement.(*expressions.LogicalExpression).Children()[0].(*expressions.CallExpression)
		literal := call.Arguments()[1].(*expressions.LiteralExpression).Value().(valueinterfaces.ArrayValue)
		afterCandidates += literal.ElementCount()
	}
	if !changed || afterCandidates > beforeCandidates {
		return source, nil
	}
	resultChildren := make([]expressions.Expression, 0, len(children))
	for index, child := range children {
		if removed[index] {
			continue
		}
		if replacements[index] != nil {
			child = replacements[index]
		}
		resultChildren = append(resultChildren, child)
	}
	var result expressions.Expression
	if len(resultChildren) == 1 {
		result = resultChildren[0]
	} else {
		result, err = expressions.NewLogicalExpression(foundation.And, resultChildren)
		if err != nil {
			return nil, fmt.Errorf("group AND construction: %w", err)
		}
	}
	if !result.Type().EqualsTo(source.Type()) || reconstruction.MeasureExpression(result).NodeCount > reconstruction.MeasureExpression(source).NodeCount {
		return source, nil
	}
	return result, nil
}
