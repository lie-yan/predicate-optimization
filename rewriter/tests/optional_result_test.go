package rewriter_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func literal() *expressions.LiteralExpression {
	typ := must(types.NewBoolType(foundation.MayNull))
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(typ))))
}

func TestOptionalReplacement(t *testing.T) {
	var useDefault foundation.Optional[expressions.Expression]
	if _, present := useDefault.Get(); present {
		t.Fatal("zero optional must request default traversal")
	}
	null := literal()
	replacement := foundation.Some[expressions.Expression](null)
	node, present := replacement.Get()
	if !present || node != null {
		t.Fatal("NULL literal replacement lost")
	}
	// Optional itself can contain nil. This is distinct from absence, but violates
	// the callback contract and is rejected by traversal.
	invalid := foundation.Some[expressions.Expression](nil)
	if _, present := invalid.Get(); !present {
		t.Fatal("present nil conflated with default traversal")
	}
}
