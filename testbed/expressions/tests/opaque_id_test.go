package expressions_test

import (
	"testing"

	testbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
)

func TestOpaqueID(t *testing.T) {
	makeID := func(owner, token string) *testbed.OpaqueID {
		id, err := testbed.NewOpaqueID(owner, token)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := makeID("owner", "column"), makeID("owner", "column")
	if a == b || !a.EqualsTo(b) {
		t.Fatal("identity laws failed")
	}
	if a.EqualsTo(makeID("other", "column")) || a.EqualsTo(makeID("owner", "other")) {
		t.Fatal("identity component ignored")
	}
	if a.String() != `OpaqueID(owner="owner", token="column")` {
		t.Fatal("incorrect diagnostic")
	}
	for _, parts := range [][2]string{{"", ""}, {"owner", ""}, {"", "column"}} {
		if id, err := testbed.NewOpaqueID(parts[0], parts[1]); id != nil || err == nil {
			t.Fatal("empty component accepted")
		}
	}
}
