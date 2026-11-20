package array_exclusion_grouping_rewriter_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type observingGrouping struct {
	*array_exclusion_grouping_rewriter.ArrayExclusionGroupingRewriter
	references int
}

func (r *observingGrouping) RewriteReference(_ *expressions.ReferenceExpression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	r.references++
	return foundation.Optional[expressions.Expression]{}, nil
}

func referenceOccurrences(node expressions.Expression) int {
	count := 0
	if _, ok := node.(*expressions.ReferenceExpression); ok {
		count++
	}
	for _, child := range node.Children() {
		count += referenceOccurrences(child)
	}
	return count
}

func verifyPublishedGroup(t *testing.T, source, expected expressions.Expression, contexts []*recordingContext, wantLookups int) expressions.Expression {
	t.Helper()
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	before := expressions.PrettyPrint(source)
	children := source.Children()
	saved := slices.Clone(children)
	for _, context := range contexts {
		context.events = nil
	}
	observer := &observingGrouping{ArrayExclusionGroupingRewriter: array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{})}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](observer)
	result, err := tree.Rewrite(source, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.EqualsTo(expected) || !result.Type().EqualsTo(source.Type()) {
		t.Fatalf("unexpected grouped tree\n%s\nwant\n%s", expressions.PrettyPrint(result), expressions.PrettyPrint(expected))
	}
	if observer.references != referenceOccurrences(source) {
		t.Fatal("descendants skipped or traversed twice")
	}
	if reconstruction.MeasureExpression(result).NodeCount >= reconstruction.MeasureExpression(source).NodeCount {
		t.Fatal("accepted groups did not reduce node occurrences")
	}
	lookups := 0
	for _, context := range contexts {
		for _, event := range context.events {
			if event.operation == "evaluate" {
				t.Fatal("grouping evaluated input")
			}
			if event.operation == "lookup" {
				lookups++
				if event.name != functions.BuiltInArrayContainsAny {
					t.Fatal("wrong replacement family lookup")
				}
			}
		}
	}
	if lookups != wantLookups {
		t.Fatalf("lookups=%d, want %d", lookups, wantLookups)
	}
	if before != expressions.PrettyPrint(source) || !slices.Equal(children, saved) || !slices.Equal(source.Children(), saved) {
		t.Fatal("input changed")
	}
	if err := utils.ValidateForOptimization(result); err != nil {
		t.Fatal(err)
	}
	observer.references = 0
	if again := must(tree.Rewrite(result, struct{}{})); again != result {
		t.Fatal("second pass did not preserve identity")
	}
	if observer.references != referenceOccurrences(result) {
		t.Fatal("second pass repeated descendant traversal")
	}
	return result
}

func TestPublishOrderedGroups(t *testing.T) {
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/wrapped=%t", n, wrapped), func(t *testing.T) {
				var provider functioninterfaces.FunctionContext = must(testbed.NewStandardContext())
				if wrapped {
					provider = &wrappedContext{delegate: provider}
				}
				context := &recordingContext{FunctionContext: provider}
				integer := must(types.NewIntType(types.Int64, foundation.NonNull))
				f := newMembershipFixture(context, integer, n)
				v := func(n int64) valueinterfaces.Value { return must(testbedvalues.NewInteger(integer, n)) }
				first := fixtureExclude(f.contains(f.input, v(2)))
				second := fixtureExclude(f.containsAny(f.input, v(1), v(2)))
				grouped := f.expectedGroup(f.input, v(2), v(1), v(2))
				pair := fixtureLogical(foundation.And, first, second)
				t.Run("sole survivor", func(t *testing.T) { verifyPublishedGroup(t, pair, grouped, []*recordingContext{context}, 1) })
				t.Run("repeated occurrences", func(t *testing.T) {
					verifyPublishedGroup(t, fixtureLogical(foundation.And, first, second, first), f.expectedGroup(f.input, v(2), v(1), v(2), v(2)), []*recordingContext{context}, 1)
				})
				other := fixtureReference("other", f.input.Type())
				flag := fixtureReference("flag", must(types.NewBoolType(foundation.MayNull)))
				otherFirst, otherSecond := fixtureExclude(f.contains(other, v(9))), fixtureExclude(f.contains(other, v(7)))
				t.Run("interleaved groups", func(t *testing.T) {
					source := fixtureLogical(foundation.And, first, otherFirst, flag, second, otherSecond)
					expected := fixtureLogical(foundation.And, grouped, f.expectedGroup(other, v(9), v(7)), flag)
					result := verifyPublishedGroup(t, source, expected, []*recordingContext{context}, 2)
					if result.Children()[2] != flag {
						t.Fatal("unrelated operand instance changed")
					}
				})
				t.Run("rebuilt region without local group", func(t *testing.T) {
					verifyPublishedGroup(t, fixtureLogical(foundation.And, flag, pair), fixtureLogical(foundation.And, flag, grouped), []*recordingContext{context}, 1)
				})
				t.Run("shared groups beneath OR", func(t *testing.T) {
					verifyPublishedGroup(t, fixtureLogical(foundation.Or, pair, pair), fixtureLogical(foundation.Or, grouped, grouped), []*recordingContext{context}, 2)
				})
				t.Run("enclosing call", func(t *testing.T) {
					handle := must(context.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{pair.Type()}))
					source := must(expressions.NewCallExpression(context, handle, []expressions.Expression{pair}))
					expected := must(expressions.NewCallExpression(context, handle, []expressions.Expression{grouped}))
					verifyPublishedGroup(t, source, expected, []*recordingContext{context}, 1)
				})
			})
		}
	}
}

func TestPublishIndependentGroupsWithMissOrFailure(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	contexts := []*recordingContext{{FunctionContext: must(testbed.NewStandardContext())}, {FunctionContext: must(testbed.NewStandardContext())}}
	a, b := newMembershipFixture(contexts[0], integer, foundation.MayNull), newMembershipFixture(contexts[1], integer, foundation.MayNull)
	v := func(n int64) valueinterfaces.Value { return must(testbedvalues.NewInteger(integer, n)) }
	a1, a2 := fixtureExclude(a.contains(a.input, v(1))), fixtureExclude(a.contains(a.input, v(2)))
	b1, b2 := fixtureExclude(b.contains(b.input, v(3))), fixtureExclude(b.contains(b.input, v(4)))
	source := fixtureLogical(foundation.And, a1, b1, a2, b2)
	expected := fixtureLogical(foundation.And, a.expectedGroup(a.input, v(1), v(2)), b1, b2)
	contexts[1].fault = func(event functionEvent) error {
		if event.operation == "lookup" {
			return functions.ErrUnsupportedLookup
		}
		return nil
	}
	// The successful group's first position is retained; every missed member stays.
	verifyPublishedGroup(t, source, expected, contexts, 2)
	sentinel := errors.New("second group failed")
	contexts[1].fault = func(event functionEvent) error {
		if event.operation == "lookup" {
			return errors.Join(functions.ErrUnsupportedLookup, functions.ErrContractViolation, sentinel)
		}
		return nil
	}
	before := expressions.PrettyPrint(source)
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{}))
	result, err := tree.Rewrite(source, struct{}{})
	if result != nil || !errors.Is(err, sentinel) || !errors.Is(err, functions.ErrContractViolation) || expressions.PrettyPrint(source) != before {
		t.Fatalf("partial tree escaped or failure was hidden: %v", err)
	}
	contexts[1].fault = nil
	expected = fixtureLogical(foundation.And, a.expectedGroup(a.input, v(1), v(2)), b.expectedGroup(b.input, v(3), v(4)))
	verifyPublishedGroup(t, source, expected, contexts, 2)
}
