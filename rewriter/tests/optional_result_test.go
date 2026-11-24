package rewriter_test

import (
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
