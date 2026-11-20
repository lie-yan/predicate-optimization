package array_exclusion_grouping_rewriter_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

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
)

func boundaryOperations(events []functionEvent) []string {
	operations := make([]string, len(events))
	for i, event := range events {
		operations[i] = event.operation
	}
	return operations
}

func boundaryTree() rewriter.ExpressionTreeRewriter[struct{}] {
	return rewriter.NewExpressionTreeRewriter[struct{}](array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{}))
}

func boundaryPair(context functioninterfaces.FunctionContext, n foundation.Nullability) (expressions.Expression, expressions.Expression) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	f := newMembershipFixture(context, integer, n)
	one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
	source := fixtureLogical(foundation.And, fixtureExclude(f.contains(f.input, one)), fixtureExclude(f.contains(f.input, two)))
	return source, f.expectedGroup(f.input, one, two)
}

func TestProviderFailurePhasesAndRetry(t *testing.T) {
	sentinel := errors.New("provider detail")
	type failure struct {
		name            string
		cause           error
		categories      []error
		unsupportedOnly bool
	}
	failures := []failure{
		{"unsupported", functions.ErrUnsupportedLookup, []error{functions.ErrUnsupportedLookup}, true},
		{"wrapped unsupported", fmt.Errorf("provider: %w", functions.ErrUnsupportedLookup), []error{functions.ErrUnsupportedLookup}, true},
		{"nested joined unsupported", fmt.Errorf("outer: %w", errors.Join(sentinel, fmt.Errorf("inner: %w", functions.ErrUnsupportedLookup))), []error{sentinel, functions.ErrUnsupportedLookup}, true},
		{"uncategorized", sentinel, []error{sentinel}, false},
	}
	for _, category := range []error{functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation, functions.ErrEvaluationFailure, functions.ErrEvaluationUnavailable, functions.ErrEvaluationDisallowed} {
		failures = append(failures,
			failure{"wrapped " + category.Error(), fmt.Errorf("provider: %w", category), []error{category}, false},
			failure{"joined " + category.Error(), fmt.Errorf("provider: %w", errors.Join(functions.ErrUnsupportedLookup, fmt.Errorf("nested: %w", errors.Join(sentinel, category)))), []error{functions.ErrUnsupportedLookup, sentinel, category}, false},
		)
	}
	// Two source reads precede lookup, target recognition and checked binding.
	// A sixth query rebinds an enclosing call after its child has been grouped.
	operations := []string{"metadata", "metadata", "lookup", "metadata", "metadata", "metadata"}
	for _, phase := range []struct {
		name  string
		index int
	}{{"source metadata", 1}, {"target lookup", 2}, {"target metadata", 3}, {"checked construction", 4}, {"enclosing reconstruction", 5}} {
		for _, failure := range failures {
			for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				t.Run(fmt.Sprintf("%s/%s/%v", phase.name, failure.name, n), func(t *testing.T) {
					context := &recordingContext{FunctionContext: must(testbed.NewStandardContext())}
					source, expected := boundaryPair(context, n)
					if phase.index == 5 {
						handle := must(context.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{source.Type()}))
						source = must(expressions.NewCallExpression(context, handle, []expressions.Expression{source}))
						expected = must(expressions.NewCallExpression(context, handle, []expressions.Expression{expected}))
					}
					if err := utils.ValidateForOptimization(source); err != nil {
						t.Fatal(err)
					}
					before, children := expressions.PrettyPrint(source), source.Children()
					context.events = nil
					context.fault = func(functionEvent) error {
						if len(context.events) == phase.index+1 {
							return failure.cause
						}
						return nil
					}
					tree := boundaryTree()
					result, err := tree.Rewrite(source, struct{}{})
					if phase.index == 2 && failure.unsupportedOnly {
						if err != nil || result != source {
							t.Fatalf("unsupported lookup did not retain region: %v", err)
						}
					} else {
						if result != nil || err == nil {
							t.Fatalf("failure became a result: %v, %v", result, err)
						}
						for _, category := range failure.categories {
							if !errors.Is(err, category) {
								t.Fatalf("lost cause %v: %v", category, err)
							}
						}
					}
					if !slices.Equal(boundaryOperations(context.events), operations[:phase.index+1]) {
						t.Fatalf("wrong failure phase: %v", context.events)
					}
					if expressions.PrettyPrint(source) != before || !slices.Equal(source.Children(), children) {
						t.Fatal("failed attempt changed input")
					}
					context.fault, context.events = nil, nil
					retry := must(tree.Rewrite(source, struct{}{}))
					if !retry.EqualsTo(expected) || !retry.Type().EqualsTo(source.Type()) {
						t.Fatal("retry lost expected tree/Type")
					}
					count := 5
					if phase.index == 5 {
						count = 6
					}
					if !slices.Equal(boundaryOperations(context.events), operations[:count]) {
						t.Fatalf("retry queried/executed unexpectedly: %v", context.events)
					}
					if err := utils.ValidateForOptimization(retry); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

// Deliberately return another immutable handle without changing the source's
// metadata. This simulates a lookup contract failure, not invalid input.
type returnedTargetContext struct {
	functioninterfaces.FunctionContext
	// Correct target used during fixture construction and retry.
	normal functioninterfaces.FunctionHandle
	// Optional override for the explicit contains-any lookup.
	target functioninterfaces.FunctionHandle
}

func (c *returnedTargetContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	if name == functions.BuiltInArrayContainsAny {
		if c.target != nil {
			return c.target, nil
		}
		return c.normal, nil
	}
	return c.FunctionContext.LookupBuiltInFunction(name, args)
}

func TestReturnedTargetBoundaries(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	policies := []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
	for _, name := range []string{"foreign handle", "wrong overload", "nullable target result", "unsupported target family"} {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			t.Run(fmt.Sprintf("%s/%v", name, n), func(t *testing.T) {
				targetArray := array
				var resultType types.Type = boolean
				family := functions.BuiltInArrayContainsAny
				switch name {
				case "wrong overload":
					targetArray = must(types.NewArrayType(must(types.NewIntType(types.Int32, foundation.NonNull)), foundation.NonNull))
				case "nullable target result":
					resultType = must(boolean.WithNullability(foundation.MayNull))
				case "unsupported target family":
					family = functions.BuiltInNot
				}
				alternative := membershipDefinition(family, []types.Type{targetArray, targetArray}, resultType, policies)
				alternative.Name = "alternative"
				catalog := must(testbed.NewCatalog([]testbed.Definition{
					membershipDefinition(functions.BuiltInArrayContains, []types.Type{array, integer}, boolean, policies),
					membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{array, array}, boolean, policies),
					alternative,
				}))
				provider := &returnedTargetContext{FunctionContext: testbed.NewContext(catalog), normal: catalog.Handles()[1]}
				context := &recordingContext{FunctionContext: provider}
				source, expected := boundaryPair(context, n)
				if err := utils.ValidateForOptimization(source); err != nil {
					t.Fatal(err)
				}
				provider.target = catalog.Handles()[2]
				if name == "foreign handle" {
					provider.target = membershipCatalog(integer).Handles()[1]
				}
				before := expressions.PrettyPrint(source)
				context.events = nil
				tree := boundaryTree()
				result, err := tree.Rewrite(source, struct{}{})
				operations := []string{"metadata", "metadata", "lookup", "metadata"}
				switch name {
				case "foreign handle":
					if result != nil || !errors.Is(err, functions.ErrIncompatibleHandle) {
						t.Fatalf("foreign handle was hidden: %v", err)
					}
				case "wrong overload":
					// Unsupported binding is an error here, after successful explicit lookup.
					if result != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
						t.Fatalf("wrong overload was hidden: %v", err)
					}
					operations = append(operations, "metadata")
				default:
					// Nullable implementation results or another family decline at recognition,
					// before a call with a changed effective result Type can be constructed.
					if err != nil || result != source {
						t.Fatalf("unsupported target was published: %v", err)
					}
				}
				if !slices.Equal(boundaryOperations(context.events), operations) || expressions.PrettyPrint(source) != before {
					t.Fatal("unexpected query boundary or input mutation")
				}
				provider.target, context.events = nil, nil
				retry := must(tree.Rewrite(source, struct{}{}))
				if !retry.EqualsTo(expected) || !retry.Type().EqualsTo(source.Type()) {
					t.Fatal("returned-target retry failed")
				}
				if err := utils.ValidateForOptimization(retry); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestLaterGroupRollbackAndRetry(t *testing.T) {
	for _, failAt := range []int{2, 3, 4} {
		t.Run(fmt.Sprintf("second group event %d", failAt), func(t *testing.T) {
			a := &recordingContext{FunctionContext: must(testbed.NewStandardContext())}
			b := &recordingContext{FunctionContext: must(testbed.NewStandardContext())}
			first, firstExpected := boundaryPair(a, foundation.MayNull)
			second, secondExpected := boundaryPair(b, foundation.MayNull)
			left, right := first.Children(), second.Children()
			source := fixtureLogical(foundation.And, left[0], right[0], left[1], right[1])
			expected := fixtureLogical(foundation.And, firstExpected, secondExpected)
			if err := utils.ValidateForOptimization(source); err != nil {
				t.Fatal(err)
			}
			before := expressions.PrettyPrint(source)
			sentinel := errors.New("later group failed")
			a.events, b.events = nil, nil
			b.fault = func(functionEvent) error {
				if len(b.events) == failAt+1 {
					if !slices.Equal(boundaryOperations(a.events), []string{"metadata", "metadata", "lookup", "metadata", "metadata"}) {
						t.Fatal("first group was not prepared before later failure")
					}
					return errors.Join(functions.ErrUnsupportedLookup, functions.ErrContractViolation, sentinel)
				}
				return nil
			}
			tree := boundaryTree()
			result, err := tree.Rewrite(source, struct{}{})
			if result != nil || !errors.Is(err, sentinel) || !errors.Is(err, functions.ErrContractViolation) || !errors.Is(err, functions.ErrUnsupportedLookup) {
				t.Fatalf("partial result or missing error: %v", err)
			}
			if expressions.PrettyPrint(source) != before {
				t.Fatal("later failure changed source")
			}
			b.fault = nil
			if retry := must(tree.Rewrite(source, struct{}{})); !retry.EqualsTo(expected) {
				t.Fatal("retry retained failed attempt state")
			}
			// Only explicit unsupported lookup is a miss: retain both members of b
			// while publishing a, even when b's error is wrapped/joined with details.
			b.fault = func(event functionEvent) error {
				if event.operation == "lookup" {
					return fmt.Errorf("target unavailable: %w", errors.Join(functions.ErrUnsupportedLookup, sentinel))
				}
				return nil
			}
			missed := fixtureLogical(foundation.And, firstExpected, right[0], right[1])
			verifyPublishedGroup(t, source, missed, []*recordingContext{a, b}, 2)
		})
	}
}
