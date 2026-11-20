package array_exclusion_grouping_rewriter

import (
	"errors"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// These private-stage tests use valid metadata-only catalogs to verify admission,
// not executable equivalence. Public grouping tests cover complete rewrites.
func admissionMust[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type admissionContext struct {
	functioninterfaces.FunctionContext
	reads int
	fault error
}

func (c *admissionContext) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.reads++
	if c.fault != nil {
		return nil, c.fault
	}
	return c.FunctionContext.Metadata(h)
}

func (*admissionContext) LookupBuiltInFunction(functions.BuiltInFunctionName, []types.Type) (functioninterfaces.FunctionHandle, error) {
	panic("recognition must not resolve another function")
}

func (*admissionContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("recognition must not evaluate")
}

func admissionLiteral(v valueinterfaces.Value) *expressions.LiteralExpression {
	return admissionMust(expressions.NewLiteralExpression(v))
}

func admissionReference(name string, typ types.Type) *expressions.ReferenceExpression {
	return admissionMust(expressions.NewReferenceExpression(admissionMust(ids.NewColumnID("admission", name)), typ, name))
}

func admissionLogical(op foundation.LogicalOperator, args ...expressions.Expression) *expressions.LogicalExpression {
	return admissionMust(expressions.NewLogicalExpression(op, args))
}

// A named membership gets the admitted signature; an unnamed custom call uses
// its exact argument Types and can represent valid but unsupported source shapes.
func admissionCall(name functions.BuiltInFunctionName, result types.Type, args ...expressions.Expression) (*expressions.CallExpression, *admissionContext) {
	parameters := make([]types.Type, len(args))
	policies := make([]functions.NullPolicy, len(args))
	for i, arg := range args {
		parameters[i] = arg.Type()
		if name != "" {
			parameters[i] = admissionMust(arg.Type().WithNullability(foundation.NonNull))
			if i == 0 {
				policies[i] = functions.PropagateNull
			}
		} else if arg.Type().Nullability() == foundation.MayNull {
			policies[i] = functions.PassNull
		}
	}
	var builtin foundation.Optional[functions.BuiltInFunctionName]
	if name != "" {
		builtin = foundation.Some(name)
	}
	metadata := admissionMust(functions.NewFunctionMetadata(admissionMust(functions.NewFunctionSignature(parameters, result)), "membership", builtin, foundation.Optional[foundation.OperatorType]{}, policies))
	catalog := admissionMust(testbed.NewCatalog([]testbed.Definition{{Namespace: "admission", Name: "membership", Metadata: metadata}}))
	context := &admissionContext{FunctionContext: testbed.NewContext(catalog)}
	call := admissionMust(expressions.NewCallExpression(context, catalog.Handles()[0], args))
	context.reads = 0
	return call, context
}

func TestCollectExclusionOccurrences(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	array := admissionMust(types.NewArrayType(integer, foundation.NonNull))
	one, two := admissionMust(testbedvalues.NewInteger(integer, 1)), admissionMust(testbedvalues.NewInteger(integer, 2))
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		t.Run(n.Suffix(), func(t *testing.T) {
			inputType := admissionMust(array.WithNullability(n))
			reference := admissionReference("array", inputType)
			inputs := []expressions.Expression{reference, admissionLiteral(admissionMust(testbedvalues.NewArray(inputType.(*types.ArrayType), []valueinterfaces.Value{one})))}
			if n == foundation.MayNull {
				inputs = append(inputs, admissionLiteral(admissionMust(testbedvalues.NewNull(inputType))))
			}
			for _, input := range inputs {
				literal := admissionLiteral(admissionMust(testbedvalues.NewArray(array, []valueinterfaces.Value{two, one, two})))
				scalar, scalarContext := admissionCall(functions.BuiltInArrayContains, boolean, input, admissionLiteral(one))
				any, anyContext := admissionCall(functions.BuiltInArrayContainsAny, boolean, input, literal)
				first, second := admissionLogical(foundation.Not, scalar), admissionLogical(foundation.Not, any)
				nested := admissionLogical(foundation.And, first, second)
				group := admissionLogical(foundation.And, admissionLiteral(admissionMust(testbedvalues.NewBool(boolean, true))), first, nested, second, first)
				before := expressions.PrettyPrint(group)
				occurrences := admissionMust(collectExclusions(group))
				if len(occurrences) != 3 || scalarContext.reads != 2 || anyContext.reads != 1 {
					t.Fatal("direct occurrence collection/query count differs")
				}
				for i, index := range []int{1, 3, 4} {
					got := occurrences[i]
					wantCall, wantContext, wantExclusion, wantValues := scalar, scalarContext, first, []valueinterfaces.Value{one}
					if index == 3 {
						wantCall, wantContext, wantExclusion, wantValues = any, anyContext, second, []valueinterfaces.Value{two, one, two}
					}
					if got.index != index || got.exclusion != wantExclusion || got.call != wantCall || got.array != input || got.context != wantContext || !slices.Equal(got.candidates, wantValues) {
						t.Fatal("occurrence lost provenance, ordered Values or paired context")
					}
				}
				occurrences[1].candidates[0] = one
				if literal.Value().(*testbedvalues.ArrayValue).Elements()[0] != two || expressions.PrettyPrint(group) != before {
					t.Fatal("analysis mutated source storage")
				}
				scalarContext.reads, anyContext.reads = 0, 0
				for _, other := range []*expressions.LogicalExpression{admissionLogical(foundation.Or, first, second), admissionLogical(foundation.Not, nested)} {
					if got := admissionMust(collectExclusions(other)); len(got) != 0 {
						t.Fatal("non-AND region collected exclusions")
					}
				}
				if scalarContext.reads != 0 || anyContext.reads != 0 {
					t.Fatal("collector descended into unsupported region")
				}
			}
		})
	}
}

