package functions_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
)

func Example() {
	name := functions.BuiltInArrayContainsAny
	operator := foundation.Negate
	fmt.Println(name)
	fmt.Println(operator.IsArithmeticOperator())
	// Output:
	// array_contains_any
	// true
}
