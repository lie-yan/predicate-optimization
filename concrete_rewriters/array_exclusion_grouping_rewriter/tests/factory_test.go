package array_exclusion_grouping_rewriter_test

import (
	"errors"
	"testing"

	grouping "github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

type externalArrayFactory struct{ testbedvalues.ValueFactory }

func (f externalArrayFactory) NewArray(typ *types.ArrayType, children []valueinterfaces.Value) (valueinterfaces.Value, error) {
	value, err := f.ValueFactory.NewArray(typ, children)
	if err != nil {
		return nil, err
	}
	return fixtures.WrapArray(value.(valueinterfaces.ArrayValue)), nil
}

func TestGroupingExternalArraysAndFactory(t *testing.T) {
	context := must(testbed.NewStandardContext())
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	for _, nullable := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		f := newMembershipFixture(context, integer, nullable)
		one := fixtures.WrapScalar(must(testbedvalues.NewInteger(integer, 1)))
		two := must(testbedvalues.NewInteger(integer, 2))
		three := fixtures.WrapScalar(must(testbedvalues.NewInteger(integer, 3)))
		a := fixtureLiteral(fixtures.WrapArray(must(testbedvalues.NewArray(f.array, []valueinterfaces.Value{one, two}))))
		b := fixtureLiteral(fixtures.WrapArray(must(testbedvalues.NewArray(f.array, []valueinterfaces.Value{two, three}))))
		source := fixtureLogical(foundation.And, fixtureExclude(f.call(functions.BuiltInArrayContainsAny, f.input, a)), fixtureExclude(f.call(functions.BuiltInArrayContainsAny, f.input, b)))
		before := expressions.PrettyPrint(source)
		factory := &fixtures.RecordingFactory{Delegate: externalArrayFactory{}}
		tree := rewriter.NewExpressionTreeRewriter[struct{}](grouping.NewArrayExclusionGroupingRewriter(factory))
		result := must(tree.Rewrite(source, struct{}{}))
		if len(factory.Requests) != 1 || factory.Requests[0].Kind != "Array" || !factory.Requests[0].Type.EqualsTo(f.array) {
			t.Fatal("wrong factory request")
		}
		want := []valueinterfaces.Value{one, two, two, three}
		payload := factory.Requests[0].Payload.([]valueinterfaces.Value)
		output := result.(*expressions.LogicalExpression).Children()[0].(*expressions.CallExpression).Arguments()[1].(*expressions.LiteralExpression).Value()
		array, ok := output.(fixtures.ArrayView)
		if !ok || !result.Type().EqualsTo(source.Type()) || len(payload) != 4 || len(array.Elements()) != 4 {
			t.Fatal("lost external output, type, or duplicates")
		}
		for i, v := range array.Elements() {
			if !v.EqualsTo(want[i]) || !payload[i].EqualsTo(want[i]) {
				t.Fatal("candidate order changed")
			}
		}
		if _, ok := array.Elements()[0].(fixtures.ScalarView); !ok {
			t.Fatal("child representation changed")
		}
		evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
		inputs := []valueinterfaces.Value{fixtures.WrapArray(f.inputValue()), fixtures.WrapArray(f.inputValue(three)), fixtures.WrapArray(f.inputValue(must(testbedvalues.NewInteger(integer, 9))))}
		if nullable == foundation.MayNull {
			inputs = append(inputs, fixtures.WrapScalar(must(testbedvalues.NewNull(f.input.Type()))))
		}
		for _, input := range inputs {
			env := &fixtureEnvironment{bindings: []fixtureBinding{{f.input.ID(), input}}}
			if !must(evaluator.Evaluate(source, env)).EqualsTo(must(evaluator.Evaluate(result, env))) {
				t.Fatal("replacement changed semantics")
			}
		}
		factory.Failure = errors.New("array construction failed")
		if failed, err := tree.Rewrite(source, struct{}{}); failed != nil || !errors.Is(err, factory.Failure) {
			t.Fatal("array failure hidden or partial result published")
		}
		if expressions.PrettyPrint(source) != before {
			t.Fatal("source changed")
		}
	}
	if assertions.Enabled {
		for _, factory := range []valueinterfaces.ValueFactory{nil, (*testbedvalues.ValueFactory)(nil)} {
			func() {
				defer func() {
					if recover() == nil {
						t.Error("nil factory accepted")
					}
				}()
				grouping.NewArrayExclusionGroupingRewriter(factory)
			}()
		}
	}
}

func TestGroupingExternalNestedCandidates(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	element := must(types.NewArrayType(integer, foundation.NonNull))
	context := testbed.NewContext(membershipCatalog(element))
	f := newMembershipFixture(context, element, foundation.NonNull)
	empty := fixtures.WrapArray(must(testbedvalues.NewArray(element, nil)))
	nested := fixtures.WrapArray(must(testbedvalues.NewArray(element, []valueinterfaces.Value{fixtures.WrapScalar(must(testbedvalues.NewInteger(integer, 1)))})))
	// An empty array of element Type is a single contains candidate, not an
	// empty contains-any list. The outer list retains both nested children.
	outer := fixtureLiteral(fixtures.WrapArray(must(testbedvalues.NewArray(f.array, []valueinterfaces.Value{nested, empty}))))
	source := fixtureLogical(foundation.And, fixtureExclude(f.contains(f.input, empty)), fixtureExclude(f.call(functions.BuiltInArrayContainsAny, f.input, outer)))
	factory := &fixtures.RecordingFactory{Delegate: externalArrayFactory{}}
	result := must(rewriter.NewExpressionTreeRewriter[struct{}](grouping.NewArrayExclusionGroupingRewriter(factory)).Rewrite(source, struct{}{}))
	if result == source || len(factory.Requests) != 1 {
		t.Fatal("nested candidates not grouped")
	}
	children := factory.Requests[0].Payload.([]valueinterfaces.Value)
	if len(children) != 3 || !children[0].EqualsTo(empty) || !children[1].EqualsTo(nested) || !children[2].EqualsTo(empty) {
		t.Fatal("nested candidates flattened or deduplicated")
	}
}
