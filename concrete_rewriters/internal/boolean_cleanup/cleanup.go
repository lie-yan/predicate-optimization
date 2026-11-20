package boolean_cleanup

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// TruthLiteral preserves the original Boolean Type, including its nullability.
// A required nil Type asserts; invalid truths and NULL under Bool! are contract errors.
func TruthLiteral(factory valueinterfaces.ValueFactory, typ *types.BoolType, truth foundation.TruthValue) (*expressions.LiteralExpression, error) {
	assertions.NotNil(factory)
	if typ == nil {
		panic("boolean_cleanup: expected non-nil Boolean type")
	}
	var value valueinterfaces.Value
	var err error
	switch truth {
	case foundation.True, foundation.False:
		value, err = factory.NewBool(typ, truth == foundation.True)
	case foundation.Null:
		value, err = factory.NewNull(typ)
	default:
		return nil, fmt.Errorf("%w: invalid truth value %d", rewriter.ErrInvalidReplacement, truth)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: truth literal for %s: %w", rewriter.ErrInvalidReplacement, typ, err)
	}
	literal, err := expressions.NewLiteralExpression(value)
	if err != nil {
		return nil, fmt.Errorf("%w: truth literal: %w", rewriter.ErrInvalidReplacement, err)
	}
	return literal, nil
}

// CleanupLogical applies fixed Boolean identities to proposed
// children. Child replacements must already be justified by the caller. Absence
// includes expected Type narrowing; callers may still rebuild with those children.
func (r *Cleaner) CleanupLogical(source *expressions.LogicalExpression, children []expressions.Expression) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	assertions.NotNilPointer(source)
	if admitted, err := r.CanChangeChildren(source, children); err != nil || !admitted {
		return absent, err
	}
	operator := source.Operator()
	var candidate expressions.Expression
	var err error
	if operator == foundation.Not {
		truth, literal := literalTruth(children[0])
		if !literal {
			return absent, nil
		}
		// NOT preserves NULL; only concrete Boolean truth is inverted.
		switch truth {
		case foundation.True:
			truth = foundation.False
		case foundation.False:
			truth = foundation.True
		}
		candidate, err = TruthLiteral(r.factory, source.Type().(*types.BoolType), truth)
	} else {
		neutral, decisive := foundation.True, foundation.False
		if operator == foundation.Or {
			neutral, decisive = foundation.False, foundation.True
		}
		retained := make([]expressions.Expression, 0, len(children))
		allLiteral, combined := true, neutral
		for _, child := range children {
			truth, literal := literalTruth(child)
			if !literal {
				allLiteral = false
				retained = append(retained, child)
				continue
			}
			// Compute three-valued truth for the all-literal case. A decisive literal
			// wins over NULL, but does not justify dropping any nonliteral computation.
			if truth == decisive {
				combined = decisive
			} else if truth == foundation.Null && combined != decisive {
				combined = foundation.Null
			}
			if truth != neutral {
				retained = append(retained, child)
			}
		}
		// An absorbing literal fixes the value, but every proposed operand would
		// disappear. Inspect each occurrence, including those after an ineligible
		// sibling, so refusal cannot hide a later provider error. If omission is
		// refused, fall back to neutral cleanup that retains those computations.
		absorb := false
		if !allLiteral && combined == decisive {
			absorb = true
			for i, child := range children {
				eligible, err := r.CanOmit(child)
				if err != nil {
					return absent, fmt.Errorf("%s operand %d omission: %w", operator, i, err)
				}
				absorb = absorb && eligible
			}
		}
		switch {
		case allLiteral || absorb:
			// This also covers zero survivors: AND(True, True) -> True and
			// OR(False, False) -> False, with the original group's Type.
			candidate, err = TruthLiteral(r.factory, source.Type().(*types.BoolType), combined)
		case len(retained) == len(children):
			return absent, nil
		case len(retained) == 1:
			candidate = retained[0]
		default:
			// Preserve survivor order and grouping; remove only neutral literals.
			candidate, err = expressions.NewLogicalExpression(operator, retained)
		}
	}
	if err != nil {
		return absent, err
	}
	// A:Bool! AND True:Bool? cannot become A:Bool! because the group is Bool?.
	// This is an inapplicable cleanup, not malformed child evidence.
	if !candidate.Type().EqualsTo(source.Type()) {
		return absent, nil
	}
	return foundation.Some(candidate), nil
}

// literalTruth reads known Boolean/NULL payloads without evaluating expressions.
// Callers establish Boolean Types before inspecting children.
func literalTruth(expression expressions.Expression) (foundation.TruthValue, bool) {
	literal, ok := expression.(*expressions.LiteralExpression)
	if !ok {
		return foundation.False, false
	}
	if literal.Value().IsNull() {
		return foundation.Null, true
	}
	if literal.Value().(valueinterfaces.BoolValue).Bool() {
		return foundation.True, true
	}
	return foundation.False, true
}
