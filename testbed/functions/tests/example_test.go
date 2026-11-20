package functions_test

import (
	"fmt"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func ExampleNewStandardContext() {
	standard, err := testbed.NewStandardContext()
	if err != nil {
		panic(err)
	}
	// Consumers depend on the shared interface. Setup selects the provider.
	var provider functioninterfaces.FunctionContext = standard
	boolean, err := types.NewBoolType(foundation.MayNull)
	if err != nil {
		panic(err)
	}
	argument, err := testbedvalues.NewBool(boolean, true)
	if err != nil {
		panic(err)
	}
	argumentTypes := []types.Type{argument.Type()}
	handle, err := provider.LookupBuiltInFunction(shared.BuiltInNot, argumentTypes)
	if err != nil {
		panic(err)
	}
	metadata, err := provider.Metadata(handle)
	if err != nil {
		panic(err)
	}
	// Bind from expression argument types, not the handle's implementation types.
	signature, err := shared.BindFunctionSignature(metadata, argumentTypes)
	if err != nil {
		panic(err)
	}
	result, err := provider.Evaluate(handle, signature, []valueinterfaces.Value{argument})
	if err != nil {
		panic(err)
	}
	name, _ := metadata.BuiltInName().Get()
	fmt.Println(name)
	fmt.Println(metadata.Signature().ResultType())
	fmt.Println(result.Type(), result.(*testbedvalues.BoolValue).Bool())

	null, err := testbedvalues.NewNull(boolean)
	if err != nil {
		panic(err)
	}
	result, err = provider.Evaluate(handle, signature, []valueinterfaces.Value{null})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Type(), result.IsNull())
	// Output:
	// not
	// Bool!
	// Bool? false
	// Bool? true
}
