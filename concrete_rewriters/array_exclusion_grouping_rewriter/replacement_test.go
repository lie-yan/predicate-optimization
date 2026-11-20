package array_exclusion_grouping_rewriter

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type replacementContext struct {
	functioninterfaces.FunctionContext
	events      []string
	lookupTypes []types.Type
	target      functioninterfaces.FunctionHandle
	failure     string
	cause       error
	reads       int
}

func (c *replacementContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	c.events = append(c.events, "lookup")
	c.lookupTypes = slices.Clone(args)
	if c.failure == "lookup" {
		return nil, c.cause
	}
	if c.target != nil {
		return c.target, nil
	}
	return c.FunctionContext.LookupBuiltInFunction(name, args)
}

func (c *replacementContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.events = append(c.events, "metadata")
	c.reads++
	if c.failure == "metadata" || c.failure == "construction" && c.reads == 2 {
		return nil, c.cause
	}
	return c.FunctionContext.Metadata(handle)
}

func (*replacementContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("replacement construction must not evaluate")
}
func (c *replacementContext) reset() { c.events = nil; c.reads = 0; c.lookupTypes = nil }

func TestBuildGroupReplacement(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		arrayType := admissionMust(types.NewArrayType(integer, n))
		reference := admissionReference("array", arrayType)
		inputs := []expressions.Expression{reference, admissionLiteral(admissionMust(testbedvalues.NewArray(arrayType, nil)))}
		if n == foundation.MayNull {
			inputs = append(inputs, admissionLiteral(admissionMust(testbedvalues.NewNull(arrayType))))
		}
		for index, input := range inputs {
			t.Run(fmt.Sprintf("%v/input=%d", n, index), func(t *testing.T) {
				context := &replacementContext{FunctionContext: admissionMust(testbed.NewStandardContext())}
				first := groupingExclusion(context, input, 2)
				second := groupingExclusion(context, input, 1, 2)
				source := admissionLogical(foundation.And, first, second, first)
				before := expressions.PrettyPrint(source)
				group := groupExclusions(admissionMust(collectExclusions(source)))[0]
				var expectedValues []valueinterfaces.Value
				for _, member := range group.members {
					expectedValues = append(expectedValues, member.candidates...)
				}
				context.reset()
				replacement, present := admissionMust(buildGroupReplacement(testbedvalues.ValueFactory{}, group)).Get()
				if !present || !replacement.Type().EqualsTo(source.Type()) {
					t.Fatal("missing or incorrectly typed replacement")
				}
				not := replacement.(*expressions.LogicalExpression)
				call := not.Children()[0].(*expressions.CallExpression)
				args := call.Arguments()
				if not.Operator() != foundation.Not || call.Context() != context || args[0] != input {
					t.Fatal("negation, context or retained array changed")
				}
				literal := args[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue)
				elements := literal.Elements()
				if !slices.Equal(elements, expectedValues) {
					t.Fatal("candidate order, duplicates or Value identity changed")
				}
				var numbers []int64
				for _, item := range elements {
					numbers = append(numbers, item.(*testbedvalues.IntegerValue).Integer())
				}
				if !slices.Equal(numbers, []int64{2, 1, 2, 2}) || literal.Type().Nullability() != foundation.NonNull {
					t.Fatal("incorrect aggregate literal")
				}
				metadata := admissionMust(context.FunctionContext.Metadata(call.Handle()))
				if name, ok := metadata.BuiltInName().Get(); !ok || name != functions.BuiltInArrayContainsAny {
					t.Fatal("replacement is not contains-any")
				}
				if !slices.Equal(context.events, []string{"lookup", "metadata", "metadata"}) || !context.lookupTypes[0].EqualsTo(input.Type()) || !context.lookupTypes[1].EqualsTo(literal.Type()) {
					t.Fatal("query sequence or effective lookup Types changed")
				}
				// Mutating caller-owned record storage after construction cannot change
				// the published literal's copied container or the immutable source.
				group.members[1].candidates[0] = admissionMust(testbedvalues.NewInteger(integer, 99))
				elements[0] = admissionMust(testbedvalues.NewInteger(integer, 98))
				if !slices.Equal(literal.Elements(), expectedValues) || expressions.PrettyPrint(source) != before {
					t.Fatal("replacement aliases mutable source containers")
				}
			})
		}
	}
}

func TestGroupReplacementFailurePhasesAndRetry(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	array := admissionReference("array", admissionMust(types.NewArrayType(integer, foundation.MayNull)))
	sentinel := errors.New("replacement provider failure")
	for _, tc := range []struct {
		phase    string
		stronger error
		miss     bool
	}{
		{"lookup", nil, true},
		{"lookup", functions.ErrContractViolation, false},
		{"lookup", functions.ErrEvaluationUnavailable, false},
		{"metadata", nil, false},
		{"construction", nil, false},
	} {
		t.Run(fmt.Sprintf("%s/%v", tc.phase, tc.stronger), func(t *testing.T) {
			context := &replacementContext{FunctionContext: admissionMust(testbed.NewStandardContext())}
			source := admissionLogical(foundation.And, groupingExclusion(context, array, 1), groupingExclusion(context, array, 2))
			group := groupExclusions(admissionMust(collectExclusions(source)))[0]
			before := expressions.PrettyPrint(source)
			context.reset()
			context.failure, context.cause = tc.phase, fmt.Errorf("provider: %w", errors.Join(functions.ErrUnsupportedLookup, sentinel, tc.stronger))
			result, err := buildGroupReplacement(testbedvalues.ValueFactory{}, group)
			if _, present := result.Get(); present {
				t.Fatal("failure/miss published a candidate")
			}
			if tc.miss {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, sentinel) || !errors.Is(err, functions.ErrUnsupportedLookup) || tc.stronger != nil && !errors.Is(err, tc.stronger) {
				t.Fatalf("error category or cause lost: %v", err)
			}
			wantReads := 0
			if tc.phase == "metadata" {
				wantReads = 1
			}
			if tc.phase == "construction" {
				wantReads = 2
			}
			if context.reads != wantReads || expressions.PrettyPrint(source) != before {
				t.Fatal("wrong failure phase or mutated source")
			}
			context.failure = ""
			context.reset()
			if _, present := admissionMust(buildGroupReplacement(testbedvalues.ValueFactory{}, group)).Get(); !present {
				t.Fatal("retry failed")
			}
		})
	}
}