func TestExclusionShapeMissesAvoidMetadata(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	array := admissionMust(types.NewArrayType(integer, foundation.NonNull))
	nullableArray := admissionMust(array.WithNullability(foundation.MayNull)).(*types.ArrayType)
	nullableElement := admissionMust(integer.WithNullability(foundation.MayNull))
	arrayOfNullable := admissionMust(types.NewArrayType(nullableElement, foundation.NonNull))
	input := admissionReference("array", array)
	one := admissionLiteral(admissionMust(testbedvalues.NewInteger(integer, 1)))
	opaque := admissionMust(expressions.NewOpaqueExpression(admissionMust(ids.NewOpaqueID("admission", "array")), array, "opaque"))
	producer, producerContext := admissionCall("", array)
	cases := []struct {
		name string
		args []expressions.Expression
	}{
		{"arity zero", nil},
		{"arity one", []expressions.Expression{input}},
		{"arity three", []expressions.Expression{input, one, one}},
		{"scalar input", []expressions.Expression{one, one}},
		{"producer", []expressions.Expression{producer, one}},
		{"opaque", []expressions.Expression{opaque, one}},
		{"dynamic", []expressions.Expression{input, admissionReference("candidate", integer)}},
		{"empty list", []expressions.Expression{input, admissionLiteral(admissionMust(testbedvalues.NewArray(array, nil)))}},
		{"NULL list", []expressions.Expression{input, admissionLiteral(admissionMust(testbedvalues.NewNull(nullableArray)))}},
		{"nullable list", []expressions.Expression{input, admissionLiteral(admissionMust(testbedvalues.NewArray(nullableArray, []valueinterfaces.Value{one.Value()})))}},
		{"nullable element", []expressions.Expression{admissionReference("nullable elements", arrayOfNullable), one}},
		{"NULL candidate", []expressions.Expression{input, admissionLiteral(admissionMust(testbedvalues.NewNull(nullableElement)))}},
		{"nullable scalar candidate", []expressions.Expression{input, admissionLiteral(admissionMust(testbedvalues.NewInteger(nullableElement.(*types.IntType), 1)))}},
		{"different list element", []expressions.Expression{input, admissionLiteral(admissionMust(testbedvalues.NewArray(admissionMust(types.NewArrayType(boolean, foundation.NonNull)), []valueinterfaces.Value{admissionMust(testbedvalues.NewBool(boolean, true))})))}},
		{"different element", []expressions.Expression{input, admissionLiteral(admissionMust(testbedvalues.NewBool(boolean, true)))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call, context := admissionCall("", boolean, tc.args...)
			if got, err := recognizeExclusion(7, admissionLogical(foundation.Not, call)); err != nil {
				t.Fatal(err)
			} else if _, present := got.Get(); present {
				t.Fatal("unsupported shape admitted")
			}
			if context.reads != 0 || producerContext.reads != 0 {
				t.Fatal("shape miss queried metadata")
			}
		})
	}
	call, context := admissionCall(functions.BuiltInArrayContains, boolean, input, one)
	for _, expression := range []expressions.Expression{call, admissionLogical(foundation.Not, admissionLiteral(admissionMust(testbedvalues.NewBool(boolean, true)))), admissionLogical(foundation.Or, call, call)} {
		if _, present := admissionMust(recognizeExclusion(0, expression)).Get(); present {
			t.Fatal("unsupported exclusion wrapper admitted")
		}
	}
	if context.reads != 0 {
		t.Fatal("wrapper shape miss queried metadata")
	}
}

