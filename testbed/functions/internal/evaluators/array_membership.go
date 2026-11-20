// Package evaluators supplies concrete testbed callbacks for checked implementation
// inputs. The parent function context handles validation and null propagation.
package evaluators

import (
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ArrayContains returns an evaluator that scans a checked Array<Int64!> for one non-null
// candidate. The shared boundary handles outer-array null propagation first.
func ArrayContains(resultType *types.BoolType) func([]valueinterfaces.Value) (valueinterfaces.Value, error) {
	return func(arguments []valueinterfaces.Value) (valueinterfaces.Value, error) {
		candidate := arguments[1].(valueinterfaces.IntegerValue).Integer()
		for _, element := range arguments[0].(valueinterfaces.ArrayValue).Elements() {
			if element.(valueinterfaces.IntegerValue).Integer() == candidate {
				return testbedvalues.NewBool(resultType, true)
			}
		}
		return testbedvalues.NewBool(resultType, false)
	}
}

// ArrayContainsAny returns an evaluator that tests a checked Array<Int64!> against a non-null
// candidate array. The shared boundary handles outer-array null propagation first.
func ArrayContainsAny(resultType *types.BoolType) func([]valueinterfaces.Value) (valueinterfaces.Value, error) {
	return func(arguments []valueinterfaces.Value) (valueinterfaces.Value, error) {
		// NULL propagation and argument checks already ran in the context.
		// An empty input cannot match, so avoid copying/hashing candidates.
		elements := arguments[0].(valueinterfaces.ArrayValue).Elements()
		if len(elements) == 0 {
			return testbedvalues.NewBool(resultType, false)
		}
		candidateValues := arguments[1].(valueinterfaces.ArrayValue).Elements()
		if len(candidateValues) == 0 {
			return testbedvalues.NewBool(resultType, false)
		}
		// Candidate positions can repeat; let the map grow with distinct values
		// instead of reserving capacity for the whole literal list.
		candidates := make(map[int64]struct{})
		for _, candidate := range candidateValues {
			candidates[candidate.(valueinterfaces.IntegerValue).Integer()] = struct{}{}
		}
		for _, element := range elements {
			if _, found := candidates[element.(valueinterfaces.IntegerValue).Integer()]; found {
				return testbedvalues.NewBool(resultType, true)
			}
		}
		return testbedvalues.NewBool(resultType, false)
	}
}
