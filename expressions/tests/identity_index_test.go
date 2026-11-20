package expressions_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

// Slice-backed IDs remain non-comparable and have deliberately identical diagnostics.
type keyedColumn struct {
	parts       []string
	comparisons *int
}

func (id keyedColumn) Key() string    { return fmt.Sprintf("index-tests/column:%q", id.parts) }
func (id keyedColumn) String() string { return "same diagnostic" }
func (id keyedColumn) EqualsTo(other interfaces.ColumnID) bool {
	if id.comparisons != nil {
		(*id.comparisons)++
	}
	return id.Key() == other.Key()
}

type alternateColumn struct{ keyedColumn }

type keyedOpaque struct{ parts []string }

func (id keyedOpaque) Key() string                             { return fmt.Sprintf("index-tests/opaque:%q", id.parts) }
func (id keyedOpaque) String() string                          { return "same diagnostic" }
func (id keyedOpaque) EqualsTo(other interfaces.OpaqueID) bool { return id.Key() == other.Key() }

type alternateOpaque struct{ keyedOpaque }

func TestValidationIdentityKeys(t *testing.T) {
	nonnull := fixture(types.NewBoolType(foundation.NonNull))
	nullable := fixture(types.NewBoolType(foundation.MayNull))
	provider := fixture(testfunctions.NewStandardContext())
	validators := []struct {
		name string
		run  func(expressions.Expression) error
	}{
		{"structural", utils.Validate},
		{"optimization", utils.ValidateForOptimization},
		{"legacy", func(root expressions.Expression) error { return utils.ValidateWithContext(root, provider) }},
	}
	for _, opaque := range []bool{false, true} {
		makeNode := func(parts []string, alternate bool, typ types.Type) expressions.Expression {
			if opaque {
				base := keyedOpaque{parts}
				var id interfaces.OpaqueID = base
				if alternate {
					id = alternateOpaque{base}
				}
				return fixture(expressions.NewOpaqueExpression(id, typ, ""))
			}
			base := keyedColumn{parts: parts}
			var id interfaces.ColumnID = base
			if alternate {
				id = alternateColumn{base}
			}
			return fixture(expressions.NewReferenceExpression(id, typ, ""))
		}
		for _, scenario := range []struct {
			name        string
			left, right []string
			same        bool
		}{
			{"separate allocations", []string{"scope", "a"}, []string{"scope", "a"}, true},
			{"same diagnostic", []string{"scope", "a"}, []string{"scope", "b"}, false},
			{"separate scopes", []string{"scope", "a"}, []string{"other", "a"}, false},
			{"component boundaries", []string{"a:b", "c"}, []string{"a", "b:c"}, false},
		} {
			t.Run(fmt.Sprintf("opaque=%t/%s", opaque, scenario.name), func(t *testing.T) {
				for _, alternate := range []bool{false, true} {
					left := makeNode(scenario.left, false, nonnull)
					for _, rightType := range []types.Type{nonnull, nullable} {
						right := makeNode(scenario.right, alternate, rightType)
						for _, children := range [][]expressions.Expression{{left, right}, {right, left}} {
							root := fixture(expressions.NewLogicalExpression(foundation.And, children))
							for _, validator := range validators {
								wantConflict := scenario.same && rightType == nullable
								if err := validator.run(root); (err != nil) != wantConflict {
									t.Fatalf("%s conflict=%t: %v", validator.name, wantConflict, err)
								}
							}
						}
					}
				}
			})
		}
	}
}

func TestValidationIdentityKeysAvoidAllPairs(t *testing.T) {
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	comparisons := 0
	children := make([]expressions.Expression, 2048)
	for i := range children {
		id := keyedColumn{[]string{fmt.Sprint(i)}, &comparisons}
		children[i] = fixture(expressions.NewReferenceExpression(id, boolean, ""))
	}
	root := fixture(expressions.NewLogicalExpression(foundation.And, children))
	if err := utils.ValidateForOptimization(root); err != nil {
		t.Fatal(err)
	}
	if comparisons != 0 {
		t.Fatal("validation scanned IDs instead of indexing keys", comparisons)
	}
	id := keyedColumn{[]string{"0"}, &comparisons}
	conflict := fixture(expressions.NewReferenceExpression(id, fixture(boolean.WithNullability(foundation.MayNull)), ""))
	root = fixture(expressions.NewLogicalExpression(foundation.And, append(children, conflict)))
	if utils.ValidateForOptimization(root) == nil {
		t.Fatal("late declaration conflict was missed")
	}
}
