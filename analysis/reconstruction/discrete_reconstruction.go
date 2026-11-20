package reconstruction

import (
	"math"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/internal/function_admission"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// reconstructDiscrete preserves stored entry order and shares immutable Values.
// Callers supply a nonempty list and have already checked null/result-Type policy.
func (r *ScalarReconstructor) reconstructDiscrete(summary *facts.PredicateSummary, entries []valueinterfaces.Value, isInclusion bool) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	for _, entry := range entries {
		if _, floating := entry.Type().(*types.FloatType); floating && math.IsNaN(entry.(valueinterfaces.FloatValue).Float()) {
			return absent, nil
		}
	}
	if len(entries) == 1 {
		literal, err := expressions.NewLiteralExpression(entries[0])
		if err != nil {
			return absent, err
		}
		operator := foundation.Equal
		_, boolean := summary.Operand().Type().(*types.BoolType)
		if !isInclusion && !boolean {
			operator = foundation.NotEqual
		}
		call, err := function_admission.ResolveComparison(r.context, operator, summary.Operand(), literal)
		if err != nil {
			return absent, err
		}
		if !isInclusion && boolean {
			not, err := expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{call})
			if err != nil {
				return absent, err
			}
			return foundation.Some[expressions.Expression](not), nil
		}
		return foundation.Some[expressions.Expression](call), nil
	}

	// Admit the target context's equality before constructing a nonempty IN list.
	// IN stores no context; its consumer must use the same admitted equality semantics.
	if err := function_admission.CheckMembershipEquality(r.context, summary.Operand().Type(), entries[0].Type()); err != nil {
		return absent, err
	}
	arguments := make([]expressions.Expression, len(entries)+1)
	arguments[0] = summary.Operand()
	for i, entry := range entries {
		literal, err := expressions.NewLiteralExpression(entry)
		if err != nil {
			return absent, err
		}
		arguments[i+1] = literal
	}
	in, err := expressions.NewSpecialFormExpression(foundation.SpecialFormIn, summary.Type(), arguments)
	if err != nil {
		return absent, err
	}
	if isInclusion {
		return foundation.Some[expressions.Expression](in), nil
	}
	not, err := expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{in})
	if err != nil {
		return absent, err
	}
	return foundation.Some[expressions.Expression](not), nil
}
