package array_exclusion_grouping_rewriter_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Record invocation of the provider boundary, including NULL-propagating calls.
// Metadata and overload lookups are deliberately outside the execution trace.
type executionContext struct {
	functioninterfaces.FunctionContext
	trace *[]string
}

func (c *executionContext) Evaluate(handle functioninterfaces.FunctionHandle, signature *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	*c.trace = append(*c.trace, "call:"+handle.Name())
	return c.FunctionContext.Evaluate(handle, signature, args)
}

type executionBinding struct {
	id    expressioninterfaces.ColumnID
	label string
	value valueinterfaces.Value
}

type executionEnvironment struct {
	trace    *[]string
	bindings []executionBinding
	reads    int
	// Only negative contract fixtures override the stable binding lookup.
	override func(int) (valueinterfaces.Value, error)
}

func (e *executionEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for _, binding := range e.bindings {
		if binding.id.EqualsTo(id) {
			*e.trace = append(*e.trace, "read:"+binding.label)
			e.reads++
			if e.override != nil {
				value, err := e.override(e.reads)
				if err != nil {
					return foundation.Optional[valueinterfaces.Value]{}, err
				}
				return foundation.Some(value), nil
			}
			return foundation.Some(binding.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, fmt.Errorf("unexpected reference: %s", id)
}

func assertExecutionTrace(t *testing.T, actual, want []string) {
	t.Helper()
	if !slices.Equal(actual, want) {
		t.Fatalf("trace=%v, want %v", actual, want)
	}
}

func rewriteWithoutExecution(t *testing.T, source expressions.Expression, trace *[]string) expressions.Expression {
	t.Helper()
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	*trace = nil
	result := must(boundaryTree().Rewrite(source, struct{}{}))
	assertExecutionTrace(t, *trace, nil)
	return result
}

func TestArrayRewriteDoesNotProbeExecution(t *testing.T) {
	var trace []string
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	// This provider has complete metadata but no executable implementation.
	context := &executionContext{FunctionContext: testbed.NewContext(membershipCatalog(integer)), trace: &trace}
	fixture := newMembershipFixture(context, integer, foundation.NonNull)
	one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
	source := fixtureLogical(foundation.And, fixtureExclude(fixture.containsAny(fixture.input, one)), fixtureExclude(fixture.containsAny(fixture.input, two)))
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	normalization := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	normalized := must(normalization.Rewrite(source, struct{}{}))
	assertExecutionTrace(t, trace, nil)
	result := rewriteWithoutExecution(t, normalized, &trace)
	if normalized == source || result == normalized {
		t.Fatal("expected both normalization and grouping")
	}
	assertExecutionTrace(t, trace, nil)
	// Rewriting success does not grant execution availability. It neither calls
	// Evaluate nor receives an Environment; later evaluation still reports failure.
	for _, node := range []expressions.Expression{source, normalized, result} {
		trace = nil
		env := &executionEnvironment{trace: &trace, bindings: []executionBinding{{fixture.input.ID(), "A", fixture.inputValue()}}}
		value, err := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(node, env)
		if value != nil || !errors.Is(err, functions.ErrEvaluationUnavailable) {
			t.Fatalf("missing implementation was hidden: %v", err)
		}
		family := "array_contains_any"
		if node == normalized {
			family = "array_contains"
		}
		assertExecutionTrace(t, trace, []string{"read:A", "call:" + family})
	}
}

func TestGroupingReducesAdmittedEvaluations(t *testing.T) {
	for _, literal := range []bool{false, true} {
		for _, kind := range []string{"empty", "hit", "NULL"} {
			t.Run(fmt.Sprintf("literal=%t/%s", literal, kind), func(t *testing.T) {
				var trace []string
				integer := must(types.NewIntType(types.Int64, foundation.NonNull))
				context := &executionContext{FunctionContext: must(testbed.NewStandardContext()), trace: &trace}
				n := foundation.NonNull
				if kind == "NULL" {
					n = foundation.MayNull
				}
				fixture := newMembershipFixture(context, integer, n)
				one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
				var input valueinterfaces.Value = fixture.inputValue()
				if kind == "hit" {
					input = fixture.inputValue(two)
				}
				if kind == "NULL" {
					input = must(testbedvalues.NewNull(fixture.input.Type()))
				}
				var operand expressions.Expression = fixture.input
				if literal {
					operand = fixtureLiteral(input)
				}
				source := fixtureLogical(foundation.And, fixtureExclude(fixture.contains(operand, one)), fixtureExclude(fixture.containsAny(operand, two, one)), fixtureExclude(fixture.contains(operand, two)))
				result := rewriteWithoutExecution(t, source, &trace)
				if result == source {
					t.Fatal("admitted group was not rewritten")
				}
				var original valueinterfaces.Value
				for i, node := range []expressions.Expression{source, result} {
					trace = nil
					env := &executionEnvironment{trace: &trace, bindings: []executionBinding{{fixture.input.ID(), "A", input}}}
					actual := must(interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(node, env))
					families := []string{"array_contains", "array_contains_any", "array_contains"}
					if i == 1 {
						families = []string{"array_contains_any"}
					}
					var want []string
					for _, family := range families {
						if !literal {
							want = append(want, "read:A")
						}
						want = append(want, "call:"+family)
					}
					assertExecutionTrace(t, trace, want)
					if i == 0 {
						original = actual
					} else if !actual.EqualsTo(original) || !actual.Type().EqualsTo(source.Type()) {
						t.Fatal("changed truth or Type")
					}
					if kind == "NULL" && !actual.IsNull() {
						t.Fatal("known NULL did not propagate")
					}
				}
			})
		}
	}
}

func executionCall(trace *[]string, name string, result types.Type, args []expressions.Expression, run func([]valueinterfaces.Value) (valueinterfaces.Value, error)) *expressions.CallExpression {
	var parameters []types.Type
	var policies []functions.NullPolicy
	for _, arg := range args {
		parameters = append(parameters, arg.Type())
		policy := functions.NullPolicyUnspecified
		if arg.Type().Nullability() == foundation.MayNull {
			policy = functions.PassNull
		}
		policies = append(policies, policy)
	}
	metadata := must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(parameters, result)), name, foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, policies))
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "execution", Name: name, Metadata: metadata}}))
	context := &executionContext{FunctionContext: must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: catalog.Handles()[0], Evaluate: run}})), trace: trace}
	return must(expressions.NewCallExpression(context, catalog.Handles()[0], args))
}

