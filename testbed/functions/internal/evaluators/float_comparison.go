package evaluators

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// FloatComparison returns an evaluator using native float comparisons.
// They preserve unordered NaN behavior; cmp.Compare would impose
// a total order and would not implement this provider's SQL comparison policy.
func FloatComparison(operator foundation.OperatorType, resultType *types.BoolType) func([]valueinterfaces.Value) (valueinterfaces.Value, error) {
	return func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
		a, b := args[0].(valueinterfaces.FloatValue).Float(), args[1].(valueinterfaces.FloatValue).Float()
		var result bool
		switch operator {
		case foundation.Equal:
			result = a == b
		case foundation.NotEqual:
			result = a != b
		case foundation.LessThan:
			result = a < b
		case foundation.LessThanOrEqual:
			result = a <= b
		case foundation.GreaterThan:
			result = a > b
		case foundation.GreaterThanOrEqual:
			result = a >= b
		default:
			return nil, fmt.Errorf("%w: float comparison operator", shared.ErrContractViolation)
		}
		return testbedvalues.NewBool(resultType, result)
	}
}
