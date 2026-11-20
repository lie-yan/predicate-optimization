package array_exclusion_grouping_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

type countedColumnID struct {
	expressioninterfaces.ColumnID
	comparisons *int
}

func (id *countedColumnID) EqualsTo(other expressioninterfaces.ColumnID) bool {
	*id.comparisons++
	return id.ColumnID.EqualsTo(other)
}

// Count identity checks, not wall time: distinct keys must never trigger the
// former all-partitions scan, and matching aliases compare only in their bucket.
func TestGroupingIndexWorkAndOrder(t *testing.T) {
	for _, count := range []int{128, 2048} {
		for _, paired := range []bool{false, true} {
			t.Run(fmt.Sprintf("n=%d/paired=%t", count, paired), func(t *testing.T) {
				integer := must(types.NewIntType(types.Int64, foundation.NonNull))
				fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, foundation.NonNull)
				columns := count
				if paired {
					columns /= 2
				}
				comparisons := 0
				var children []expressions.Expression
				for i := 0; i < count; i++ {
					reference := fixtureReference(fmt.Sprintf("column%d", i%columns), fixture.input.Type())
					alias := must(expressions.NewReferenceExpression(&countedColumnID{reference.ID(), &comparisons}, reference.Type(), "alias"))
					children = append(children, fixtureExclude(fixture.contains(alias, must(testbedvalues.NewInteger(integer, int64(i))))))
				}
				source := fixtureLogical(foundation.And, children...)
				if err := utils.ValidateForOptimization(source); err != nil {
					t.Fatal(err)
				}
				comparisons = 0
				result := must(boundaryTree().Rewrite(source, struct{}{}))
				if comparisons != count-columns {
					t.Fatalf("identity comparisons=%d, want %d", comparisons, count-columns)
				}
				if !paired {
					if result != source {
						t.Fatal("distinct columns grouped")
					}
				} else {
					if result.ChildCount() != columns {
						t.Fatal("wrong partition count")
					}
					for i, child := range result.Children() {
						call := child.Children()[0].(*expressions.CallExpression)
						args := call.Arguments()
						if args[0] != children[i].Children()[0].Children()[0] {
							t.Fatal("first array identity changed")
						}
						items := args[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue).Elements()
						if len(items) != 2 || items[0].(*testbedvalues.IntegerValue).Integer() != int64(i) || items[1].(*testbedvalues.IntegerValue).Integer() != int64(i+columns) {
							t.Fatal("first-seen group/member order changed")
						}
					}
				}
			})
		}
	}
}
