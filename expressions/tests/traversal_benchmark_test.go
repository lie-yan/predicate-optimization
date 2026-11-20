package expressions_test

import (
	"testing"

	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	interpreterinterfaces "github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

// A shared subtree is deliberately visited at each occurrence.
func BenchmarkExpressionChildTraversal(b *testing.B) {
	leaf := logicalLiteral(foundation.NonNull, true)
	children := make([]expressions.Expression, 64)
	for i := range children {
		children[i] = leaf
	}
	group := fixture(expressions.NewLogicalExpression(foundation.And, children))
	for i := range children {
		children[i] = group
	}
	root := fixture(expressions.NewLogicalExpression(foundation.And, children))
	tree := rewriter.NewExpressionTreeRewriter[struct{}](rewriter.BaseExpressionRewriter[struct{}]{})
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	b.Run("UnchangedRewrite", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, err := tree.Rewrite(root, struct{}{})
			if err != nil || result != root {
				b.Fatal("rewrite changed source", err)
			}
		}
	})
	b.Run("Measure", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			if size := reconstruction.MeasureExpression(root); size.NodeCount != 4161 {
				b.Fatal(size)
			}
		}
	})
	b.Run("PrettyPrint", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			if expressions.PrettyPrint(root) == "" {
				b.Fatal("empty tree")
			}
		}
	})
	b.Run("Validate", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			if err := utils.Validate(root); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Evaluate", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			if _, err := evaluator.Evaluate(root, struct {
				interpreterinterfaces.Environment
			}{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
