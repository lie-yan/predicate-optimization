package expressions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestCompositeExpressionIdentityWithoutAllocations(t *testing.T) {
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	literal := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	provider := fixture(testbed.NewStandardContext())
	handle := fixture(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer}))
	boolean := logicalLiteral(foundation.NonNull, true)
	cases := []expressions.Expression{
		fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{boolean, boolean})),
		fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{literal, literal})),
		fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean.Type(), []expressions.Expression{literal, literal})),
	}
	for _, expression := range cases {
		if n := testing.AllocsPerRun(100, func() {
			if !expression.EqualsTo(expression) {
				panic("identity rejected")
			}
		}); n != 0 {
			t.Fatalf("%T identity comparison allocated %g objects", expression, n)
		}
	}
}