func TestUnrelatedExecutionOrderAndFirstFailure(t *testing.T) {
	var trace []string
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	context := &executionContext{FunctionContext: must(testbed.NewStandardContext()), trace: &trace}
	fixture := newMembershipFixture(context, integer, foundation.NonNull)
	other := fixtureReference("other", fixture.input.Type())
	one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
	names := []string{"first", "second", "third", "outer"}
	failures := make(map[string]error)
	makeCall := func(name string, args ...expressions.Expression) *expressions.CallExpression {
		return executionCall(&trace, name, boolean, args, func(actual []valueinterfaces.Value) (valueinterfaces.Value, error) {
			if err := failures[name]; err != nil {
				return nil, err
			}
			if len(actual) != 0 {
				return actual[0], nil
			}
			// A decisive FALSE must not hide subsequent unrelated failures.
			return testbedvalues.NewBool(boolean, false)
		})
	}
	first, second, third := makeCall("first"), makeCall("second"), makeCall("third")
	region := fixtureLogical(foundation.And, fixtureExclude(fixture.contains(fixture.input, one)), first, fixtureExclude(fixture.contains(other, one)), fixtureExclude(fixture.contains(fixture.input, two)), second, fixtureExclude(fixture.containsAny(other, two)), third)
	source := makeCall("outer", region)
	result := rewriteWithoutExecution(t, source, &trace)
	if result == source {
		t.Fatal("multiple groups were not rewritten")
	}
	sourceTrace := []string{"read:A", "call:array_contains", "call:first", "read:B", "call:array_contains", "read:A", "call:array_contains", "call:second", "read:B", "call:array_contains_any", "call:third", "call:outer"}
	resultTrace := []string{"read:A", "call:array_contains_any", "call:first", "read:B", "call:array_contains_any", "call:second", "call:third", "call:outer"}
	for failed := -1; failed < len(names); failed++ {
		clear(failures)
		if failed >= 0 {
			for _, name := range names[failed:] {
				failures[name] = errors.New(name + " failed")
			}
		}
		for i, node := range []expressions.Expression{source, result} {
			trace = nil
			env := &executionEnvironment{trace: &trace, bindings: []executionBinding{{fixture.input.ID(), "A", fixture.inputValue()}, {other.ID(), "B", fixture.inputValue()}}}
			value, err := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(node, env)
			want := sourceTrace
			if i == 1 {
				want = resultTrace
			}
			if failed >= 0 {
				if value != nil || !errors.Is(err, failures[names[failed]]) {
					t.Fatalf("first failure changed: %v", err)
				}
				for _, name := range names[failed+1:] {
					if errors.Is(err, failures[name]) {
						t.Fatalf("later error escaped: %v", err)
					}
				}
				want = want[:slices.Index(want, "call:"+names[failed])+1]
			} else if err != nil || value.IsNull() || value.(*testbedvalues.BoolValue).Bool() {
				t.Fatalf("unexpected successful result: %v %v", value, err)
			}
			assertExecutionTrace(t, trace, want)
		}
	}
}

