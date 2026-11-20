package functions

import (
	"errors"
	"testing"

	shared "github.com/lie-yan/predicate-optimization/functions"
)

// Private access is required to forge a same-domain entry not in the catalog.
func TestContextUnknownEntry(t *testing.T) {
	catalog, err := NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := NewContext(catalog)
	for _, definition := range []*entry{nil, {name: "missing"}} {
		handle := &Handle{identity: catalog.identity, entry: definition}
		if _, err := provider.Metadata(handle); !errors.Is(err, shared.ErrUnknownHandle) {
			t.Fatalf("expected unknown entry, got %v", err)
		}
	}
}
