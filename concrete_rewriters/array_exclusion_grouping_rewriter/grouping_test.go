package array_exclusion_grouping_rewriter

import (
	"reflect"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func groupingExclusion(context functioninterfaces.FunctionContext, input expressions.Expression, items ...int64) *expressions.LogicalExpression {
	integer := input.Type().(*types.ArrayType).Element().(*types.IntType)
	candidates := make([]valueinterfaces.Value, len(items))
	for i, n := range items {
		candidates[i] = admissionMust(testbedvalues.NewInteger(integer, n))
	}
	name := functions.BuiltInArrayContains
	var literal *expressions.LiteralExpression
	if len(items) == 1 {
		literal = admissionLiteral(candidates[0])
	} else {
		name = functions.BuiltInArrayContainsAny
		literal = admissionLiteral(admissionMust(testbedvalues.NewArray(admissionMust(types.NewArrayType(integer, foundation.NonNull)), candidates)))
	}
	handle := admissionMust(context.LookupBuiltInFunction(name, []types.Type{input.Type(), literal.Type()}))
	call := admissionMust(expressions.NewCallExpression(context, handle, []expressions.Expression{input, literal}))
	return admissionLogical(foundation.Not, call)
}

func TestGroupExclusionsPreservesOccurrenceOrder(t *testing.T) {
	provider := &struct {
		functioninterfaces.FunctionContext
	}{admissionMust(testbed.NewStandardContext())}
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	array := admissionMust(types.NewArrayType(integer, foundation.MayNull))
	x, y, z := admissionReference("x", array), admissionReference("y", array), admissionReference("z", array)
	// Equal IDs may be separate references with unrelated diagnostic names.
	alias := admissionMust(expressions.NewReferenceExpression(x.ID(), admissionMust(types.NewArrayType(integer, foundation.MayNull)), "alias"))
	repeated := groupingExclusion(provider, x, 2)
	truth := admissionLiteral(admissionMust(testbedvalues.NewBool(admissionMust(types.NewBoolType(foundation.NonNull)), true)))
	source := admissionLogical(foundation.And, repeated, groupingExclusion(provider, y, 9), truth, groupingExclusion(provider, alias, 1, 2), groupingExclusion(provider, y, 7), groupingExclusion(provider, z, 4), repeated)
	occurrences := admissionMust(collectExclusions(source))
	before := expressions.PrettyPrint(source)
	// Disable every provider method after admission; grouping must use only the
	// recorded context identity, Types, operand identity and occurrence order.
	provider.FunctionContext = nil
	groups := groupExclusions(occurrences)
	if len(groups) != 2 {
		t.Fatalf("groups=%d, want two", len(groups))
	}
	for i, want := range [][]int{{0, 3, 6}, {1, 4}} {
		var indices []int
		var candidates []int64
		for _, member := range groups[i].members {
			indices = append(indices, member.index)
			for _, value := range member.candidates {
				candidates = append(candidates, value.(*testbedvalues.IntegerValue).Integer())
			}
		}
		expected := [][]int64{{2, 1, 2, 2}, {9, 7}}[i]
		if !slices.Equal(indices, want) || !slices.Equal(candidates, expected) {
			t.Fatalf("group %d order/duplicates lost: %v %v", i, indices, candidates)
		}
	}
	if groups[0].members[0].exclusion != repeated || groups[0].members[2].exclusion != repeated || groups[0].members[1].array != alias {
		t.Fatal("member occurrence identities changed")
	}
	if expressions.PrettyPrint(source) != before {
		t.Fatal("grouping mutated input expressions")
	}
	groups[0].members[0].index = 99
	if occurrences[0].index != 0 {
		t.Fatal("group member slice aliases occurrence storage")
	}
	if len(groupExclusions(nil)) != 0 || len(groupExclusions(occurrences[:1])) != 0 {
		t.Fatal("empty/single occurrence became a group")
	}
}

// Interface-bearing structs can be statically comparable but dynamically unsafe.
// A Type.Comparable check alone would miss the slice hidden in payload.
type groupingContextValue struct {
	functioninterfaces.FunctionContext
	payload any
}

type groupingHandleValue struct {
	functioninterfaces.FunctionHandle
	marker []int
}

func TestGroupContextBoundaries(t *testing.T) {
	provider := admissionMust(testbed.NewStandardContext())
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	array := admissionMust(types.NewArrayType(integer, foundation.NonNull))
	input := admissionReference("x", array)
	sharedA := groupingContextValue{FunctionContext: provider, payload: 1}
	sharedB := groupingContextValue{FunctionContext: provider, payload: 1}
	hiddenSlice := groupingContextValue{FunctionContext: provider, payload: []int{1}}
	if !reflect.TypeOf(hiddenSlice).Comparable() || reflect.ValueOf(hiddenSlice).Comparable() {
		t.Fatal("fixture must isolate dynamic comparability")
	}
	other := &struct {
		functioninterfaces.FunctionContext
	}{provider}
	for _, tc := range []struct {
		name        string
		left, right functioninterfaces.FunctionContext
		groups      int
	}{
		{"shared pointer", provider, provider, 1},
		{"equal context values", sharedA, sharedB, 1},
		{"distinct contexts same provider", provider, other, 0},
		{"distinct comparable values", sharedA, groupingContextValue{FunctionContext: provider, payload: 2}, 0},
		{"hidden slice both", hiddenSlice, hiddenSlice, 0},
		{"hidden slice right", provider, hiddenSlice, 0},
		{"hidden slice left", hiddenSlice, provider, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := admissionLogical(foundation.And, groupingExclusion(tc.left, input, 1), groupingExclusion(tc.right, input, 2))
			members := admissionMust(collectExclusions(source))
			if got := groupExclusions(members); len(got) != tc.groups {
				t.Fatalf("groups=%d, want %d", len(got), tc.groups)
			}
		})
	}
	// A non-comparable handle is a valid recorded call payload, never a grouping key.
	wrapper := &groupingHandleContext{FunctionContext: provider}
	source := admissionLogical(foundation.And, groupingExclusion(wrapper, input, 1), groupingExclusion(wrapper, input, 2))
	members := admissionMust(collectExclusions(source))
	if reflect.ValueOf(members[0].call.Handle()).Comparable() || !members[0].call.Handle().EqualsTo(members[0].call.Handle()) {
		t.Fatal("invalid non-comparable handle fixture")
	}
	if len(groupExclusions(members)) != 1 {
		t.Fatal("non-comparable handles blocked grouping")
	}
}