func TestCompositeCandidateAndMetadataBoundary(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int32, foundation.NonNull))
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	element := admissionMust(types.NewArrayType(integer, foundation.NonNull))
	array := admissionMust(types.NewArrayType(element, foundation.NonNull))
	input := admissionReference("nested", array)
	emptyElement := admissionMust(testbedvalues.NewArray(element, nil))
	for _, name := range []functions.BuiltInFunctionName{functions.BuiltInArrayContains, functions.BuiltInArrayContainsAny} {
		literal := admissionLiteral(emptyElement)
		if name == functions.BuiltInArrayContainsAny {
			literal = admissionLiteral(admissionMust(testbedvalues.NewArray(array, []valueinterfaces.Value{emptyElement})))
		}
		call, context := admissionCall(name, boolean, input, literal)
		exclusion := admissionLogical(foundation.Not, call)
		occurrence, present := admissionMust(recognizeExclusion(0, exclusion)).Get()
		if !present || len(occurrence.candidates) != 1 || occurrence.candidates[0] != emptyElement || context.reads != 1 {
			t.Fatal("composite scalar/list candidate confused")
		}
		sentinel := errors.New("source metadata unavailable")
		context.fault = errors.Join(functions.ErrUnsupportedLookup, sentinel)
		good, goodContext := admissionCall(name, boolean, input, literal)
		got, err := collectExclusions(admissionLogical(foundation.And, admissionLogical(foundation.Not, good), exclusion))
		if goodContext.reads != 1 || got != nil || !errors.Is(err, sentinel) || !errors.Is(err, functions.ErrUnsupportedLookup) {
			t.Fatal("metadata error became miss or partial output")
		}
		context.fault = nil
		if _, present := admissionMust(recognizeExclusion(0, exclusion)).Get(); !present {
			t.Fatal("retry failed")
		}
	}
	for _, name := range []functions.BuiltInFunctionName{"", functions.BuiltInArrayContainsAny} {
		custom, context := admissionCall(name, boolean, input, admissionLiteral(emptyElement))
		if _, present := admissionMust(recognizeExclusion(0, admissionLogical(foundation.Not, custom))).Get(); present || context.reads != 1 {
			t.Fatal("valid custom or mismatched-family metadata admitted as contains")
		}
	}
}
