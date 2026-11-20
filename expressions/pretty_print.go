package expressions

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// PrettyPrint returns a Unicode diagnostic tree in stored child order.
// The root and all reachable nodes must be valid, immutable and non-nil.
// Shared subtrees are expanded at every occurrence. The result has no
// trailing newline and is neither serialization nor an identity key.
func PrettyPrint(root Expression) string {
	assertions.NotNil(root)
	var out strings.Builder
	writePrettyLabel(&out, root)
	stack := []prettyPrintFrame{{node: root, children: root.Children()}}
	for len(stack) > 0 {
		parent := &stack[len(stack)-1]
		if parent.next == len(parent.children) {
			stack = stack[:len(stack)-1]
			continue
		}
		index := parent.next
		child := parent.children[index]
		parent.next++
		out.WriteByte('\n')
		// Ancestors retain whether their currently expanded child has later siblings.
		for _, ancestor := range stack[:len(stack)-1] {
			if ancestor.next < len(ancestor.children) {
				out.WriteString("│ ")
			} else {
				out.WriteString("  ")
			}
		}
		if parent.next < len(parent.children) {
			out.WriteString("├ ")
		} else {
			out.WriteString("└ ")
		}
		out.WriteString(prettyChildRole(parent.node, index))
		out.WriteString(": ")
		writePrettyLabel(&out, child)
		stack = append(stack, prettyPrintFrame{node: child, children: child.Children()})
	}
	return out.String()
}

// A frame retains one occurrence's children, read once, and its next child index.
type prettyPrintFrame struct {
	node     Expression
	children []Expression
	next     int
}

func prettyChildRole(parent Expression, index int) string {
	role := "child"
	switch node := parent.(type) {
	case *CallExpression:
		role = "arg"
	case *LogicalExpression:
		role = "operand"
	case *SpecialFormExpression:
		if node.Form() == foundation.SpecialFormIn {
			if index == 0 {
				return "target"
			}
			role = "candidate"
			index--
		} else {
			role = "arg"
		}
	}
	return role + "[" + strconv.Itoa(index) + "]"
}

// Labels use local getters only; Expression.String may recursively render children.
func writePrettyLabel(out *strings.Builder, expression Expression) {
	switch node := expression.(type) {
	case *LiteralExpression:
		out.WriteString("LiteralExpression value=")
		writePrettyValue(out, node.Value())
	case *ReferenceExpression:
		out.WriteString("ReferenceExpression name=")
		out.WriteString(strconv.QuoteToASCII(node.DisplayName()))
		out.WriteString(" id=")
		out.WriteString(strconv.QuoteToASCII(node.ID().String()))
	case *CallExpression:
		out.WriteString("CallExpression function=")
		handle := node.Handle()
		out.WriteString(strconv.QuoteToASCII(handle.Namespace()))
		out.WriteByte('.')
		out.WriteString(strconv.QuoteToASCII(handle.Name()))
	case *LogicalExpression:
		out.WriteString("LogicalExpression ")
		out.WriteString(node.Operator().String())
	case *SpecialFormExpression:
		out.WriteString("SpecialFormExpression ")
		out.WriteString(node.Form().String())
	case *OpaqueExpression:
		out.WriteString("OpaqueExpression name=")
		out.WriteString(strconv.QuoteToASCII(node.DisplayName()))
		out.WriteString(" id=")
		out.WriteString(strconv.QuoteToASCII(node.ID().String()))
	default:
		out.WriteString("Expression concrete=")
		out.WriteString(strconv.QuoteToASCII(fmt.Sprintf("%T", expression)))
	}
	out.WriteString(" : ")
	writePrettySingleLine(out, expression.Type().String())
}

// Preserve diagnostic ASCII punctuation while escaping bytes/runes that could alter layout.
func writePrettySingleLine(out *strings.Builder, text string) {
	for len(text) > 0 {
		if text[0] >= ' ' && text[0] <= '~' {
			out.WriteByte(text[0])
			text = text[1:]
			continue
		}
		_, size := utf8.DecodeRuneInString(text)
		quoted := strconv.QuoteToASCII(text[:size])
		out.WriteString(quoted[1 : len(quoted)-1])
		text = text[size:]
	}
}

func writePrettyValue(out *strings.Builder, value valueinterfaces.Value) {
	if value.IsNull() {
		out.WriteString("NULL")
		return
	}
	switch typ := value.Type().(type) {
	case *types.BoolType:
		out.WriteString(strconv.FormatBool(value.(valueinterfaces.BoolValue).Bool()))
	case *types.IntType:
		out.WriteString(strconv.FormatInt(value.(valueinterfaces.IntegerValue).Integer(), 10))
	case *types.FloatType:
		out.WriteString(strconv.FormatFloat(value.(valueinterfaces.FloatValue).Float(), 'g', -1, int(typ.FloatWidth())))
	case *types.VarCharType:
		out.WriteString(strconv.QuoteToASCII(value.(valueinterfaces.VarCharValue).VarChar()))
	case *types.ArrayType:
		out.WriteByte('[')
		writePrettyValues(out, value.(valueinterfaces.ArrayValue).Elements())
		out.WriteByte(']')
	case *types.StructType:
		out.WriteByte('{')
		writePrettyValues(out, value.(valueinterfaces.StructValue).Fields())
		out.WriteByte('}')
	default:
		writePrettySingleLine(out, fmt.Sprintf("<%T>", value))
	}
}

func writePrettyValues(out *strings.Builder, elements []valueinterfaces.Value) {
	for i, element := range elements {
		if i > 0 {
			out.WriteString(", ")
		}
		writePrettyValue(out, element)
	}
}
