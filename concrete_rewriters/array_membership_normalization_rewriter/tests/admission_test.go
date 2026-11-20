package array_membership_normalization_rewriter_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func admissionReference(typ types.Type, name string) expressions.Expression {
	return must(expressions.NewReferenceExpression(must(ids.NewColumnID("admission", name)), typ, name))
}

// Construction-only catalogs let these tests describe valid calls outside the
// admitted family. They establish rejection, not executable equivalence.
func TestSingletonShapeAdmission(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.NonNull)
	nullableArray := must(f.array.WithNullability(foundation.MayNull)).(*types.ArrayType)
	nullableElement := must(f.integer.WithNullability(foundation.MayNull)).(*types.IntType)
	arrayOfNullable := must(types.NewArrayType(nullableElement, foundation.NonNull))
	bound8, bound16 := uint64(8), uint64(16)
	varchar8 := must(types.NewVarCharType(&bound8, foundation.NonNull))
	varchar16 := must(types.NewVarCharType(&bound16, foundation.NonNull))
	array8 := must(types.NewArrayType(varchar8, foundation.NonNull))
	array16 := must(types.NewArrayType(varchar16, foundation.NonNull))
	nested8 := must(types.NewArrayType(array8, foundation.NonNull))
	nested16 := must(types.NewArrayType(array16, foundation.NonNull))
	literal := func(value valueinterfaces.Value) expressions.Expression {
		return must(expressions.NewLiteralExpression(value))
	}
	cases := []struct {
		name string
		args []expressions.Expression
	}{
		{"empty", []expressions.Expression{f.input, f.candidates()}},
		{"multiple", []expressions.Expression{f.input, f.candidates(1, 2)}},
		{"duplicates", []expressions.Expression{f.input, f.candidates(1, 1)}},
		{"dynamic", []expressions.Expression{f.input, admissionReference(f.array, "candidates")}},
		{"null container", []expressions.Expression{f.input, literal(must(testbedvalues.NewNull(nullableArray)))}},
		{"nullable declared container", []expressions.Expression{f.input, literal(f.arrayValue(nullableArray, 1))}},
		{"nullable declared element", []expressions.Expression{admissionReference(arrayOfNullable, "input"), literal(must(testbedvalues.NewArray(arrayOfNullable, []valueinterfaces.Value{must(testbedvalues.NewInteger(nullableElement, 1))})))}},
		{"null element", []expressions.Expression{admissionReference(arrayOfNullable, "input"), literal(must(testbedvalues.NewArray(arrayOfNullable, []valueinterfaces.Value{must(testbedvalues.NewNull(nullableElement))})))}},
		{"scalar input", []expressions.Expression{f.element(1), f.candidates(1)}},
		{"scalar candidate", []expressions.Expression{f.input, f.element(1)}},
		{"varchar constraints differ", []expressions.Expression{admissionReference(array8, "input"), literal(must(testbedvalues.NewArray(array16, []valueinterfaces.Value{must(testbedvalues.NewVarChar(varchar16, "tag"))})))}},
		{"nested constraints differ", []expressions.Expression{admissionReference(nested8, "input"), literal(must(testbedvalues.NewArray(nested16, []valueinterfaces.Value{must(testbedvalues.NewArray(array16, nil))})))}},
		{"one argument", []expressions.Expression{f.input}},
		{"three arguments", []expressions.Expression{f.input, f.candidates(1), f.element(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argumentTypes := make([]types.Type, len(tc.args))
			policies := make([]functions.NullPolicy, len(tc.args))
			for i, argument := range tc.args {
				argumentTypes[i] = argument.Type()
				if argument.Type().Nullability() == foundation.MayNull {
					policies[i] = functions.PassNull
				}
			}
			definition := membershipDefinition(functions.BuiltInArrayContainsAny, argumentTypes, must(types.NewBoolType(foundation.NonNull)), policies)
			catalog := must(testbed.NewCatalog([]testbed.Definition{definition}))
			context := &recordingContext{FunctionContext: testbed.NewContext(catalog)}
			source := must(expressions.NewCallExpression(context, catalog.Handles()[0], tc.args))
			assertAdmissionRetains(t, context, source, 0, 0)
		})
	}
}

// Each case starts with an admitted signature. Mutations are immutable metadata
// alternatives, never changes to metadata behind an already constructed call.
type admissionMetadata struct {
	arguments []types.Type
	result    types.Type
	name      foundation.Optional[functions.BuiltInFunctionName]
	operator  foundation.Optional[foundation.OperatorType]
	policies  []functions.NullPolicy
}

func (m admissionMetadata) definition(name functions.BuiltInFunctionName) testbed.Definition {
	metadata := must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(m.arguments, m.result)), string(name), m.name, m.operator, m.policies))
	// Even a misleading display name AND handle name cannot grant admission.
	return testbed.Definition{Namespace: "admission", Name: string(name), Metadata: metadata}
}

