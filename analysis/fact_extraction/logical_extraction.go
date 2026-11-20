package fact_extraction

import (
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/internal/diagnostics"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// extractLogical combines only exact summaries about the same reference and
// full Type. Boolean literals acquire that domain from a nonconstant child;
// unsupported or mixed-operand children prevent an exact parent summary.
func (e *ScalarExtractor) extractLogical(node *expressions.LogicalExpression) (*facts.PredicateSummary, error) {
	// Collect child summaries and literal truths before choosing a common operand domain.
	count := node.ChildCount()
	summaries := make([]*facts.PredicateSummary, count)
	truths := make([]foundation.TruthValue, count)
	var operand *expressions.ReferenceExpression
	unsupported := false
	for i := 0; i < count; i++ {
		child := node.ChildAt(i)
		// A known Boolean literal needs no extraction; its set is assigned after domain selection.
		if literal, ok := child.(*expressions.LiteralExpression); ok && !literal.Value().IsNull() {
			if literal.Value().(valueinterfaces.BoolValue).Bool() {
				truths[i] = foundation.True
			}
			continue
		}
		// Recursively analyze non-literals, retaining failures separately from unsupported children.
		result, err := e.Extract(child)
		if err != nil {
			return nil, err
		}
		summary, present := result.Summary().Get()
		if !present {
			unsupported = true
			continue
		}
		// Every exact child must constrain the same column identity and full Type.
		summaries[i] = summary
		if operand == nil {
			operand = summary.Operand()
		} else if !operand.EqualsTo(summary.Operand()) {
			unsupported = true
		}
	}
	// Inspect all children before rejecting the group: a decisive literal or an
	// unsupported sibling must not hide a later provider failure.
	if unsupported {
		return nil, diagnostics.LogicalUnsupported()
	}
	// Constants alone do not identify a value universe for a predicate summary.
	if operand == nil {
		return nil, diagnostics.ConstantOnly()
	}
	// Lift constants to universal/empty sets and collect each child's null-input outcome.
	var set value_set.ValueSet
	for i, summary := range summaries {
		var childSet value_set.ValueSet
		var err error
		if summary != nil {
			childSet = summary.TrueSet()
			if truth, present := summary.NullTruth().Get(); present {
				truths[i] = truth
			}
		} else if truths[i] == foundation.True {
			childSet, err = finite.NewExcludedEquatableValueSet(e.factory, operand.Type(), nil)
		} else {
			childSet, err = finite.NewEquatableValueSet(e.factory, operand.Type(), nil)
		}
		if err != nil {
			return nil, err
		}
		// Seed the fold with the first set, then intersect for AND or union for OR.
		if i == 0 {
			set = childSet
			continue
		}
		switch node.Operator() {
		case foundation.And:
			set, err = set.Intersect(childSet)
		case foundation.Or:
			set, err = set.Union(childSet)
		}
		if err != nil {
			return nil, err
		}
	}
	// Unary NOT complements the entire non-null universe, including NaN for floats.
	if node.Operator() == foundation.Not {
		var err error
		set, err = set.Complement()
		if err != nil {
			return nil, err
		}
	}
	// Compose null-input truth separately and preserve the original source and result Type.
	nullTruth := foundation.Optional[foundation.TruthValue]{}
	if operand.Type().Nullability() == foundation.MayNull {
		nullTruth = foundation.Some(composeNullTruth(node.Operator(), truths))
	}
	return facts.NewPredicateSummary(operand, set, nullTruth, node)
}

// composeNullTruth applies three-valued Boolean logic to the outcomes at null
// input, independently of the set algebra over non-null inputs.
func composeNullTruth(operator foundation.LogicalOperator, truths []foundation.TruthValue) foundation.TruthValue {
	// NOT swaps True and False while preserving Null.
	if operator == foundation.Not {
		switch truths[0] {
		case foundation.True:
			return foundation.False
		case foundation.False:
			return foundation.True
		default:
			return foundation.Null
		}
	}
	// For AND, False dominates; otherwise any Null yields Null, and all True yields True.
	if operator == foundation.And {
		result := foundation.True
		for _, truth := range truths {
			if truth == foundation.False {
				return foundation.False
			}
			if truth == foundation.Null {
				result = foundation.Null
			}
		}
		return result
	}
	// For OR, True dominates; otherwise any Null yields Null, and all False yields False.
	result := foundation.False
	for _, truth := range truths {
		if truth == foundation.True {
			return foundation.True
		}
		if truth == foundation.Null {
			result = foundation.Null
		}
	}
	return result
}
