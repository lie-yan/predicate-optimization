package expressions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	shared "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
)

func TestColumnID(t *testing.T) {
	makeID := func(scope, symbol string) *testbed.ColumnID {
		id, err := testbed.NewColumnID(scope, symbol)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b, c := makeID("scope", "column"), makeID("scope", "column"), makeID("scope", "column")
	if a == b || !a.EqualsTo(a) || !a.EqualsTo(b) || !b.EqualsTo(a) || !b.EqualsTo(c) || !a.EqualsTo(c) {
		t.Fatal("identity laws failed")
	}
	if a.EqualsTo(makeID("other", "column")) || a.EqualsTo(makeID("scope", "other")) {
		t.Fatal("identity component ignored")
	}
	if a.String() != `ColumnID(scope="scope", symbol="column")` {
		t.Fatal("incorrect diagnostic")
	}
	for _, parts := range [][2]string{{"", ""}, {"scope", ""}, {"", "column"}} {
		if id, err := testbed.NewColumnID(parts[0], parts[1]); id != nil || err == nil {
			t.Fatal("empty component accepted")
		}
	}
	var nilID *testbed.ColumnID
	for _, call := range []func(){func() { a.EqualsTo(nil) }, func() { a.EqualsTo(shared.ColumnID(nilID)) }, func() { nilID.EqualsTo(a) }, func() { _ = nilID.String() }} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil did not assert")
				}
			}()
			call()
		}()
	}
}
