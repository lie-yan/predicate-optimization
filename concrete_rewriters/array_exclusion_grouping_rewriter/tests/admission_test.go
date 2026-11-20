package array_exclusion_grouping_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// verifyAdmissionMiss establishes validity before observing the rule. Source
// recognition may query metadata, but cannot re-resolve or execute stored calls.
func verifyAdmissionMiss(t *testing.T, node expressions.Expression, context *recordingContext, metadataQueries int) {
	t.Helper()
	source := fixtureLogical(foundation.And, node, node)
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	context.events = nil
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{}))
	if got := must(tree.Rewrite(source, struct{}{})); got != source {
		t.Fatalf("unsupported expression changed:\n%s", expressions.PrettyPrint(got))
	}
	if len(context.events) != metadataQueries {
		t.Fatalf("events=%v, want %d metadata queries", context.events, metadataQueries)
	}
	for _, event := range context.events {
		if event.operation != "metadata" {
			t.Fatalf("unexpected provider operation: %s", event.operation)
		}
	}

	// An unrelated supported group still succeeds, retaining each missed node.
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	supportedContext := &recordingContext{FunctionContext: must(testbed.NewStandardContext())}
	f := newMembershipFixture(supportedContext, integer, foundation.NonNull)
	input := fixtureReference("supported", f.input.Type())
	value := must(testbedvalues.NewInteger(integer, 9))
	exclusion := fixtureExclude(f.contains(input, value))
	mixed := fixtureLogical(foundation.And, node, exclusion, node, exclusion)
	expected := fixtureLogical(foundation.And, node, f.expectedGroup(input, value, value), node)
	result := verifyPublishedGroup(t, mixed, expected, []*recordingContext{context, supportedContext}, 1)
	if result.Children()[0] != node || result.Children()[2] != node {
		t.Fatal("unsupported operands were rebuilt")
	}
}

func TestAdmissionCandidateAndWrapperShapes(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	nullableInteger := must(types.NewIntType(types.Int64, foundation.MayNull))
	value := must(testbedvalues.NewInteger(integer, 1))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	nullableArray := must(types.NewArrayType(integer, foundation.MayNull))
	nullableElements := must(types.NewArrayType(nullableInteger, foundation.NonNull))
	input := fixtureLiteral(must(testbedvalues.NewArray(array, []valueinterfaces.Value{value})))
	cases := []struct {
		name      string
		operand   expressions.Expression
		candidate expressions.Expression
		family    functions.BuiltInFunctionName
	}{
		{"empty list", input, fixtureLiteral(must(testbedvalues.NewArray(array, nil))), functions.BuiltInArrayContainsAny},
		{"dynamic list", input, fixtureReference("candidates", array), functions.BuiltInArrayContainsAny},
		{"dynamic scalar", input, fixtureReference("candidate", integer), functions.BuiltInArrayContains},
		{"NULL list", input, fixtureLiteral(must(testbedvalues.NewNull(nullableArray))), functions.BuiltInArrayContainsAny},
		{"nullable list", input, fixtureLiteral(must(testbedvalues.NewArray(nullableArray, []valueinterfaces.Value{value}))), functions.BuiltInArrayContainsAny},
		{"NULL scalar", input, fixtureLiteral(must(testbedvalues.NewNull(nullableInteger))), functions.BuiltInArrayContains},
		{"nullable scalar", input, fixtureLiteral(must(testbedvalues.NewInteger(nullableInteger, 1))), functions.BuiltInArrayContains},
		{"nullable candidate elements", input, fixtureLiteral(must(testbedvalues.NewArray(nullableElements, []valueinterfaces.Value{must(testbedvalues.NewInteger(nullableInteger, 1))}))), functions.BuiltInArrayContainsAny},
		{"NULL candidate element", input, fixtureLiteral(must(testbedvalues.NewArray(nullableElements, []valueinterfaces.Value{must(testbedvalues.NewNull(nullableInteger))}))), functions.BuiltInArrayContainsAny},
		{"nullable input elements", fixtureLiteral(must(testbedvalues.NewArray(nullableElements, nil))), fixtureLiteral(value), functions.BuiltInArrayContains},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// These are valid provider overloads, even when outside this rule's family.
			policies := []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
			if tc.candidate.Type().Nullability() == foundation.MayNull {
				policies[1] = functions.PassNull
			}
			definition := membershipDefinition(tc.family, []types.Type{tc.operand.Type(), tc.candidate.Type()}, must(types.NewBoolType(foundation.NonNull)), policies)
			catalog := must(testbed.NewCatalog([]testbed.Definition{definition}))
			context := &recordingContext{FunctionContext: testbed.NewContext(catalog)}
			call := must(expressions.NewCallExpression(context, catalog.Handles()[0], []expressions.Expression{tc.operand, tc.candidate}))
			verifyAdmissionMiss(t, fixtureExclude(call), context, 0)
		})
	}
	context := &recordingContext{FunctionContext: must(testbed.NewStandardContext())}
	f := newMembershipFixture(context, integer, foundation.NonNull)
	call := f.contains(input, value)
	t.Run("positive predicate", func(t *testing.T) { verifyAdmissionMiss(t, call, context, 0) })
	notHandle := must(context.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{call.Type()}))
	callNot := must(expressions.NewCallExpression(context, notHandle, []expressions.Expression{call}))
	t.Run("call form NOT", func(t *testing.T) { verifyAdmissionMiss(t, callNot, context, 0) })
	t.Run("OR boundary", func(t *testing.T) {
		verifyAdmissionMiss(t, fixtureLogical(foundation.Or, fixtureExclude(call), fixtureExclude(call)), context, 0)
	})

	// A call returning an array is a valid membership input but is not a stable leaf.
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "array", Metadata: must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(nil, array)), "array", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, nil))}}))
	arrayCall := must(expressions.NewCallExpression(testbed.NewContext(catalog), catalog.Handles()[0], nil))
	t.Run("computed array", func(t *testing.T) { verifyAdmissionMiss(t, fixtureExclude(f.contains(arrayCall, value)), context, 0) })
}

