package evaluators

import (
	"fmt"
	"math"
	"math/big"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// IntegerArithmetic returns an evaluator for the verified Int64 domain. Exact
// intermediates prevent Go overflow from silently defining Milvus semantics.
func IntegerArithmetic(operator foundation.OperatorType, resultType *types.IntType) func([]valueinterfaces.Value) (valueinterfaces.Value, error) {
	return func(arguments []valueinterfaces.Value) (valueinterfaces.Value, error) {
		left := arguments[0].(valueinterfaces.IntegerValue).Integer()
		a := big.NewInt(left)
		result := new(big.Int)
		if operator == foundation.Negate {
			result.Neg(a)
		} else {
			right := arguments[1].(valueinterfaces.IntegerValue).Integer()
			b := big.NewInt(right)
			if operator == foundation.Divide || operator == foundation.Modulus {
				if right == 0 {
					return nil, fmt.Errorf("%w: division or modulus by zero", shared.ErrEvaluationFailure)
				}
				// Even remainder is excluded: the corresponding signed C++
				// division is not representable for this operand pair.
				if left == math.MinInt64 && right == -1 {
					return nil, fmt.Errorf("%w: unresolved Int64 division overflow", shared.ErrEvaluationUnavailable)
				}
			}
			switch operator {
			case foundation.Add:
				result.Add(a, b)
			case foundation.Subtract:
				result.Sub(a, b)
			case foundation.Multiply:
				result.Mul(a, b)
			case foundation.Divide:
				result.Quo(a, b)
			case foundation.Modulus:
				result.Rem(a, b)
			default:
				return nil, fmt.Errorf("%w: unsupported arithmetic operator", shared.ErrContractViolation)
			}
		}
		if !result.IsInt64() {
			return nil, fmt.Errorf("%w: unresolved Int64 arithmetic overflow", shared.ErrEvaluationUnavailable)
		}
		return testbedvalues.NewInteger(resultType, result.Int64())
	}
}
