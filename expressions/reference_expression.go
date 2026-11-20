package expressions

import (
	"fmt"

	interfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/types"
)

// ReferenceExpression denotes a resolved input, independently of whether its
// value is available. Use NewReferenceExpression; the zero value is invalid.
// A binding environment must reject conflicting Types for the same interfaces.ColumnID.
type ReferenceExpression struct {
	id          interfaces.ColumnID
	typ         types.Type
	displayName string
}

var _ Expression = (*ReferenceExpression)(nil)

// NewReferenceExpression shares the caller-owned immutable ID and Type.
// Nil IDs/Types assert; invalid non-nil Types return errors. ID validity is
// the caller's responsibility. The diagnostic
// name may be empty. Construction does not register a binding or resolve a Value.
func NewReferenceExpression(id interfaces.ColumnID, typ types.Type, displayName string) (*ReferenceExpression, error) {
	assertions.NotNil(id)
	assertions.NotNil(typ)
	return &ReferenceExpression{id: id, typ: typ, displayName: displayName}, nil
}

// ID returns the shared caller-owned immutable column identity.
func (e *ReferenceExpression) ID() interfaces.ColumnID { assertions.NotNilPointer(e); return e.id }

func (e *ReferenceExpression) Type() types.Type { assertions.NotNilPointer(e); return e.typ }

func (e *ReferenceExpression) DisplayName() string { assertions.NotNilPointer(e); return e.displayName }

func (e *ReferenceExpression) Children() []Expression { assertions.NotNilPointer(e); return nil }

// ChildAt returns one shared child without copying a slice. Invalid indexes panic.
func (e *ReferenceExpression) ChildAt(index int) Expression {
	assertions.NotNilPointer(e)
	panic("expressions: leaf has no children")
}

// ChildCount returns the number of ordered expression children.
func (e *ReferenceExpression) ChildCount() int { assertions.NotNilPointer(e); return 0 }

func (e *ReferenceExpression) EqualsTo(other Expression) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	return e.equalsTo(other, nil)
}

func (e *ReferenceExpression) equalsTo(other Expression, _ map[[2]Expression]bool) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	o, ok := other.(*ReferenceExpression)
	return ok && e.id.EqualsTo(o.id) && e.typ.EqualsTo(o.typ)
}

func (e *ReferenceExpression) String() string {
	assertions.NotNilPointer(e)
	return fmt.Sprintf("ReferenceExpression(column=%s, type=%s, name=%q)", e.id, e.typ, e.displayName)
}

func (*ReferenceExpression) isExpression() {}

// WithChildren implements checked immutable reconstruction.
func (e *ReferenceExpression) WithChildren(children []Expression) (Expression, error) {
	assertions.NotNilPointer(e)
	for _, child := range children {
		assertions.NotNil(child)
	}
	if len(children) != 0 {
		return nil, fmt.Errorf("leaf expression accepts no children")
	}
	return e, nil
}