func TestMembershipMetadataAdmission(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.NonNull)
	boolean := must(types.NewBoolType(foundation.NonNull))
	cases := []struct {
		name   string
		mutate func(*admissionMetadata)
	}{
		{"missing builtin identity", func(m *admissionMetadata) { m.name = foundation.Optional[functions.BuiltInFunctionName]{} }},
		{"unknown builtin identity", func(m *admissionMetadata) {
			m.name = foundation.Some(functions.BuiltInFunctionName("custom.membership"))
		}},
		{"operator classification", func(m *admissionMetadata) { m.operator = foundation.Some(foundation.Equal) }},
		{"unspecified input policy", func(m *admissionMetadata) { m.policies[0] = functions.NullPolicyUnspecified }},
		{"pass null input policy", func(m *admissionMetadata) {
			m.arguments[0] = must(m.arguments[0].WithNullability(foundation.MayNull))
			m.policies[0] = functions.PassNull
		}},
		{"propagate null candidate policy", func(m *admissionMetadata) { m.policies[1] = functions.PropagateNull }},
		{"pass null candidate policy", func(m *admissionMetadata) {
			m.arguments[1] = must(m.arguments[1].WithNullability(foundation.MayNull))
			m.policies[1] = functions.PassNull
		}},
		{"nullable implementation input", func(m *admissionMetadata) { m.arguments[0] = must(m.arguments[0].WithNullability(foundation.MayNull)) }},
		{"nullable implementation candidate", func(m *admissionMetadata) { m.arguments[1] = must(m.arguments[1].WithNullability(foundation.MayNull)) }},
		{"nullable result", func(m *admissionMetadata) { m.result = must(boolean.WithNullability(foundation.MayNull)) }},
		{"nonboolean result", func(m *admissionMetadata) { m.result = f.integer }},
	}
	for _, side := range []string{"source", "target"} {
		for _, tc := range cases {
			t.Run(side+"/"+tc.name, func(t *testing.T) {
				sourceMetadata := admissionMetadata{[]types.Type{f.array, f.array}, boolean, foundation.Some(functions.BuiltInArrayContainsAny), foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}}
				targetMetadata := admissionMetadata{[]types.Type{f.array, f.integer}, boolean, foundation.Some(functions.BuiltInArrayContains), foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}}
				if side == "source" {
					tc.mutate(&sourceMetadata)
				} else {
					tc.mutate(&targetMetadata)
				}
				catalog := must(testbed.NewCatalog([]testbed.Definition{sourceMetadata.definition(functions.BuiltInArrayContainsAny), targetMetadata.definition(functions.BuiltInArrayContains)}))
				context := &recordingContext{FunctionContext: testbed.NewContext(catalog)}
				source := must(expressions.NewCallExpression(context, catalog.Handles()[0], []expressions.Expression{f.input, f.candidates(1)}))
				lookups, metadataQueries := 0, 1
				if side == "target" {
					lookups = 1
					if name, ok := targetMetadata.name.Get(); ok && name == functions.BuiltInArrayContains {
						metadataQueries++
					}
				}
				assertAdmissionRetains(t, context, source, lookups, metadataQueries)
			})
		}
	}
}

func TestTargetElementTypeAdmission(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.NonNull)
	boolean := must(types.NewBoolType(foundation.NonNull))
	otherInteger := must(types.NewIntType(types.Int32, foundation.NonNull))
	otherArray := must(types.NewArrayType(otherInteger, foundation.NonNull))
	for _, tc := range []struct {
		name string
		args []types.Type
	}{
		{"candidate width", []types.Type{f.array, otherInteger}},
		{"input element width", []types.Type{otherArray, f.integer}},
		{"scalar input", []types.Type{f.integer, f.integer}},
		{"array candidate", []types.Type{f.array, f.array}},
		{"arity", []types.Type{f.array}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policies := make([]functions.NullPolicy, len(tc.args))
			policies[0] = functions.PropagateNull
			sourceDefinition := membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{f.array, f.array}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified})
			targetDefinition := membershipDefinition(functions.BuiltInArrayContains, tc.args, boolean, policies)
			catalog := must(testbed.NewCatalog([]testbed.Definition{sourceDefinition, targetDefinition}))
			context := &recordingContext{FunctionContext: testbed.NewContext(catalog)}
			source := must(expressions.NewCallExpression(context, catalog.Handles()[0], []expressions.Expression{f.input, f.candidates(1)}))
			// Ordinary lookup rejects these incompatible signatures before target
			// metadata admission; no fabricated invalid call is needed.
			assertAdmissionRetains(t, context, source, 1, 1)
		})
	}
}

func assertAdmissionRetains(t *testing.T, context *recordingContext, source *expressions.CallExpression, wantLookups, wantMetadata int) {
	t.Helper()
	before := expressions.PrettyPrint(source)
	context.events = nil
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	got, err := tree.Rewrite(source, struct{}{})
	if err != nil || got != source || !got.Type().EqualsTo(source.Type()) || expressions.PrettyPrint(source) != before {
		t.Fatalf("outside-admission call changed: %v", err)
	}
	lookups, metadata := 0, 0
	for _, event := range context.events {
		switch event.operation {
		case "lookup":
			lookups++
		case "metadata":
			metadata++
		case "evaluate":
			t.Fatal("admission evaluated a function")
		}
	}
	if lookups != wantLookups || metadata != wantMetadata {
		t.Fatalf("lookup/metadata queries = %d/%d, want %d/%d", lookups, metadata, wantLookups, wantMetadata)
	}
}
