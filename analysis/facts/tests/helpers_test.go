package facts_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	expressiontestbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
	"github.com/lie-yan/predicate-optimization/types"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func reference(typ types.Type, scope, symbol string) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(expressiontestbed.NewColumnID(scope, symbol)), typ, "display"))
}

func expectAssertion(t *testing.T, call func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected precondition assertion")
		}
	}()
	call()
}
