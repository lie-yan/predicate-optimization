package array_exclusion_grouping_rewriter_test

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type membershipFixture struct {
	context functioninterfaces.FunctionContext // Preserve its dynamic representation.
	element types.Type
	array   *types.ArrayType // Non-null candidate container.
	input   *expressions.ReferenceExpression
}

func newMembershipFixture(context functioninterfaces.FunctionContext, element types.Type, n foundation.Nullability) *membershipFixture {
	array := must(types.NewArrayType(element, foundation.NonNull))
	return &membershipFixture{
		context: context, element: element, array: array,
		input: fixtureReference("array", must(array.WithNullability(n))),
	}
}

func fixtureReference(name string, typ types.Type) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(ids.NewColumnID("grouping", name)), typ, name))
}

func fixtureLiteral(value valueinterfaces.Value) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(value))
}

func fixtureLogical(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, children))
}

func (f *membershipFixture) candidates(items ...valueinterfaces.Value) *expressions.LiteralExpression {
	return fixtureLiteral(must(testbedvalues.NewArray(f.array, items)))
}

func (f *membershipFixture) inputValue(items ...valueinterfaces.Value) *testbedvalues.ArrayValue {
	return must(testbedvalues.NewArray(f.input.Type().(*types.ArrayType), items))
}

func (f *membershipFixture) call(name functions.BuiltInFunctionName, input, candidate expressions.Expression) *expressions.CallExpression {
	handle := must(f.context.LookupBuiltInFunction(name, []types.Type{input.Type(), candidate.Type()}))
	return must(expressions.NewCallExpression(f.context, handle, []expressions.Expression{input, candidate}))
}

func (f *membershipFixture) contains(input expressions.Expression, item valueinterfaces.Value) *expressions.CallExpression {
	return f.call(functions.BuiltInArrayContains, input, fixtureLiteral(item))
}

func (f *membershipFixture) containsAny(input expressions.Expression, items ...valueinterfaces.Value) *expressions.CallExpression {
	return f.call(functions.BuiltInArrayContainsAny, input, f.candidates(items...))
}

func fixtureExclude(call *expressions.CallExpression) *expressions.LogicalExpression {
	return fixtureLogical(foundation.Not, call)
}

// Expected grouping is assembled from explicit constants, never by traversing
// or extracting candidates from a source predicate or invoking the rewriter.
func (f *membershipFixture) expectedGroup(input expressions.Expression, items ...valueinterfaces.Value) *expressions.LogicalExpression {
	return fixtureExclude(f.containsAny(input, items...))
}

type fixtureBinding struct {
	id    expressioninterfaces.ColumnID
	value valueinterfaces.Value
}
type fixtureEnvironment struct {
	bindings []fixtureBinding
	lookups  []expressioninterfaces.ColumnID
	failure  error
}

func (e *fixtureEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	e.lookups = append(e.lookups, id)
	if e.failure != nil {
		return foundation.Optional[valueinterfaces.Value]{}, e.failure
	}
	for _, binding := range e.bindings {
		if binding.id.EqualsTo(id) {
			return foundation.Some(binding.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

func membershipDefinition(name functions.BuiltInFunctionName, args []types.Type, result types.Type, policies []functions.NullPolicy) testbed.Definition {
	metadata := must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(args, result)), string(name), foundation.Some(name), foundation.Optional[foundation.OperatorType]{}, policies))
	return testbed.Definition{Namespace: "grouping.fixture", Name: string(name), Metadata: metadata}
}

func membershipCatalog(element types.Type) *testbed.Catalog {
	array := must(types.NewArrayType(element, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	policies := []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
	return must(testbed.NewCatalog([]testbed.Definition{
		membershipDefinition(functions.BuiltInArrayContains, []types.Type{array, element}, boolean, policies),
		membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{array, array}, boolean, policies),
	}))
}

// The ordinary testbed supplies validation, null propagation and Type adaptation;
// these callbacks implement only the chosen element relation on concrete testbedvalues.
func executableMembershipContext(catalog *testbed.Catalog, equal func(valueinterfaces.Value, valueinterfaces.Value) bool) *testbed.Context {
	metadataContext := testbed.NewContext(catalog)
	var bindings []testbed.EvaluationBinding
	for _, handle := range catalog.Handles() {
		metadata := must(metadataContext.Metadata(handle))
		name, _ := metadata.BuiltInName().Get()
		resultType := metadata.Signature().ResultType().(*types.BoolType)
		bindings = append(bindings, testbed.EvaluationBinding{Handle: handle, Evaluate: func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
			candidates := []valueinterfaces.Value{args[1]}
			if name == functions.BuiltInArrayContainsAny {
				candidates = args[1].(*testbedvalues.ArrayValue).Elements()
			}
			for _, element := range args[0].(*testbedvalues.ArrayValue).Elements() {
				for _, candidate := range candidates {
					if equal(element, candidate) {
						return testbedvalues.NewBool(resultType, true)
					}
				}
			}
			return testbedvalues.NewBool(resultType, false)
		}})
	}
	return must(testbed.NewContextWithEvaluators(catalog, bindings))
}

// Float elements use IEEE equality (NaN never matches, signed zeros match).
// Other elements use typed structural equality, including nested composites.
// These are fixture contracts, not assertions about Milvus composite membership.
func fixtureElementEqual(left, right valueinterfaces.Value) bool {
	if floating, ok := left.(*testbedvalues.FloatValue); ok {
		return floating.Float() == right.(*testbedvalues.FloatValue).Float()
	}
	return left.EqualsTo(right)
}
