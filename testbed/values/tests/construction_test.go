package values_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestConstructorNilAssertions(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	record := fixtureValue(types.NewStructType([]types.StructField{{Name: "x", Type: integer}}, foundation.NonNull))
	cases := map[string]func(){
		"null nil type":       func() { _, _ = testbedvalues.NewNull(nil) },
		"null typed nil type": func() { _, _ = testbedvalues.NewNull((*types.ArrayType)(nil)) },
		"bool type":           func() { _, _ = testbedvalues.NewBool(nil, true) },
		"int type":            func() { _, _ = testbedvalues.NewInteger(nil, 1) },
		"float type":          func() { _, _ = testbedvalues.NewFloat(nil, 1) },
		"varchar type":        func() { _, _ = testbedvalues.NewVarChar(nil, "") },
		"array type":          func() { _, _ = testbedvalues.NewArray(nil, nil) },
		"struct type":         func() { _, _ = testbedvalues.NewStruct(nil, nil) },
		"array nil child":     func() { _, _ = testbedvalues.NewArray(array, []valueinterfaces.Value{nil}) },
		"array typed nil child": func() {
			_, _ = testbedvalues.NewArray(array, []valueinterfaces.Value{(*testbedvalues.IntegerValue)(nil)})
		},
		"struct nil child": func() { _, _ = testbedvalues.NewStruct(record, []valueinterfaces.Value{nil}) },
		"struct typed nil child": func() {
			_, _ = testbedvalues.NewStruct(record, []valueinterfaces.Value{(*testbedvalues.IntegerValue)(nil)})
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Fatal("expected non-nil assertion")
				}
			}()
			call()
		})
	}
}

func TestConstructionSharesNestedComposites(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	var child valueinterfaces.Value = fixtureValue(testbedvalues.NewInteger(integer, 8))
	for depth := 0; depth < 64; depth++ {
		typ := fixtureValue(types.NewArrayType(child.Type(), foundation.NonNull))
		parent := fixtureValue(testbedvalues.NewArray(typ, []valueinterfaces.Value{child, child}))
		children := parent.Elements()
		if children[0] != child || children[1] != child || parent.Type() != typ {
			t.Fatal("construction copied immutable children or Type")
		}
		child = parent
	}
}
