package logical_flattening_rewriter_test

import (
	"strconv"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

var flatteningBenchmarkResult expressions.Expression

func BenchmarkLogicalFlattening(b *testing.B) {
	for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		for _, shape := range []string{"left", "right", "balanced", "flat", "repeat"} {
			for _, count := range []int{128, 1024, 8192, 65536} {
				b.Run(operator.String()+"/"+shape+"/"+strconv.Itoa(count), func(b *testing.B) {
					operands := flatteningOperands(count)
					tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
					sourceShape := shape
					if shape == "repeat" {
						sourceShape = "left"
					}
					source := flatteningWorkload(operator, sourceShape, operands)
					if shape == "repeat" {
						source = must(tree.Rewrite(source, struct{}{}))
					}
					checkLargeFlattened(b, source, must(tree.Rewrite(source, struct{}{})), operator, operands)
					// Time only public traversal, including collector allocation and checked
					// reconstruction. Setup, validation and initial normalization are excluded.
					b.ReportAllocs()
					b.ResetTimer()
					var result expressions.Expression
					for i := 0; i < b.N; i++ {
						var err error
						result, err = tree.Rewrite(source, struct{}{})
						if err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					flatteningBenchmarkResult = result
					checkLargeFlattened(b, source, result, operator, operands)
					if (shape == "flat" || shape == "repeat") && result != source {
						b.Fatal("normalized input lost identity")
					}
				})
			}
		}
	}
}
