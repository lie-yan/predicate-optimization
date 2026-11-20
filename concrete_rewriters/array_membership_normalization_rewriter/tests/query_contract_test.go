package array_membership_normalization_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

// Extracting shared recognition must not add inspection, speculative binding or
// execution. Checked target construction owns the final metadata query.
func TestNormalizationQueryContract(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			t.Run(fmt.Sprintf("wrapped=%t/nullability=%v", wrapped, nullability), func(t *testing.T) {
				f := newNormalizationFixture(boundaryProvider(wrapped), nullability)
				source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001))
				expected := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
				f.context.events = nil
				tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
				result := must(tree.Rewrite(source, struct{}{}))
				if !result.EqualsTo(expected) {
					t.Fatal("normalization changed")
				}
				events := f.context.events
				if len(events) != 4 {
					t.Fatalf("provider queries=%d, want 3 metadata + 1 lookup", len(events))
				}
				if events[0].operation != "metadata" || !events[0].handle.EqualsTo(source.Handle()) || events[1].operation != "lookup" || events[1].name != functions.BuiltInArrayContains {
					t.Fatal("source recognition/target lookup sequence changed")
				}
				for _, event := range events[2:] {
					if event.operation != "metadata" || !event.handle.EqualsTo(expected.Handle()) {
						t.Fatal("target recognition/checked construction sequence changed")
					}
				}
			})
		}
	}
}
