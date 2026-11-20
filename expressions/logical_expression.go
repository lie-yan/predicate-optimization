package expressions

import (
	"fmt"
	"strings"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// LogicalExpression represents ordered Boolean composition. Construction
// preserves grouping and duplicates; evaluation and simplification are separate.
// Use NewLogicalExpression; the zero value is invalid.
type LogicalExpression struct {
	operator foundation.LogicalOperator
	typ      *types.BoolType
	children []Expression
}

var _ Expression = (*LogicalExpression)(nil)

// NewLogicalExpression accepts one child for NOT and at least two for AND/OR.
// Children must be valid immutable expressions; nil children assert. Invalid
// operators, arity or non-Boolean operands return errors. Result nullability
// is conservative: any nullable child makes the result nullable.
func NewLogicalExpression(operator foundation.LogicalOperator, children []Expression) (*LogicalExpression, error) {
	for _, child := range children {
		assertions.NotNil(child)
	}
	if !operator.IsValid() {
		return nil, fmt.Errorf("invalid logical operator: %d", operator)
	}
	if (operator == foundation.Not && len(children) != 1) || (operator != foundation.Not && len(children) < 2) {
		return nil, fmt.Errorf("invalid arity %d for %s", len(children), operator)
	}
	nullability := foundation.NonNull
	for i, child := range children {
		boolean, ok := child.Type().(*types.BoolType)
		if !ok {
			return nil, fmt.Errorf("logical operand %d must have Boolean type", i)
		}
		if boolean.Nullability() == foundation.MayNull {
			nullability = foundation.MayNull
		}
	}
	typ, err := types.NewBoolType(nullability)
	if err != nil {
		return nil, err
	}
	return &LogicalExpression{operator: operator, typ: typ, children: append([]Expression(nil), children...)}, nil
}

func (e *LogicalExpression) Operator() foundation.LogicalOperator {
	assertions.NotNilPointer(e)
	return e.operator
}
func (e *LogicalExpression) Type() types.Type { assertions.NotNilPointer(e); return e.typ }

// Children returns a copied slice sharing immutable child expressions.
func (e *LogicalExpression) Children() []Expression {
	assertions.NotNilPointer(e)
	return append([]Expression(nil), e.children...)
}

// ChildAt returns one shared child without copying a slice. Invalid indexes panic.
func (e *LogicalExpression) ChildAt(index int) Expression {
	assertions.NotNilPointer(e)
	return e.children[index]
}

// ChildCount returns the number of ordered expression children.
func (e *LogicalExpression) ChildCount() int { assertions.NotNilPointer(e); return len(e.children) }

func (e *LogicalExpression) EqualsTo(other Expression) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	// Shared immutable instances need no comparison memo.
	if e == other {
		return true
	}
	return e.equalsTo(other, make(map[[2]Expression]bool))
}

func (e *LogicalExpression) equalsTo(other Expression, seen map[[2]Expression]bool) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	o, ok := other.(*LogicalExpression)
	if !ok {
		return false
	}
	pair := [2]Expression{e, o}
	if e == o || seen[pair] {
		return true
	}
	if e.operator != o.operator || !e.typ.EqualsTo(o.typ) || len(e.children) != len(o.children) {
		return false
	}
	for i, child := range e.children {
		if !child.equalsTo(o.children[i], seen) {
			return false
		}
	}
	seen[pair] = true
	return true
}

func (e *LogicalExpression) String() string {
	assertions.NotNilPointer(e)
	parts := make([]string, len(e.children))
	for i, child := range e.children {
		parts[i] = child.String()
	}
	return fmt.Sprintf("LogicalExpression(operator=%s, type=%s, children=[%s])", e.operator, e.typ, strings.Join(parts, ", "))
}

func (*LogicalExpression) isExpression() {}

// WithChildren implements checked immutable reconstruction.
func (e *LogicalExpression) WithChildren(children []Expression) (Expression, error) {
	assertions.NotNilPointer(e)
	if unchangedChildren(e.children, children) {
		return e, nil
	}
	rebuilt, err := NewLogicalExpression(e.operator, children)
	if err != nil {
		return nil, err
	}
	return checkRebuiltType(e, rebuilt)
}
