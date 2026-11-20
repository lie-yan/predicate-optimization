package expressions

import (
	"fmt"
	"strings"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// SpecialFormExpression stores a form, explicit result Type and ordered
// expression arguments. Form-specific contracts govern argument roles.
// Construction supports primitive scalar IN.
// Use NewSpecialFormExpression; zero is invalid.
type SpecialFormExpression struct {
	form      foundation.SpecialForm
	typ       types.Type
	arguments []Expression
}

var _ Expression = (*SpecialFormExpression)(nil)

// NewSpecialFormExpression validates the supported form's typing contract and
// copies arguments. Inputs must be valid immutable expressions; nil inputs
// assert. IN uses arguments[0] as target and the rest as candidate expressions.
// An empty candidate list is representable but its evaluation remains gated.
// No evaluation, normalization, coercion or overload resolution occurs here.
func NewSpecialFormExpression(form foundation.SpecialForm, resultType types.Type, arguments []Expression) (*SpecialFormExpression, error) {
	assertions.NotNil(resultType)
	for _, argument := range arguments {
		assertions.NotNil(argument)
	}
	if !form.IsValid() {
		return nil, fmt.Errorf("invalid special form: %d", form)
	}
	switch form {
	case foundation.SpecialFormIn:
		if err := validateInArguments(resultType, arguments); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("construction of special form %s is not supported", form)
	}
	return &SpecialFormExpression{form: form, typ: resultType, arguments: append([]Expression(nil), arguments...)}, nil
}

func validateInArguments(resultType types.Type, arguments []Expression) error {
	if len(arguments) == 0 {
		return fmt.Errorf("IN requires a target expression")
	}
	target := arguments[0].Type()
	switch target.(type) {
	case *types.BoolType, *types.IntType, *types.FloatType, *types.VarCharType:
	default:
		return fmt.Errorf("IN target must be Bool, integer, float or VARCHAR")
	}
	nullability := target.Nullability()
	for i, argument := range arguments[1:] {
		adapted, err := argument.Type().WithNullability(target.Nullability())
		if err != nil {
			return fmt.Errorf("IN candidate %d: %w", i, err)
		}
		if !adapted.EqualsTo(target) {
			return fmt.Errorf("IN candidate %d differs beyond outer nullability", i)
		}
		if argument.Type().Nullability() == foundation.MayNull {
			nullability = foundation.MayNull
		}
	}
	boolean, ok := resultType.(*types.BoolType)
	if !ok || boolean.Nullability() != nullability {
		return fmt.Errorf("IN result must be Bool with derived nullability %s", nullability.Suffix())
	}
	return nil
}

func (e *SpecialFormExpression) Form() foundation.SpecialForm {
	assertions.NotNilPointer(e)
	return e.form
}
func (e *SpecialFormExpression) Type() types.Type { assertions.NotNilPointer(e); return e.typ }

// Arguments returns a copy sharing immutable expression children.
func (e *SpecialFormExpression) Arguments() []Expression {
	assertions.NotNilPointer(e)
	return append([]Expression(nil), e.arguments...)
}
func (e *SpecialFormExpression) Children() []Expression { return e.Arguments() }

// ChildAt returns one shared child without copying a slice. Invalid indexes panic.
func (e *SpecialFormExpression) ChildAt(index int) Expression {
	assertions.NotNilPointer(e)
	return e.arguments[index]
}

// ChildCount returns the number of ordered expression children.
func (e *SpecialFormExpression) ChildCount() int {
	assertions.NotNilPointer(e)
	return len(e.arguments)
}

func (e *SpecialFormExpression) EqualsTo(other Expression) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	// Shared immutable instances need no comparison memo.
	if e == other {
		return true
	}
	return e.equalsTo(other, make(map[[2]Expression]bool))
}

func (e *SpecialFormExpression) equalsTo(other Expression, seen map[[2]Expression]bool) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	o, ok := other.(*SpecialFormExpression)
	if !ok {
		return false
	}
	pair := [2]Expression{e, o}
	if e == o || seen[pair] {
		return true
	}
	if e.form != o.form || !e.typ.EqualsTo(o.typ) || len(e.arguments) != len(o.arguments) {
		return false
	}
	for i, argument := range e.arguments {
		if !argument.equalsTo(o.arguments[i], seen) {
			return false
		}
	}
	seen[pair] = true
	return true
}

func (e *SpecialFormExpression) String() string {
	assertions.NotNilPointer(e)
	parts := make([]string, len(e.arguments))
	for i, argument := range e.arguments {
		parts[i] = argument.String()
	}
	return fmt.Sprintf("SpecialFormExpression(form=%s, type=%s, arguments=[%s])", e.form, e.typ, strings.Join(parts, ", "))
}
func (*SpecialFormExpression) isExpression() {}

// WithChildren implements checked immutable reconstruction.
func (e *SpecialFormExpression) WithChildren(children []Expression) (Expression, error) {
	assertions.NotNilPointer(e)
	if unchangedChildren(e.arguments, children) {
		return e, nil
	}
	rebuilt, err := NewSpecialFormExpression(e.form, e.typ, children)
	if err != nil {
		return nil, err
	}
	return checkRebuiltType(e, rebuilt)
}
