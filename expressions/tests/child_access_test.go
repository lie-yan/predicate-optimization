package expressions_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
)

func childAccessFixtures() []expressions.Expression {
	yes, no := logicalLiteral(foundation.NonNull, true), logicalLiteral(foundation.NonNull, false)
	ref := fixture(expressions.NewReferenceExpression(fixture(ids.NewColumnID("t", "x")), yes.Type(), "x"))
	opaque := fixture(expressions.NewOpaqueExpression(fixture(ids.NewOpaqueID("owner", "x")), yes.Type(), "opaque"))
	provider := fixture(testbed.NewStandardContext())
	handle := fixture(provider.ComparisonFunction(foundation.Equal, yes.Type(), no.Type()))
	call := fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{yes, no}))
	logical := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, ref, yes}))
	membership := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, yes.Type(), []expressions.Expression{ref, yes, no}))
	return []expressions.Expression{yes, ref, opaque, call, logical, membership}
}

func TestExpressionChildCount(t *testing.T) {
	for _, node := range childAccessFixtures() {
		t.Run(fmt.Sprintf("%T", node), func(t *testing.T) {
			children := node.Children()
			if node.ChildCount() != len(children) {
				t.Fatalf("ChildCount() = %d; want %d", node.ChildCount(), len(children))
			}
			if allocations := testing.AllocsPerRun(100, func() {
				if node.ChildCount() != len(children) {
					panic("unstable child count")
				}
			}); allocations != 0 {
				t.Fatalf("ChildCount allocated %g times", allocations)
			}
		})
	}
}

func TestExpressionChildCountNilReceiver(t *testing.T) {
	for _, node := range []expressions.Expression{
		(*expressions.LiteralExpression)(nil),
		(*expressions.ReferenceExpression)(nil),
		(*expressions.OpaqueExpression)(nil),
		(*expressions.CallExpression)(nil),
		(*expressions.LogicalExpression)(nil),
		(*expressions.SpecialFormExpression)(nil),
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Errorf("%T nil receiver did not assert", node)
				}
			}()
			node.ChildCount()
		}()
	}
}

func TestExpressionChildAt(t *testing.T) {
	for _, node := range childAccessFixtures() {
		t.Run(fmt.Sprintf("%T", node), func(t *testing.T) {
			children := node.Children()
			for i, want := range children {
				if node.ChildAt(i) != want {
					t.Fatalf("child %d changed identity or order", i)
				}
				children[i] = nil
				if node.ChildAt(i) != want || node.Children()[i] != want {
					t.Fatal("mutating returned slice changed child storage")
				}
			}
			if allocations := testing.AllocsPerRun(100, func() {
				for i := 0; i < node.ChildCount(); i++ {
					if node.ChildAt(i) == nil {
						panic("missing child")
					}
				}
			}); allocations != 0 {
				t.Fatalf("indexed access allocated %g times", allocations)
			}
			for _, index := range []int{-1, node.ChildCount(), node.ChildCount() + 1} {
				func() {
					defer func() {
						if recover() == nil {
							t.Errorf("index %d did not panic", index)
						}
					}()
					node.ChildAt(index)
				}()
			}
		})
	}
}

func TestExpressionChildAtNilReceiver(t *testing.T) {
	if !assertions.Enabled {
		t.Skip("nil receiver assertions require debug build")
	}
	for _, node := range []expressions.Expression{
		(*expressions.LiteralExpression)(nil),
		(*expressions.ReferenceExpression)(nil),
		(*expressions.OpaqueExpression)(nil),
		(*expressions.CallExpression)(nil),
		(*expressions.LogicalExpression)(nil),
		(*expressions.SpecialFormExpression)(nil),
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%T nil receiver did not panic", node)
				}
			}()
			node.ChildAt(0)
		}()
	}
}
