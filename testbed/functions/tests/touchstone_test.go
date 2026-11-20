package functions_test

import (
	"testing"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// This checks the function boundary, not a parsed expression or a rewrite.
// IN and AND/OR belong to the future expression layer. Equality is checked as
// a scalar reasoning operation; this does not prescribe an IN implementation.
func TestTouchstoneFunctionBoundary(t *testing.T) {
	f := newArrayFixture(t)
	text, err := types.NewVarCharType(nil, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	literal := func(s string) valueinterfaces.Value {
		v, err := testbedvalues.NewVarChar(text, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		t.Run(n.Suffix(), func(t *testing.T) {
			// Explicit primary schema; article_type remains non-null in both variants.
			schema := map[string]types.Type{"article_type": text}
			for _, field := range []string{"is_review", "brand_id"} {
				schema[field], err = f.integer.WithNullability(n)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, field := range []string{"tag_ids", "category_ids", "mall_ids"} {
				schema[field], err = f.array.WithNullability(n)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, tc := range []struct {
				name, field      string
				builtin          shared.BuiltInFunctionName // Empty identifies scalar equality.
				input, candidate valueinterfaces.Value
				want             bool
			}{
				{"article tsa_04 equality", "article_type", "", literal("tsa_01"), literal("tsa_04"), false},
				{"article IN equality support", "article_type", "", literal("tsa_01"), literal("tsa_01"), true},
				{"review IN equality support", "is_review", "", evalInteger(t, f.integer, 8795005), evalInteger(t, f.integer, 8795005), true},
				{"brand IN equality support", "brand_id", "", evalInteger(t, f.integer, 8794011), evalInteger(t, f.integer, 8794011), true},
				{"input tag any", "tag_ids", shared.BuiltInArrayContainsAny, f.value(t, 8791002), f.value(t, 8791001, 8791002), true},
				{"input tag contains", "tag_ids", shared.BuiltInArrayContains, f.value(t, 8791004), evalInteger(t, f.integer, 8791004), true},
				{"input category any", "category_ids", shared.BuiltInArrayContainsAny, f.value(t, 8792001), f.value(t, 8792001), true},
				{"input mall any", "mall_ids", shared.BuiltInArrayContainsAny, f.value(t, 8793001), f.value(t, 8793001), true},
				{"replacement category contains", "category_ids", shared.BuiltInArrayContains, f.value(t, 8792001), evalInteger(t, f.integer, 8792001), true},
				{"replacement mall contains", "mall_ids", shared.BuiltInArrayContains, f.value(t, 8793001), evalInteger(t, f.integer, 8793001), true},
				{"replacement tag any", "tag_ids", shared.BuiltInArrayContainsAny, f.value(t, 8791009), f.value(t, 8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009), true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					input, err := tc.input.WithNullability(schema[tc.field].Nullability())
					if err != nil {
						t.Fatal(err)
					}
					if !input.Type().EqualsTo(schema[tc.field]) {
						t.Fatal("fixture violates field schema")
					}
					lookup := func() (functioninterfaces.FunctionHandle, error) {
						if tc.builtin == "" {
							return f.context.ComparisonFunction(foundation.Equal, input.Type(), tc.candidate.Type())
						}
						return f.context.LookupBuiltInFunction(tc.builtin, []types.Type{input.Type(), tc.candidate.Type()})
					}
					handle, err := lookup()
					if err != nil {
						t.Fatal(err)
					}
					again, err := lookup()
					if err != nil || !handle.EqualsTo(again) {
						t.Fatal("unstable function identity")
					}
					metadata, err := f.context.Metadata(handle)
					if err != nil {
						t.Fatal(err)
					}
					if tc.builtin == "" {
						if op, ok := metadata.Operator().Get(); !ok || op != foundation.Equal {
							t.Fatal("missing equality classification")
						}
					} else if name, ok := metadata.BuiltInName().Get(); !ok || name != tc.builtin {
						t.Fatal("missing built-in classification")
					}
					policies := metadata.NullPolicies()
					if len(policies) != 2 || policies[0] != shared.PropagateNull {
						t.Fatal("missing field null propagation")
					}
					expectedCandidatePolicy := shared.PropagateNull
					if tc.builtin != "" {
						expectedCandidatePolicy = shared.NullPolicyUnspecified
					}
					if policies[1] != expectedCandidatePolicy {
						t.Fatal("incorrect candidate null policy")
					}
					result := evaluateStandard(t, f.context, handle, input, tc.candidate)
					if result.(*testbedvalues.BoolValue).Bool() != tc.want {
						t.Fatal("incorrect concrete result")
					}
					not, err := f.context.LookupBuiltInFunction(shared.BuiltInNot, []types.Type{result.Type()})
					if err != nil {
						t.Fatal(err)
					}
					notMetadata, err := f.context.Metadata(not)
					if err != nil {
						t.Fatal(err)
					}
					if name, ok := notMetadata.BuiltInName().Get(); !ok || name != shared.BuiltInNot {
						t.Fatal("missing NOT classification")
					}
					if evaluateStandard(t, f.context, not, result).(*testbedvalues.BoolValue).Bool() == tc.want {
						t.Fatal("incorrect negation")
					}
					if schema[tc.field].Nullability() == foundation.MayNull {
						null, err := testbedvalues.NewNull(schema[tc.field])
						if err != nil {
							t.Fatal(err)
						}
						result = evaluateStandard(t, f.context, handle, null, tc.candidate)
						if !result.IsNull() || !evaluateStandard(t, f.context, not, result).IsNull() {
							t.Fatal("nullable field lost its null result")
						}
					}
				})
			}
		})
	}
	// Nullable article_type is a separate correctness variant, not the primary
	// simplification fixture. Its equality result and negation must retain null.
	nullableText, err := text.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	null, err := testbedvalues.NewNull(nullableText)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := f.context.ComparisonFunction(foundation.Equal, nullableText, text)
	if err != nil {
		t.Fatal(err)
	}
	result := evaluateStandard(t, f.context, equal, null, literal("tsa_04"))
	not, err := f.context.LookupBuiltInFunction(shared.BuiltInNot, []types.Type{result.Type()})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsNull() || !evaluateStandard(t, f.context, not, result).IsNull() {
		t.Fatal("nullable article result must remain null")
	}
}
