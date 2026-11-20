package array_exclusion_grouping_rewriter_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The interface field makes static struct comparability insufficient: its
// dynamic payload can be non-comparable. Keep these contexts stored by value.
type comparableContext struct {
	functioninterfaces.FunctionContext
	identity int
}

func verifyUnchangedPair(t *testing.T, source expressions.Expression, recorders ...*recordingContext) {
	t.Helper()
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	for _, recorder := range recorders {
		recorder.events = nil
	}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{}))
	if result := must(tree.Rewrite(source, struct{}{})); result != source {
		t.Fatalf("distinct/unsupported operands combined:\n%s", expressions.PrettyPrint(result))
	}
	count := 0
	for _, recorder := range recorders {
		for _, event := range recorder.events {
			if event.operation != "metadata" {
				t.Fatalf("pairing made unexpected %s query", event.operation)
			}
			count++
		}
	}
	if count != 2 {
		t.Fatalf("metadata queries=%d, want one per occurrence", count)
	}
}

func TestContextPartitioning(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	value := must(testbedvalues.NewInteger(integer, 1))
	for _, name := range []string{"equal comparable values", "different comparable values", "shared pointer", "distinct pointers sharing provider", "non-comparable context", "interface containing non-comparable context", "non-comparable handles"} {
		t.Run(name, func(t *testing.T) {
			recorder := &recordingContext{FunctionContext: must(testbed.NewStandardContext())}
			var left, right functioninterfaces.FunctionContext = recorder, recorder
			group := true
			switch name {
			case "equal comparable values":
				left, right = comparableContext{recorder, 1}, comparableContext{recorder, 1}
			case "different comparable values":
				left, right, group = comparableContext{recorder, 1}, comparableContext{recorder, 2}, false
			case "distinct pointers sharing provider":
				left, right, group = &comparableContext{recorder, 1}, &comparableContext{recorder, 1}, false
			case "non-comparable context":
				left = nonComparableContext{FunctionContext: recorder, marker: []byte{1}}
				right, group = left, false
			case "interface containing non-comparable context":
				left = comparableContext{nonComparableContext{FunctionContext: recorder, marker: []byte{1}}, 1}
				right, group = left, false
				if !reflect.TypeOf(left).Comparable() || reflect.ValueOf(left).Comparable() {
					t.Fatal("fixture must distinguish static/dynamic comparability")
				}
			case "non-comparable handles":
				left = &wrappedContext{delegate: recorder}
				right = left
			}
			a, b := newMembershipFixture(left, integer, foundation.MayNull), newMembershipFixture(right, integer, foundation.MayNull)
			first, second := a.contains(a.input, value), b.contains(b.input, value)
			if !first.Handle().EqualsTo(second.Handle()) {
				t.Fatal("fixture must share handle identity")
			}
			if name == "non-comparable handles" && reflect.ValueOf(first.Handle()).Comparable() {
				t.Fatal("fixture handle is comparable")
			}
			source := fixtureLogical(foundation.And, fixtureExclude(first), fixtureExclude(second))
			if group {
				result := verifyPublishedGroup(t, source, a.expectedGroup(a.input, value, value), []*recordingContext{recorder}, 1)
				if result.Children()[0].(*expressions.CallExpression).Context() != left {
					t.Fatal("replacement lost anchor context")
				}
			} else {
				verifyUnchangedPair(t, source, recorder)
			}
		})
	}
}

func TestDistinctContextsSharingCatalogDoNotCombine(t *testing.T) {
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	catalog := membershipCatalog(text)
	exact := &recordingContext{FunctionContext: executableMembershipContext(catalog, fixtureElementEqual)}
	folded := &recordingContext{FunctionContext: executableMembershipContext(catalog, func(a, b valueinterfaces.Value) bool {
		return strings.EqualFold(a.(*testbedvalues.VarCharValue).VarChar(), b.(*testbedvalues.VarCharValue).VarChar())
	})}
	a, b := newMembershipFixture(exact, text, foundation.NonNull), newMembershipFixture(folded, text, foundation.NonNull)
	candidate := must(testbedvalues.NewVarChar(text, "tag"))
	first, second := a.contains(a.input, candidate), b.contains(b.input, candidate)
	if !first.Handle().EqualsTo(second.Handle()) || must(exact.Metadata(first.Handle())) != must(folded.Metadata(second.Handle())) {
		t.Fatal("expected shared handles and metadata")
	}
	source := fixtureLogical(foundation.And, fixtureExclude(first), fixtureExclude(second))
	verifyUnchangedPair(t, source, exact, folded)
	// Merging into the first provider would yield TRUE for TAG, whereas the
	// original is FALSE because the second provider uses case-insensitive equality.
	wrong := a.expectedGroup(a.input, candidate, candidate)
	environment := &fixtureEnvironment{bindings: []fixtureBinding{{a.input.ID(), a.inputValue(must(testbedvalues.NewVarChar(text, "TAG")))}}}
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	if must(evaluator.Evaluate(source, environment)).(*testbedvalues.BoolValue).Bool() || !must(evaluator.Evaluate(wrong, environment)).(*testbedvalues.BoolValue).Bool() {
		t.Fatal("fixture did not expose cross-context semantic difference")
	}
}

func TestArrayOperandIdentity(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	value := must(testbedvalues.NewInteger(integer, 1))
	for _, name := range []string{"column aliases", "different columns same label", "shared literal", "distinct literals shared value", "shared NULL literal", "empty array-valued element"} {
		t.Run(name, func(t *testing.T) {
			element := types.Type(integer)
			item := valueinterfaces.Value(value)
			if name == "empty array-valued element" {
				element = must(types.NewArrayType(integer, foundation.NonNull))
				item = must(testbedvalues.NewArray(element.(*types.ArrayType), nil))
			}
			context := &recordingContext{FunctionContext: testbed.NewContext(membershipCatalog(element))}
			f := newMembershipFixture(context, element, foundation.MayNull)
			var first, second expressions.Expression = f.input, f.input
			group := true
			switch name {
			case "column aliases":
				second = must(expressions.NewReferenceExpression(f.input.ID(), f.input.Type(), "different label"))
			case "different columns same label":
				other := fixtureReference("other", f.input.Type())
				second = must(expressions.NewReferenceExpression(other.ID(), other.Type(), f.input.DisplayName()))
				group = false
			case "shared literal", "distinct literals shared value":
				payload := f.inputValue(item)
				first = fixtureLiteral(payload)
				second = first
				if name == "distinct literals shared value" {
					second = fixtureLiteral(payload)
					group = false
				}
			case "shared NULL literal":
				first = fixtureLiteral(must(testbedvalues.NewNull(f.input.Type())))
				second = first
			}
			source := fixtureLogical(foundation.And, fixtureExclude(f.contains(first, item)), fixtureExclude(f.contains(second, item)))
			if group {
				verifyPublishedGroup(t, source, f.expectedGroup(first, item, item), []*recordingContext{context}, 1)
			} else {
				verifyUnchangedPair(t, source, context)
			}
		})
	}
}
