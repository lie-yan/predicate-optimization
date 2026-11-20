package array_exclusion_grouping_rewriter_test

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type elementFixture struct {
	name                string
	candidate, matching valueinterfaces.Value
	matches             bool
}

func fixtureElements() []elementFixture {
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int32, foundation.NonNull))
	bound := uint64(16)
	varchar := must(types.NewVarCharType(&bound, foundation.NonNull))
	float32Type := must(types.NewFloatType(types.Float32, foundation.NonNull))
	float64Type := must(types.NewFloatType(types.Float64, foundation.NonNull))
	nested := must(types.NewArrayType(integer, foundation.NonNull))
	structure := must(types.NewStructType([]types.StructField{{Name: "tag", Type: varchar}}, foundation.NonNull))
	var result []elementFixture
	add := func(name string, value valueinterfaces.Value) {
		result = append(result, elementFixture{name, value, value, true})
	}
	add("bool", must(testbedvalues.NewBool(boolean, true)))
	add("varchar", must(testbedvalues.NewVarChar(varchar, "tag")))
	add("float32", must(testbedvalues.NewFloat(float32Type, 1.5)))
	add("float64", must(testbedvalues.NewFloat(float64Type, 1.5)))
	add("NaN", must(testbedvalues.NewFloat(float64Type, math.NaN())))
	result[len(result)-1].matches = false
	add("signed zero", must(testbedvalues.NewFloat(float64Type, math.Copysign(0, -1))))
	result[len(result)-1].matching = must(testbedvalues.NewFloat(float64Type, 0))
	add("nested array", must(testbedvalues.NewArray(nested, []valueinterfaces.Value{must(testbedvalues.NewInteger(integer, 8))})))
	add("struct", must(testbedvalues.NewStruct(structure, []valueinterfaces.Value{must(testbedvalues.NewVarChar(varchar, "tag"))})))
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ := must(types.NewIntType(width, foundation.NonNull))
		add(typ.String(), must(testbedvalues.NewInteger(typ, 8)))
	}
	return result
}

func TestPairedMembershipFixtures(t *testing.T) {
	for _, element := range fixtureElements() {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			for _, representation := range []string{"ordinary", "non-comparable handle", "non-comparable context"} {
				t.Run(fmt.Sprintf("%s/%v/%s", element.name, n, representation), func(t *testing.T) {
					var provider functioninterfaces.FunctionContext = executableMembershipContext(membershipCatalog(element.candidate.Type()), fixtureElementEqual)
					if integer, ok := element.candidate.Type().(*types.IntType); ok && integer.IntegerWidth() == types.Int64 {
						provider = must(testbed.NewStandardContext())
					}
					switch representation {
					case "non-comparable handle":
						provider = &wrappedContext{delegate: provider}
					case "non-comparable context":
						provider = nonComparableContext{FunctionContext: provider, marker: []byte{1}}
					}
					f := newMembershipFixture(provider, element.candidate.Type(), n)
					one := f.contains(f.input, element.candidate)
					any := f.containsAny(f.input, element.candidate, element.candidate)
					source := fixtureLogical(foundation.And, fixtureExclude(one), fixtureExclude(any))
					expected := f.expectedGroup(f.input, element.candidate, element.candidate, element.candidate)
					if reflect.ValueOf(provider).Comparable() != (representation != "non-comparable context") {
						t.Fatal("fixture context has wrong representation")
					}
					if reflect.ValueOf(one.Handle()).Comparable() != (representation != "non-comparable handle") {
						t.Fatal("fixture handle has wrong representation")
					}
					for _, node := range []expressions.Expression{source, expected} {
						if err := utils.ValidateForOptimization(node); err != nil {
							t.Fatal(err)
						}
					}
					rows := []struct {
						value valueinterfaces.Value
						match bool
					}{{f.inputValue(), false}, {f.inputValue(element.matching, element.matching), element.matches}}
					if n == foundation.MayNull {
						rows = append(rows, struct {
							value valueinterfaces.Value
							match bool
						}{must(testbedvalues.NewNull(f.input.Type())), false})
					}
					evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
					for _, row := range rows {
						for _, node := range []expressions.Expression{one, any, source, expected} {
							environment := &fixtureEnvironment{bindings: []fixtureBinding{{f.input.ID(), row.value}}}
							actual := must(evaluator.Evaluate(node, environment))
							wantBool := row.match
							if node == source || node == expected {
								wantBool = !row.match
							}
							var want valueinterfaces.Value = must(testbedvalues.NewBool(must(types.NewBoolType(n)), wantBool))
							if row.value.IsNull() {
								want = must(testbedvalues.NewNull(want.Type()))
							}
							if !actual.EqualsTo(want) || !actual.Type().EqualsTo(want.Type()) {
								t.Fatal("paired membership fixture differs from independent expected truth/Type")
							}
						}
					}
					for _, node := range []expressions.Expression{source, expected} {
						missing := must(evaluator.PartialEvaluate(node, &fixtureEnvironment{}))
						if residual, present := missing.Residual(); !present || !residual.Type().EqualsTo(node.Type()) {
							t.Fatal("missing binding was not residual")
						}
						if n == foundation.MayNull {
							result := must(evaluator.PartialEvaluate(node, &fixtureEnvironment{bindings: []fixtureBinding{{f.input.ID(), must(testbedvalues.NewNull(f.input.Type()))}}}))
							if value, known := result.Known(); !known || !value.IsNull() || !value.Type().EqualsTo(node.Type()) {
								t.Fatal("known NULL was not preserved")
							}
						}
					}
				})
			}
		}
	}
}

