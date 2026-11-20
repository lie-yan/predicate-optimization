package expressions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
)

// Test-only fixture builders retain access to private fields in expressions.
// This external runner can import utils without creating an import cycle.
func TestValidateMalformedGraphs(t *testing.T) {
	nodes := append(expressions.MalformedNodesForValidationTest(), expressions.MalformedCallsForValidationTest()...)
	for name, validate := range map[string]func(expressions.Expression) error{
		"structural": utils.Validate, "optimization": utils.ValidateForOptimization,
	} {
		t.Run(name, func(t *testing.T) {
			for i, node := range nodes {
				if err := validate(node); err == nil {
					t.Errorf("accepted malformed fixture %d (%T)", i, node)
				}
			}
		})
	}
}