func TestComputedAndOpaqueArraysRemainUngrouped(t *testing.T) {
	for _, shared := range []bool{false, true} {
		for _, failAt := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("shared=%t/failAt=%d", shared, failAt), func(t *testing.T) {
				var trace []string
				integer := must(types.NewIntType(types.Int64, foundation.NonNull))
				context := &executionContext{FunctionContext: must(testbed.NewStandardContext()), trace: &trace}
				fixture := newMembershipFixture(context, integer, foundation.NonNull)
				one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
				calls := 0
				sentinel := errors.New("array producer failed")
				producer := executionCall(&trace, "produce", fixture.array, nil, func([]valueinterfaces.Value) (valueinterfaces.Value, error) {
					calls++
					if calls == failAt {
						return nil, sentinel
					}
					if calls == 1 {
						return fixture.inputValue(two), nil
					}
					return fixture.inputValue(one), nil
				})
				second := producer
				if !shared {
					second = must(expressions.NewCallExpression(producer.Context(), producer.Handle(), nil))
					if second == producer || !second.EqualsTo(producer) {
						t.Fatal("expected distinct equal producer nodes")
					}
				}
				source := fixtureLogical(foundation.And, fixtureExclude(fixture.contains(producer, one)), fixtureExclude(fixture.contains(second, two)))
				result := rewriteWithoutExecution(t, source, &trace)
				if result != source || calls != 0 {
					t.Fatal("computed array was grouped or executed")
				}
				for _, node := range []expressions.Expression{source, result} {
					trace, calls = nil, 0
					actual, err := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(node, &executionEnvironment{trace: &trace})
					want := []string{"call:produce", "call:array_contains", "call:produce", "call:array_contains"}
					if failAt != 0 {
						if actual != nil || !errors.Is(err, sentinel) {
							t.Fatalf("producer failure hidden: %v", err)
						}
						want = want[:2*failAt-1]
					} else if err != nil || !actual.(*testbedvalues.BoolValue).Bool() {
						t.Fatalf("changing producer executions were merged: %v %v", actual, err)
					}
					assertExecutionTrace(t, trace, want)
				}
			})
		}
	}
	t.Run("opaque array", func(t *testing.T) {
		var trace []string
		integer := must(types.NewIntType(types.Int64, foundation.NonNull))
		context := &executionContext{FunctionContext: must(testbed.NewStandardContext()), trace: &trace}
		fixture := newMembershipFixture(context, integer, foundation.NonNull)
		opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("execution", "array")), fixture.array, "opaque array"))
		one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
		source := fixtureLogical(foundation.And, fixtureExclude(fixture.contains(opaque, one)), fixtureExclude(fixture.contains(opaque, two)))
		result := rewriteWithoutExecution(t, source, &trace)
		if result != source {
			t.Fatal("opaque array grouped")
		}
		for _, node := range []expressions.Expression{source, result} {
			actual, err := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(node, &executionEnvironment{trace: &trace})
			if actual != nil || !errors.Is(err, interpreter.ErrEvaluationUnavailable) {
				t.Fatalf("opaque failure hidden: %v", err)
			}
			assertExecutionTrace(t, trace, nil)
		}
	})
}

func TestUnstableReferenceViolatesGroupingContract(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprintf("second lookup fails=%t", failure), func(t *testing.T) {
			var trace []string
			integer := must(types.NewIntType(types.Int64, foundation.NonNull))
			context := &executionContext{FunctionContext: must(testbed.NewStandardContext()), trace: &trace}
			fixture := newMembershipFixture(context, integer, foundation.NonNull)
			one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
			source := fixtureLogical(foundation.And, fixtureExclude(fixture.contains(fixture.input, one)), fixtureExclude(fixture.contains(fixture.input, two)))
			result := rewriteWithoutExecution(t, source, &trace)
			sentinel := errors.New("second reference lookup failed")
			for i, node := range []expressions.Expression{source, result} {
				trace = nil
				env := &executionEnvironment{trace: &trace, bindings: []executionBinding{{fixture.input.ID(), "A", fixture.inputValue()}}}
				env.override = func(read int) (valueinterfaces.Value, error) {
					if failure {
						if read == 2 {
							return nil, sentinel
						}
						return fixture.inputValue(), nil
					}
					if read == 1 {
						return fixture.inputValue(two), nil
					}
					return fixture.inputValue(one), nil
				}
				actual, err := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(node, env)
				if failure && i == 0 {
					if actual != nil || !errors.Is(err, sentinel) {
						t.Fatalf("expected source failure: %v", err)
					}
					assertExecutionTrace(t, trace, []string{"read:A", "call:array_contains", "read:A"})
				} else {
					// Changing bindings: source TRUE, grouped FALSE. Failing bindings: source
					// errors, grouped TRUE. Both are deliberate violations, not valid rewrites.
					want := i == 0 || failure
					if err != nil || actual.IsNull() || actual.(*testbedvalues.BoolValue).Bool() != want {
						t.Fatalf("counterexample did not expose contract violation: %v %v", actual, err)
					}
					expected := []string{"read:A", "call:array_contains", "read:A", "call:array_contains"}
					if i == 1 {
						expected = []string{"read:A", "call:array_contains_any"}
					}
					assertExecutionTrace(t, trace, expected)
				}
			}
		})
	}
}