func TestSharedCatalogDistinctExecutionContexts(t *testing.T) {
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	catalog := membershipCatalog(text)
	exact := executableMembershipContext(catalog, fixtureElementEqual)
	folded := executableMembershipContext(catalog, func(a, b valueinterfaces.Value) bool {
		return strings.EqualFold(a.(*testbedvalues.VarCharValue).VarChar(), b.(*testbedvalues.VarCharValue).VarChar())
	})
	if exact == folded {
		t.Fatal("execution contexts must differ")
	}
	a, b := newMembershipFixture(exact, text, foundation.NonNull), newMembershipFixture(folded, text, foundation.NonNull)
	candidate := must(testbedvalues.NewVarChar(text, "tag"))
	for _, name := range []functions.BuiltInFunctionName{functions.BuiltInArrayContains, functions.BuiltInArrayContainsAny} {
		arg := fixtureLiteral(candidate)
		if name == functions.BuiltInArrayContainsAny {
			arg = a.candidates(candidate)
		}
		left, right := a.call(name, a.input, arg), b.call(name, b.input, arg)
		if !left.Handle().EqualsTo(right.Handle()) || must(exact.Metadata(right.Handle())) != must(folded.Metadata(left.Handle())) {
			t.Fatal("shared-catalog fixture should accept the same handles and metadata")
		}
		input := a.inputValue(must(testbedvalues.NewVarChar(text, "TAG")))
		evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
		x := must(evaluator.Evaluate(left, &fixtureEnvironment{bindings: []fixtureBinding{{a.input.ID(), input}}}))
		y := must(evaluator.Evaluate(right, &fixtureEnvironment{bindings: []fixtureBinding{{b.input.ID(), input}}}))
		if x.(*testbedvalues.BoolValue).Bool() || !y.(*testbedvalues.BoolValue).Bool() {
			t.Fatal("metadata/handle identity concealed distinct execution bindings")
		}
	}
}

func TestFixtureFaultsAndPairing(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	recorder := &recordingContext{FunctionContext: must(testbed.NewStandardContext())}
	f := newMembershipFixture(recorder, integer, foundation.MayNull)
	call := f.contains(f.input, must(testbedvalues.NewInteger(integer, 8)))
	metadata := must(recorder.Metadata(call.Handle()))
	input := f.inputValue(must(testbedvalues.NewInteger(integer, 8)))
	sentinel := errors.New("fixture fault")
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, operation := range []string{"lookup", "metadata", "evaluate"} {
		recorder.events = nil
		recorder.fault = func(event functionEvent) error {
			if event.operation == operation {
				return fmt.Errorf("injected %s: %w", operation, sentinel)
			}
			return nil
		}
		var err error
		switch operation {
		case "lookup":
			_, err = recorder.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.input.Type(), integer})
		case "metadata":
			_, err = recorder.Metadata(call.Handle())
		case "evaluate":
			_, err = evaluator.Evaluate(call, &fixtureEnvironment{bindings: []fixtureBinding{{f.input.ID(), input}}})
		}
		if !errors.Is(err, sentinel) || len(recorder.events) != 1 || recorder.events[0].operation != operation {
			t.Fatalf("fault at %s was not recorded/preserved: %v", operation, err)
		}
		recorder.fault = nil
		if must(recorder.Metadata(call.Handle())) != metadata {
			t.Fatal("fault changed successful metadata")
		}
		if !must(evaluator.Evaluate(call, &fixtureEnvironment{bindings: []fixtureBinding{{f.input.ID(), input}}})).(*testbedvalues.BoolValue).Bool() {
			t.Fatal("retry did not recover")
		}
	}
	env := &fixtureEnvironment{failure: sentinel}
	if _, err := evaluator.PartialEvaluate(call, env); !errors.Is(err, sentinel) || len(env.lookups) != 1 {
		t.Fatal("environment failure became missing input")
	}
	first := &wrappedContext{delegate: recorder.FunctionContext}
	second := &wrappedContext{delegate: recorder.FunctionContext}
	h := must(first.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.input.Type(), integer}))
	if _, err := second.Metadata(h); !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("foreign wrapper handle was accepted")
	}
	if _, err := second.Evaluate(h, call.Signature(), []valueinterfaces.Value{input, must(testbedvalues.NewInteger(integer, 8))}); !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("foreign wrapper handle executed")
	}
	if !h.EqualsTo(must(first.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.input.Type(), integer}))) {
		t.Fatal("repeated wrapped lookup lost semantic identity")
	}
}
