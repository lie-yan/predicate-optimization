// Package facts provides immutable predicate summaries shared by extraction,
// scoped reasoning and expression reconstruction.
package facts

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// PredicateSummary describes a predicate that is total and two-valued on
// non-null operand inputs. TrueSet records where it is true; all other non-null
// inputs yield false. NullTruth records the outcome on null input, when admitted.
// Exactness is a producer obligation, not something construction can prove.
// Instances share immutable inputs. Use NewPredicateSummary; the zero value is invalid.
type PredicateSummary struct {
	// operand identifies the referenced input whose values the summary constrains.
	operand *expressions.ReferenceExpression
	// trueSet contains exactly the non-null operand values for which the predicate is true.
	trueSet value_set.ValueSet
	// nullTruth records the predicate's outcome on null input and is present exactly when the operand is nullable.
	nullTruth foundation.Optional[foundation.TruthValue]
	// resultType preserves the source predicate's Boolean type, including its declared nullability.
	resultType *types.BoolType
	// source retains the original predicate expression described by this summary.
	source expressions.Expression
}

// NewPredicateSummary checks representation consistency without traversing or
// evaluating the source. The set must have the operand's Type with outer
// nullability removed. NullTruth is present exactly for nullable operands, and
// a Null outcome requires a nullable Boolean source Type. Result Type is taken
// directly from source, preserving conservative nullability. Existing operands,
// sets, expressions and Types must be valid and immutable; nil inputs assert.
func NewPredicateSummary(operand *expressions.ReferenceExpression, trueSet value_set.ValueSet, nullTruth foundation.Optional[foundation.TruthValue], source expressions.Expression) (*PredicateSummary, error) {
	assertions.NotNilPointer(operand)
	assertions.NotNil(trueSet)
	assertions.NotNil(source)
	resultType, ok := source.Type().(*types.BoolType)
	if !ok {
		return nil, fmt.Errorf("predicate summary source must have Boolean type")
	}
	if trueSet.Type().Nullability() != foundation.NonNull || !operand.Type().EqualsToIgnoringNullability(trueSet.Type()) {
		return nil, fmt.Errorf("%w: summary true set must match the non-null operand domain", value_set.ErrIncompatibleTypes)
	}
	truth, present := nullTruth.Get()
	if present != (operand.Type().Nullability() == foundation.MayNull) {
		return nil, fmt.Errorf("NullTruth must be present exactly for a nullable operand")
	}
	if present {
		switch truth {
		case foundation.False, foundation.True:
		case foundation.Null:
			if resultType.Nullability() != foundation.MayNull {
				return nil, fmt.Errorf("NullTruth Null requires a nullable Boolean result type")
			}
		default:
			return nil, fmt.Errorf("invalid NullTruth: %d", truth)
		}
	}
	return &PredicateSummary{operand: operand, trueSet: trueSet, nullTruth: nullTruth, resultType: resultType, source: source}, nil
}

func (s *PredicateSummary) Operand() *expressions.ReferenceExpression {
	s.assertConstructed()
	return s.operand
}
func (s *PredicateSummary) TrueSet() value_set.ValueSet { s.assertConstructed(); return s.trueSet }
func (s *PredicateSummary) NullTruth() foundation.Optional[foundation.TruthValue] {
	s.assertConstructed()
	return s.nullTruth
}
func (s *PredicateSummary) Type() types.Type               { s.assertConstructed(); return s.resultType }
func (s *PredicateSummary) Source() expressions.Expression { s.assertConstructed(); return s.source }

func (s *PredicateSummary) assertConstructed() {
	if s == nil || s.operand == nil {
		panic("facts: expected constructed PredicateSummary")
	}
}
