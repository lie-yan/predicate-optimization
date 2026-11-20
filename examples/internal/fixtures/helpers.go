// Package fixtures supplies shared setup for the runnable examples.
package fixtures

import (
	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Examples panic on unexpected setup errors to keep the API flow visible.
func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type Binding struct {
	ID    expressioninterfaces.ColumnID
	Value valueinterfaces.Value
}
type Environment []Binding

func (e Environment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for _, b := range e {
		if b.ID.EqualsTo(id) {
			return foundation.Some(b.Value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

// These construction helpers keep the examples focused on public analysis APIs.
// Like the expression examples, they panic on unexpected setup errors.
func ScalarReference(name string, typ types.Type) *expressions.ReferenceExpression {
	return Must(expressions.NewReferenceExpression(Must(ids.NewColumnID("example", name)), typ, name))
}

func ScalarLiteral(value valueinterfaces.Value) *expressions.LiteralExpression {
	return Must(expressions.NewLiteralExpression(value))
}

func ScalarMembership(ref *expressions.ReferenceExpression, entries ...valueinterfaces.Value) *expressions.SpecialFormExpression {
	args := []expressions.Expression{ref}
	for _, entry := range entries {
		args = append(args, ScalarLiteral(entry))
	}
	return Must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, Must(types.NewBoolType(ref.Type().Nullability())), args))
}

func ScalarComparison(context functioninterfaces.FunctionContext, op foundation.OperatorType, left, right expressions.Expression) *expressions.CallExpression {
	handle := Must(context.ComparisonFunction(op, left.Type(), right.Type()))
	return Must(expressions.NewCallExpression(context, handle, []expressions.Expression{left, right}))
}

func ScalarLogical(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
	return Must(expressions.NewLogicalExpression(op, children))
}

func ScalarSummary(context functioninterfaces.FunctionContext, source expressions.Expression) *facts.PredicateSummary {
	result := Must(fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context).Extract(source))
	summary, present := result.Summary().Get()
	if !present {
		panic("example requires exact extraction")
	}
	return summary
}
