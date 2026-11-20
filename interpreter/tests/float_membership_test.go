package interpreter_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestFloatMembership(t *testing.T) {
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		literal := func(n float64) *expressions.LiteralExpression {
			return must(expressions.NewLiteralExpression(must(testbedvalues.NewFloat(typ, n))))
		}
		for _, tc := range []struct {
			target, candidate float64
			want              foundation.TruthValue
		}{
			{1, 1, foundation.True}, {1, 2, foundation.False}, {math.Copysign(0, -1), 0, foundation.True}, {math.NaN(), math.NaN(), foundation.False}, {1, math.NaN(), foundation.False}, {math.Inf(1), math.Inf(1), foundation.True},
		} {
			root := membership(literal(tc.target), literal(tc.candidate))
			for _, partial := range []bool{false, true} {
				checkLogical(t, evaluator, root, tc.want, partial)
				not := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{root}))
				wantNot := foundation.True
				if tc.want == foundation.True {
					wantNot = foundation.False
				}
				checkLogical(t, evaluator, not, wantNot, partial)
			}
		}
		nullable := must(typ.WithNullability(foundation.MayNull))
		null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullable))))
		checkLogical(t, evaluator, membership(null, literal(1)), foundation.Null, false)
		checkLogical(t, evaluator, membership(null, literal(1)), foundation.Null, true)
		otherWidth := types.Float32
		if width == types.Float32 {
			otherWidth = types.Float64
		}
		other := must(expressions.NewLiteralExpression(must(testbedvalues.NewFloat(must(types.NewFloatType(otherWidth, foundation.NonNull)), 1))))
		if _, err := expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(foundation.NonNull)), []expressions.Expression{literal(1), other}); err == nil {
			t.Fatal("mixed widths admitted")
		}
	}
}
