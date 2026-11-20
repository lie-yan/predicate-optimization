// Package expressions supplies caller-side expression fixtures for development.
package expressions

import (
	"fmt"

	shared "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// ColumnID uses a nonempty scope/symbol pair as immutable testbed identity.
// Scope names are assigned by the caller; equal pairs intentionally identify
// the same column, even across separately constructed instances.
type ColumnID struct {
	scope  string
	symbol string
	key    string
}

var _ shared.ColumnID = (*ColumnID)(nil)

func NewColumnID(scope, symbol string) (*ColumnID, error) {
	if scope == "" || symbol == "" {
		return nil, fmt.Errorf("column scope and symbol must be nonempty")
	}
	return &ColumnID{scope: scope, symbol: symbol, key: fmt.Sprintf("github.com/lie-yan/predicate-optimization/testbed/column:%q:%q", scope, symbol)}, nil
}

func (id *ColumnID) EqualsTo(other shared.ColumnID) bool {
	id.assertValid()
	assertions.NotNil(other)
	return id.key == other.Key()
}

func (id *ColumnID) String() string {
	id.assertValid()
	return fmt.Sprintf("ColumnID(scope=%q, symbol=%q)", id.scope, id.symbol)
}

func (id *ColumnID) assertValid() {
	if id == nil || id.scope == "" || id.symbol == "" {
		panic("testbed/expressions: expected constructed ColumnID")
	}
}

// Key returns the precomputed, domain-qualified canonical identity.
func (id *ColumnID) Key() string {
	id.assertValid()
	return id.key
}
