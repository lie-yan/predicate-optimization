// Package functions supplies reusable caller-side function fixtures for tests
// and prototype development. Shared function contracts do not depend on it.
package functions

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
)

// Definition is setup data for an immutable resolved function entry. Metadata
// must be valid and immutable. Executable behavior is not part of this setup yet.
type Definition struct {
	Namespace string
	Name      string
	Metadata  *shared.FunctionMetadata
}

// Catalog owns a fixed set of resolved definitions and its own identity domain.
// Use NewCatalog; the zero value is invalid. Immutable metadata and types are
// shared, while caller-owned definition slices are not retained.
type Catalog struct {
	identity   *catalogIdentity
	entries    []*entry
	byBuiltIn  map[shared.BuiltInFunctionName][]*entry
	byOperator map[foundation.OperatorType][]*entry
}

// Nonzero size ensures separately allocated identity tokens have distinct addresses.
type catalogIdentity struct{ marker byte }

type entry struct {
	namespace string
	name      string
	metadata  *shared.FunctionMetadata
}

// NewCatalog canonicalizes definitions by namespace, name and complete semantic
// signature. Equal definitions share an entry; conflicting semantic metadata for
// the same identity is rejected. Display names are diagnostic; the first wins.
// Changed definitions should be placed in a new catalog identity domain.
func NewCatalog(definitions []Definition) (*Catalog, error) {
	catalog := &Catalog{
		identity:   &catalogIdentity{},
		byBuiltIn:  make(map[shared.BuiltInFunctionName][]*entry),
		byOperator: make(map[foundation.OperatorType][]*entry),
	}
	for i, definition := range definitions {
		if definition.Namespace == "" || definition.Name == "" || definition.Metadata == nil || definition.Metadata.Signature() == nil {
			return nil, fmt.Errorf("invalid function definition at position %d", i)
		}
		candidate := &entry{definition.Namespace, definition.Name, definition.Metadata}
		duplicate := false
		for _, existing := range catalog.entries {
			if existing.namespace != candidate.namespace || existing.name != candidate.name || !sameSignature(existing.metadata.Signature(), candidate.metadata.Signature()) {
				continue
			}
			if !sameProperties(existing.metadata, candidate.metadata) {
				return nil, fmt.Errorf("conflicting metadata for %s.%s", candidate.namespace, candidate.name)
			}
			duplicate = true
			break
		}
		if !duplicate {
			catalog.entries = append(catalog.entries, candidate)
			// Index canonical entries only, preserving overload order. An entry
			// can be reachable by both its built-in name and its operator.
			if name, present := candidate.metadata.BuiltInName().Get(); present {
				catalog.byBuiltIn[name] = append(catalog.byBuiltIn[name], candidate)
			}
			if operator, present := candidate.metadata.Operator().Get(); present {
				catalog.byOperator[operator] = append(catalog.byOperator[operator], candidate)
			}
		}
	}
	return catalog, nil
}

// Handles returns fresh handles in first-definition order, one per canonical
// entry. Mutating the returned slice cannot change the catalog.
func (c *Catalog) Handles() []*Handle {
	if c == nil || c.identity == nil {
		panic("testbed/functions: expected constructed Catalog")
	}
	handles := make([]*Handle, len(c.entries))
	for i, definition := range c.entries {
		handles[i] = &Handle{identity: c.identity, entry: definition}
	}
	return handles
}

func sameSignature(a, b *shared.FunctionSignature) bool {
	left, right := a.ArgumentTypes(), b.ArgumentTypes()
	if len(left) != len(right) || !a.ResultType().EqualsTo(b.ResultType()) {
		return false
	}
	for i := range left {
		if !left[i].EqualsTo(right[i]) {
			return false
		}
	}
	return true
}

func sameProperties(a, b *shared.FunctionMetadata) bool {
	if a.BuiltInName() != b.BuiltInName() || a.Operator() != b.Operator() {
		return false
	}
	left, right := a.NullPolicies(), b.NullPolicies()
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