// A catalog can expose correctly formed metadata for an incompatible requested
// overload. Returning its handle deliberately bypasses normal lookup filtering.
func replacementCatalog(element, targetElement types.Type, targetName functions.BuiltInFunctionName) *testbed.Catalog {
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	array := admissionMust(types.NewArrayType(element, foundation.NonNull))
	targetArray := admissionMust(types.NewArrayType(targetElement, foundation.NonNull))
	var definitions []testbed.Definition
	for _, spec := range []struct {
		name    string
		builtin functions.BuiltInFunctionName
		args    []types.Type
	}{
		{"contains", functions.BuiltInArrayContains, []types.Type{array, element}},
		{"target", targetName, []types.Type{targetArray, targetArray}},
	} {
		metadata := admissionMust(functions.NewFunctionMetadata(admissionMust(functions.NewFunctionSignature(spec.args, boolean)), spec.name, foundation.Some(spec.builtin), foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}))
		definitions = append(definitions, testbed.Definition{Namespace: "replacement", Name: spec.name, Metadata: metadata})
	}
	return admissionMust(testbed.NewCatalog(definitions))
}

func TestGroupReplacementReturnedOverload(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	other := admissionMust(types.NewIntType(types.Int32, foundation.NonNull))
	input := admissionReference("array", admissionMust(types.NewArrayType(integer, foundation.NonNull)))
	for _, tc := range []struct {
		name      string
		element   types.Type
		family    functions.BuiltInFunctionName
		wantError bool
	}{
		{"unsupported family", integer, functions.BuiltInNot, false},
		{"wrong width", other, functions.BuiltInArrayContainsAny, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := replacementCatalog(integer, tc.element, tc.family)
			context := &replacementContext{FunctionContext: testbed.NewContext(catalog)}
			source := admissionLogical(foundation.And, groupingExclusion(context, input, 1), groupingExclusion(context, input, 2))
			group := groupExclusions(admissionMust(collectExclusions(source)))[0]
			context.target = catalog.Handles()[1]
			context.reset()
			result, err := buildGroupReplacement(testbedvalues.ValueFactory{}, group)
			if _, present := result.Get(); present {
				t.Fatal("bad target published")
			}
			if tc.wantError {
				if !errors.Is(err, functions.ErrUnsupportedLookup) || context.reads != 2 {
					t.Fatalf("incompatible overload should fail checked construction: %v", err)
				}
			} else if err != nil || context.reads != 1 {
				t.Fatal("unsupported target family should decline before binding")
			}
		})
	}
	// Foreign handles remain errors at target metadata, not unsupported lookup.
	provider := admissionMust(testbed.NewStandardContext())
	context := &replacementContext{FunctionContext: provider}
	source := admissionLogical(foundation.And, groupingExclusion(context, input, 1), groupingExclusion(context, input, 2))
	group := groupExclusions(admissionMust(collectExclusions(source)))[0]
	foreign := admissionMust(testbed.NewStandardContext())
	context.target = admissionMust(foreign.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{input.Type(), input.Type()}))
	result, err := buildGroupReplacement(testbedvalues.ValueFactory{}, group)
	if _, present := result.Get(); present || !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatalf("foreign target became candidate or local miss: %v", err)
	}
}

func TestBuildCompositeGroupReplacement(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int32, foundation.NonNull))
	element := admissionMust(types.NewArrayType(integer, foundation.NonNull))
	catalog := replacementCatalog(element, element, functions.BuiltInArrayContainsAny)
	context := &replacementContext{FunctionContext: testbed.NewContext(catalog)}
	input := admissionReference("nested", admissionMust(types.NewArrayType(element, foundation.MayNull)))
	value := admissionMust(testbedvalues.NewArray(element, nil))
	candidate := admissionLiteral(value)
	handle := catalog.Handles()[0]
	call := admissionMust(expressions.NewCallExpression(context, handle, []expressions.Expression{input, candidate}))
	exclusion := admissionLogical(foundation.Not, call)
	group := groupExclusions(admissionMust(collectExclusions(admissionLogical(foundation.And, exclusion, exclusion))))[0]
	context.reset()
	result, present := admissionMust(buildGroupReplacement(testbedvalues.ValueFactory{}, group)).Get()
	if !present || !result.Type().EqualsTo(exclusion.Type()) {
		t.Fatal("composite grouping lost exact Type")
	}
	actual := result.Children()[0].(*expressions.CallExpression).Arguments()[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue)
	if !slices.Equal(actual.Elements(), []valueinterfaces.Value{value, value}) || !actual.Type().(*types.ArrayType).Element().EqualsTo(element) {
		t.Fatal("composite candidates were flattened or copied as values")
	}
}