func TestAdmissionMetadataAndFullElementTypes(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	input := fixtureLiteral(must(testbedvalues.NewArray(array, nil)))
	candidate := fixtureLiteral(must(testbedvalues.NewInteger(integer, 1)))
	for _, name := range []string{"misleading name", "other family", "operator classification", "first policy", "second policy", "nullable result", "non-Boolean result"} {
		t.Run(name, func(t *testing.T) {
			builtIn := foundation.Some(functions.BuiltInArrayContains)
			operator := foundation.Optional[foundation.OperatorType]{}
			policies := []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
			var result types.Type = boolean
			switch name {
			case "misleading name":
				builtIn = foundation.Optional[functions.BuiltInFunctionName]{}
			case "other family":
				builtIn = foundation.Some(functions.BuiltInArrayContainsAny)
			case "operator classification":
				operator = foundation.Some(foundation.Equal)
			case "first policy":
				policies[0] = functions.NullPolicyUnspecified
			case "second policy":
				policies[1] = functions.PropagateNull
			case "nullable result":
				result = must(boolean.WithNullability(foundation.MayNull))
			case "non-Boolean result":
				result = integer
			}
			metadata := must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature([]types.Type{array, integer}, result)), "array_contains", builtIn, operator, policies))
			catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "array_contains", Metadata: metadata}}))
			context := &recordingContext{FunctionContext: testbed.NewContext(catalog)}
			call := must(expressions.NewCallExpression(context, catalog.Handles()[0], []expressions.Expression{input, candidate}))
			if name == "non-Boolean result" {
				// NOT of an integer is invalid at construction; a valid IN wrapper keeps
				// the non-Boolean call in the graph without fabricating a malformed predicate.
				if _, err := expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{call}); err == nil {
					t.Fatal("non-Boolean NOT accepted")
				}
				predicate := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{call, candidate}))
				verifyAdmissionMiss(t, fixtureLogical(foundation.Not, predicate), context, 0)
			} else {
				verifyAdmissionMiss(t, fixtureExclude(call), context, 2)
			}
		})
	}
	bound8, bound16 := uint64(8), uint64(16)
	varchar8, varchar16 := must(types.NewVarCharType(&bound8, foundation.NonNull)), must(types.NewVarCharType(&bound16, foundation.NonNull))
	int32Type := must(types.NewIntType(types.Int32, foundation.NonNull))
	nested8, nested16 := must(types.NewArrayType(varchar8, foundation.NonNull)), must(types.NewArrayType(varchar16, foundation.NonNull))
	for _, tc := range []struct {
		name    string
		element types.Type
		value   valueinterfaces.Value
	}{
		{"integer width", integer, must(testbedvalues.NewInteger(int32Type, 1))},
		{"VARCHAR bound", varchar8, must(testbedvalues.NewVarChar(varchar16, "tag"))},
		{"nested bound", nested8, must(testbedvalues.NewArray(nested16, nil))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputType := must(types.NewArrayType(tc.element, foundation.NonNull))
			input := fixtureLiteral(must(testbedvalues.NewArray(inputType, nil)))
			definition := membershipDefinition(functions.BuiltInArrayContains, []types.Type{inputType, tc.value.Type()}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified})
			catalog := must(testbed.NewCatalog([]testbed.Definition{definition}))
			context := &recordingContext{FunctionContext: testbed.NewContext(catalog)}
			call := must(expressions.NewCallExpression(context, catalog.Handles()[0], []expressions.Expression{input, fixtureLiteral(tc.value)}))
			verifyAdmissionMiss(t, fixtureExclude(call), context, 0)
		})
	}
}

func TestAdmissionGenericElements(t *testing.T) {
	for _, element := range fixtureElements() {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			t.Run(fmt.Sprintf("%s/%v", element.name, n), func(t *testing.T) {
				// Metadata-only fixtures deliberately have no executable implementation.
				context := &recordingContext{FunctionContext: testbed.NewContext(membershipCatalog(element.candidate.Type()))}
				f := newMembershipFixture(context, element.candidate.Type(), n)
				source := fixtureLogical(foundation.And, fixtureExclude(f.contains(f.input, element.candidate)), fixtureExclude(f.containsAny(f.input, element.candidate, element.candidate)))
				verifyPublishedGroup(t, source, f.expectedGroup(f.input, element.candidate, element.candidate, element.candidate), []*recordingContext{context}, 1)
			})
		}
	}
}
