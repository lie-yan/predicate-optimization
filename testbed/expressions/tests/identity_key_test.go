package expressions_test

import (
	"testing"

	interfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
)

// A different representation interoperates by adopting the same canonical key.
type columnKey string

func (id columnKey) Key() string                             { return string(id) }
func (id columnKey) String() string                          { return "another diagnostic" }
func (id columnKey) EqualsTo(other interfaces.ColumnID) bool { return id.Key() == other.Key() }

type opaqueKey string

func (id opaqueKey) Key() string                             { return string(id) }
func (id opaqueKey) String() string                          { return "another diagnostic" }
func (id opaqueKey) EqualsTo(other interfaces.OpaqueID) bool { return id.Key() == other.Key() }

func TestCanonicalIdentityKeys(t *testing.T) {
	seen := make(map[string]bool)
	for _, pair := range [][2]string{{"scope", "field"}, {"other", "field"}, {"a:b", "c"}, {"a", "b:c"}, {"a\x00b", "c"}, {"a", "b\x00c"}, {"作用域", "字段"}, {"a\xff", "b"}, {"a\xfe", "b"}, {"a\"", "b"}} {
		column, err := testbed.NewColumnID(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		copyColumn, err := testbed.NewColumnID(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		opaque, err := testbed.NewOpaqueID(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		copyOpaque, err := testbed.NewOpaqueID(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if column.Key() != copyColumn.Key() || !column.EqualsTo(copyColumn) || !column.EqualsTo(columnKey(column.Key())) || !columnKey(column.Key()).EqualsTo(column) {
			t.Fatal("column key and equality disagree")
		}
		if opaque.Key() != copyOpaque.Key() || !opaque.EqualsTo(copyOpaque) || !opaque.EqualsTo(opaqueKey(opaque.Key())) || !opaqueKey(opaque.Key()).EqualsTo(opaque) {
			t.Fatal("opaque key and equality disagree")
		}
		for _, key := range []string{column.Key(), opaque.Key()} {
			if seen[key] {
				t.Fatal("distinct identity components/domains produced the same key", key)
			}
			seen[key] = true
		}
	}
}