// These adapters support lookup and valid construction for grouping tests.
// The executable non-comparable-handle fixture lives in the external tests package.
func (h groupingHandleValue) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	o, ok := other.(groupingHandleValue)
	return ok && h.FunctionHandle.EqualsTo(o.FunctionHandle)
}

type groupingHandleContext struct {
	functioninterfaces.FunctionContext
}

func (c *groupingHandleContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	h, err := c.FunctionContext.LookupBuiltInFunction(name, args)
	if err != nil {
		return nil, err
	}
	return groupingHandleValue{FunctionHandle: h, marker: []int{1}}, nil
}

func (c *groupingHandleContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	return c.FunctionContext.Metadata(handle.(groupingHandleValue).FunctionHandle)
}

type groupingColumnID struct {
	expressioninterfaces.ColumnID
	marker []int
}

func TestArrayOperandCompatibility(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	array := admissionMust(types.NewArrayType(integer, foundation.NonNull))
	ref := admissionReference("x", array)
	alias := admissionMust(expressions.NewReferenceExpression(groupingColumnID{ColumnID: ref.ID(), marker: []int{1}}, array, "alias"))
	other := admissionMust(expressions.NewReferenceExpression(admissionReference("y", array).ID(), array, ref.DisplayName()))
	value := admissionMust(testbedvalues.NewArray(array, nil))
	literal := admissionLiteral(value)
	// Type-mismatch pairs are individually valid expressions from separate input
	// schemas, not conflicting declarations in one validated optimization graph.
	for _, tc := range []struct {
		name        string
		left, right expressions.Expression
		want        bool
	}{
		{"reference alias", ref, alias, true},
		{"same spelling distinct column", ref, other, false},
		{"outer nullability", ref, admissionMust(expressions.NewReferenceExpression(ref.ID(), admissionMust(array.WithNullability(foundation.MayNull)), "x")), false},
		{"width", ref, admissionMust(expressions.NewReferenceExpression(ref.ID(), admissionMust(types.NewArrayType(admissionMust(types.NewIntType(types.Int32, foundation.NonNull)), foundation.NonNull)), "x")), false},
		{"same literal", literal, literal, true},
		{"shared Value distinct literal", literal, admissionLiteral(value), false},
		{"equal payload distinct literal", literal, admissionLiteral(admissionMust(testbedvalues.NewArray(array, nil))), false},
		{"reference vs literal", ref, literal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if sameArrayOperand(tc.left, tc.right) != tc.want || sameArrayOperand(tc.right, tc.left) != tc.want {
				t.Fatal("array identity or complete Type comparison differs")
			}
		})
	}
	// Nested constraints remain part of the full array Type comparison.
	small, large := uint64(8), uint64(16)
	array8 := admissionMust(types.NewArrayType(admissionMust(types.NewVarCharType(&small, foundation.NonNull)), foundation.NonNull))
	array16 := admissionMust(types.NewArrayType(admissionMust(types.NewVarCharType(&large, foundation.NonNull)), foundation.NonNull))
	nested8 := admissionReference("nested", admissionMust(types.NewArrayType(array8, foundation.NonNull)))
	nested16 := admissionMust(expressions.NewReferenceExpression(nested8.ID(), admissionMust(types.NewArrayType(array16, foundation.NonNull)), "nested"))
	if sameArrayOperand(nested8, nested16) {
		t.Fatal("nested VARCHAR bounds were ignored")
	}
	provider := admissionMust(testbed.NewStandardContext())
	for _, shared := range []bool{false, true} {
		right := admissionLiteral(value)
		if shared {
			right = literal
		}
		occurrences := admissionMust(collectExclusions(admissionLogical(foundation.And, groupingExclusion(provider, literal, 1), groupingExclusion(provider, right, 2))))
		want := 0
		if shared {
			want = 1
		}
		if len(groupExclusions(occurrences)) != want {
			t.Fatal("literal node identity not respected by grouping")
		}
	}
}
