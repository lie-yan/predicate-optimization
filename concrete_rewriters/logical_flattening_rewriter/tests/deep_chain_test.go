package logical_flattening_rewriter_test

import (
	"strconv"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

func flatteningOperands(count int) []expressions.Expression {
	operands := make([]expressions.Expression, count)
	for i := range operands {
		operands[i] = reference(strconv.Itoa(i))
	}
	return operands
}

// Build without recursion, including for skewed chains. Construction belongs
// outside the benchmark timer and does not run any rewrite rule.
func flatteningWorkload(operator foundation.LogicalOperator, shape string, operands []expressions.Expression) expressions.Expression {
	switch shape {
	case "left":
		result := logical(operator, operands[0], operands[1])
		for _, operand := range operands[2:] {
			result = logical(operator, result, operand)
		}
		return result
	case "right":
		last := len(operands) - 1
		result := logical(operator, operands[last-1], operands[last])
		for i := last - 2; i >= 0; i-- {
			result = logical(operator, operands[i], result)
		}
		return result
	case "balanced":
		level := operands
		for len(level) > 1 {
			next := make([]expressions.Expression, 0, (len(level)+1)/2)
			for i := 0; i < len(level); i += 2 {
				if i+1 == len(level) {
					next = append(next, level[i])
				} else {
					next = append(next, logical(operator, level[i], level[i+1]))
				}
			}
			level = next
		}
		return level[0]
	case "flat":
		return logical(operator, operands...)
	default:
		panic("unknown flattening workload")
	}
}

func checkLargeFlattened(t testing.TB, source, result expressions.Expression, operator foundation.LogicalOperator, operands []expressions.Expression) {
	t.Helper()
	group, ok := result.(*expressions.LogicalExpression)
	if !ok || group.Operator() != operator || group.ChildCount() != len(operands) {
		t.Fatal("wrong flattened operator or arity")
	}
	if !result.Type().EqualsTo(source.Type()) {
		t.Fatal("flattening changed Type")
	}
	// One bulk read and pointer checks avoid deep equality/PrettyPrint/interpreter
	// recursion and prevent repeated Children copies from dominating this test.
	children := result.Children()
	for i, operand := range operands {
		if children[i] != operand {
			t.Fatalf("operand %d changed order or identity", i)
		}
	}
}

func TestFlattenDeepMatchingChains(t *testing.T) {
	const count = 65536
	operands := flatteningOperands(count)
	for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		for _, shape := range []string{"left", "right"} {
			t.Run(operator.String()+"/"+shape, func(t *testing.T) {
				source := flatteningWorkload(operator, shape, operands)
				tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
				result := must(tree.Rewrite(source, struct{}{}))
				checkLargeFlattened(t, source, result, operator, operands)
				before, after := reconstruction.MeasureExpression(source), reconstruction.MeasureExpression(result)
				if before.NodeCount != 2*count-1 || after.NodeCount != count+1 || before.NodeCount-after.NodeCount != count-2 {
					t.Fatal("unexpected grouping occurrence counts")
				}
				if before.LiteralOccurrences != 0 || after.LiteralOccurrences != 0 || before.MembershipEntries != 0 || after.MembershipEntries != 0 {
					t.Fatal("unexpected payload counts")
				}
				if again := must(tree.Rewrite(result, struct{}{})); again != result {
					t.Fatal("normalized large group lost identity")
				}
			})
		}
	}
}
