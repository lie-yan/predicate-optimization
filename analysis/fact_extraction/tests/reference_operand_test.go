package fact_extraction_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// A caller-owned ID with a slice cannot be compared with == or used as a map key.
// All instances deliberately have the same diagnostic spelling.
type pathID struct{ parts []string }

func (id pathID) String() string                          { return "column" }
func (id pathID) Key() string                             { return fmt.Sprintf("fact-extraction-tests/column:%q", id.parts) }
func (id pathID) EqualsTo(other interfaces.ColumnID) bool { return id.Key() == other.Key() }

func reference(typ types.Type, parts ...string) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(pathID{parts: parts}, typ, "display"))
}

func TestReferenceOperandIdentity(t *testing.T) {
	typ := must(types.NewIntType(types.Int64, foundation.MayNull))
	a := reference(typ, "t", "x")
	same := must(expressions.NewReferenceExpression(pathID{[]string{"t", "x"}}, must(types.NewIntType(types.Int64, foundation.MayNull)), "renamed"))
	if !a.EqualsTo(same) || !same.EqualsTo(a) || !a.EqualsTo(a) {
		t.Fatal("operand identity depended on display, allocation or Go comparability")
	}
	for _, other := range []*expressions.ReferenceExpression{
		reference(typ, "other", "x"),
		reference(typ, "t", "y"),
		reference(must(types.NewIntType(types.Int64, foundation.NonNull)), "t", "x"),
		reference(must(types.NewIntType(types.Int8, foundation.MayNull)), "t", "x"),
	} {
		if a.EqualsTo(other) || other.EqualsTo(a) {
			t.Fatal("column identity or full Type ignored")
		}
	}
}

func expectAssertion(t *testing.T, call func()) {
	t.Helper()
	if !assertions.Enabled {
		return
	}
	defer func() {
		if recover() == nil {
			t.Error("expected precondition assertion")
		}
	}()
	call()
}
