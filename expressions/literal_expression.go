package expressions

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// LiteralExpression holds a fully known typed Value, including null and
// composites. Use NewLiteralExpression; the zero value is invalid.
type LiteralExpression struct {
	value valueinterfaces.Value
}

var _ Expression = (*LiteralExpression)(nil)

// NewLiteralExpression shares a valid immutable Value. Nil, including
// typed nil, violates the input precondition and panics.
func NewLiteralExpression(value valueinterfaces.Value) (*LiteralExpression, error) {
	assertions.NotNil(value)
	return &LiteralExpression{value: value}, nil
}

// Value returns the shared immutable Value.
func (e *LiteralExpression) Value() valueinterfaces.Value {
	assertions.NotNilPointer(e)
	return e.value
}

func (e *LiteralExpression) Type() types.Type { assertions.NotNilPointer(e); return e.value.Type() }

// Children is empty even for composite Values: their elements are not expressions.
func (e *LiteralExpression) Children() []Expression { assertions.NotNilPointer(e); return nil }

// ChildAt returns one shared child without copying a slice. Invalid indexes panic.
func (e *LiteralExpression) ChildAt(index int) Expression {
	assertions.NotNilPointer(e)
	panic("expressions: leaf has no children")
}

// ChildCount returns the number of ordered expression children.
func (e *LiteralExpression) ChildCount() int { assertions.NotNilPointer(e); return 0 }

func (e *LiteralExpression) EqualsTo(other Expression) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	return e.equalsTo(other, nil)
}

func (e *LiteralExpression) equalsTo(other Expression, _ map[[2]Expression]bool) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	o, ok := other.(*LiteralExpression)
	return ok && e.value.EqualsTo(o.value)
}

func (*LiteralExpression) isExpression() {}

func (e *LiteralExpression) String() string {
	assertions.NotNilPointer(e)
	return fmt.Sprintf("LiteralExpression(%s, %s)", e.Type(), formatDiagnosticValue(e.value))
}

// Formatting is local diagnostic behavior; semantic operations dispatch through
// Value. Future Value variants can still be displayed by their concrete type.
func formatDiagnosticValue(value valueinterfaces.Value) string {
	if value.IsNull() {
		return "NULL"
	}
	switch typ := value.Type().(type) {
	case *types.BoolType:
		return strconv.FormatBool(value.(valueinterfaces.BoolValue).Bool())
	case *types.IntType:
		return strconv.FormatInt(value.(valueinterfaces.IntegerValue).Integer(), 10)
	case *types.FloatType:
		return strconv.FormatFloat(value.(valueinterfaces.FloatValue).Float(), 'g', -1, int(typ.FloatWidth()))
	case *types.VarCharType:
		return strconv.Quote(value.(valueinterfaces.VarCharValue).VarChar())
	case *types.ArrayType:
		return "[" + formatDiagnosticValues(value.(valueinterfaces.ArrayValue).Elements()) + "]"
	case *types.StructType:
		return "{" + formatDiagnosticValues(value.(valueinterfaces.StructValue).Fields()) + "}"
	default:
		return fmt.Sprintf("<%T>", value)
	}
}

func formatDiagnosticValues(children []valueinterfaces.Value) string {
	parts := make([]string, len(children))
	for i, child := range children {
		parts[i] = formatDiagnosticValue(child)
	}
	return strings.Join(parts, ", ")
}

// WithChildren implements checked immutable reconstruction.
func (e *LiteralExpression) WithChildren(children []Expression) (Expression, error) {
	assertions.NotNilPointer(e)
	for _, child := range children {
		assertions.NotNil(child)
	}
	if len(children) != 0 {
		return nil, fmt.Errorf("leaf expression accepts no children")
	}
	return e, nil
}
