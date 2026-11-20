package fact_extraction_test

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func boolLiteral(n foundation.Nullability, value bool) expressions.Expression {
	typ := must(types.NewBoolType(n))
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(typ, value))))
}
