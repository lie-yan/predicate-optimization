package expressions

import (
	"fmt"

	interfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/types"
)

// OpaqueExpression preserves an unsupported region with a known result Type.
// It exposes no children and stores no upstream object or executor state.
// Use NewOpaqueExpression; the zero value is invalid.
type OpaqueExpression struct {
	id          interfaces.OpaqueID
	typ         types.Type
	displayName string
}

var _ Expression = (*OpaqueExpression)(nil)

// NewOpaqueExpression shares the caller-owned immutable ID and result Type.
// Nil IDs/Types assert; invalid non-nil Types return errors. ID validity belongs
// to its provider. displayName
// may be empty and does not participate in identity. The adapter is responsible
// for preserving the original region; construction never evaluates it.
func NewOpaqueExpression(id interfaces.OpaqueID, typ types.Type, displayName string) (*OpaqueExpression, error) {
	assertions.NotNil(id)
	assertions.NotNil(typ)
	return &OpaqueExpression{id: id, typ: typ, displayName: displayName}, nil
}

func (e *OpaqueExpression) ID() interfaces.OpaqueID { assertions.NotNilPointer(e); return e.id }
func (e *OpaqueExpression) Type() types.Type        { assertions.NotNilPointer(e); return e.typ }
func (e *OpaqueExpression) DisplayName() string     { assertions.NotNilPointer(e); return e.displayName }
func (e *OpaqueExpression) Children() []Expression  { assertions.NotNilPointer(e); return nil }

// ChildAt returns one shared child without copying a slice. Invalid indexes panic.
func (e *OpaqueExpression) ChildAt(index int) Expression {
	assertions.NotNilPointer(e)
	panic("expressions: leaf has no children")
}

// ChildCount returns the number of ordered expression children.
func (e *OpaqueExpression) ChildCount() int { assertions.NotNilPointer(e); return 0 }

func (e *OpaqueExpression) EqualsTo(other Expression) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	return e.equalsTo(other, nil)
}

func (e *OpaqueExpression) equalsTo(other Expression, _ map[[2]Expression]bool) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	o, ok := other.(*OpaqueExpression)
	return ok && e.id.EqualsTo(o.id) && e.typ.EqualsTo(o.typ)
}

func (e *OpaqueExpression) String() string {
	assertions.NotNilPointer(e)
	return fmt.Sprintf("OpaqueExpression(id=%s, type=%s, name=%q)", e.id, e.typ, e.displayName)
}
func (*OpaqueExpression) isExpression() {}

// WithChildren implements checked immutable reconstruction.
func (e *OpaqueExpression) WithChildren(children []Expression) (Expression, error) {
	assertions.NotNilPointer(e)
	for _, child := range children {
		assertions.NotNil(child)
	}
	if len(children) != 0 {
		return nil, fmt.Errorf("leaf expression accepts no children")
	}
	return e, nil
}
