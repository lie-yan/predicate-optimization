package interfaces_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// independentValue owns its payload and equality; it embeds no bundled value.
// The slice makes interface equality unsafe, and multiple accessors require
// semantic Type dispatch. Comparisons never call back into the other receiver.
type independentValue struct {
	typ           types.Type
	null          bool
	payload       any
	nonComparable []byte
}

func (v independentValue) Type() types.Type  { return v.typ }
func (v independentValue) IsNull() bool      { return v.null }
func (v independentValue) Bool() bool        { return v.payload.(bool) }
func (v independentValue) Integer() int64    { return v.payload.(int64) }
func (v independentValue) Float() float64    { return v.payload.(float64) }
func (v independentValue) VarChar() string   { return v.payload.(string) }
func (v independentValue) ElementCount() int { return len(v.payload.([]valueinterfaces.Value)) }
func (v independentValue) ElementAt(index int) valueinterfaces.Value {
	return v.payload.([]valueinterfaces.Value)[index]
}

func (v independentValue) Elements() []valueinterfaces.Value {
	return append([]valueinterfaces.Value(nil), v.payload.([]valueinterfaces.Value)...)
}

func (v independentValue) Fields() []valueinterfaces.Value {
	return append([]valueinterfaces.Value(nil), v.payload.([]valueinterfaces.Value)...)
}

func (v independentValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	if v.null && n == foundation.NonNull {
		return nil, fmt.Errorf("cannot narrow null")
	}
	typ, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	v.typ = typ
	return v, nil
}

func (v independentValue) EqualsTo(other valueinterfaces.Value) bool {
	if !v.typ.EqualsTo(other.Type()) || v.null != other.IsNull() {
		return false
	}
	if v.null {
		return true
	}
	var children []valueinterfaces.Value
	switch v.typ.(type) {
	case *types.BoolType:
		return v.Bool() == other.(valueinterfaces.BoolValue).Bool()
	case *types.IntType:
		return v.Integer() == other.(valueinterfaces.IntegerValue).Integer()
	case *types.FloatType:
		return math.Float64bits(v.Float()) == math.Float64bits(other.(valueinterfaces.FloatValue).Float())
	case *types.VarCharType:
		return v.VarChar() == other.(valueinterfaces.VarCharValue).VarChar()
	case *types.ArrayType:
		children = other.(valueinterfaces.ArrayValue).Elements()
	case *types.StructType:
		children = other.(valueinterfaces.StructValue).Fields()
	default:
		panic("unexpected domain")
	}
	own := v.payload.([]valueinterfaces.Value)
	if len(own) != len(children) {
		return false
	}
	for i, child := range own {
		if !child.EqualsTo(children[i]) {
			return false
		}
	}
	return true
}

var (
	_ valueinterfaces.BoolValue    = independentValue{}
	_ valueinterfaces.IntegerValue = independentValue{}
	_ valueinterfaces.FloatValue   = independentValue{}
	_ valueinterfaces.VarCharValue = independentValue{}
	_ valueinterfaces.ArrayValue   = independentValue{}
	_ valueinterfaces.StructValue  = independentValue{}
)

func independentCopy(v valueinterfaces.Value) independentValue {
	result := independentValue{typ: v.Type(), null: v.IsNull()}
	if result.null {
		return result
	}
	var children []valueinterfaces.Value
	switch v.Type().(type) {
	case *types.BoolType:
		result.payload = v.(valueinterfaces.BoolValue).Bool()
	case *types.IntType:
		result.payload = v.(valueinterfaces.IntegerValue).Integer()
	case *types.FloatType:
		result.payload = v.(valueinterfaces.FloatValue).Float()
	case *types.VarCharType:
		result.payload = v.(valueinterfaces.VarCharValue).VarChar()
	case *types.ArrayType:
		children = v.(valueinterfaces.ArrayValue).Elements()
	case *types.StructType:
		children = v.(valueinterfaces.StructValue).Fields()
	}
	switch v.Type().(type) {
	case *types.ArrayType, *types.StructType:
		copied := make([]valueinterfaces.Value, len(children))
		for i, child := range children {
			copied[i] = independentCopy(child)
		}
		result.payload = copied
	}
	return result
}

func TestEqualityConformanceAllRepresentations(t *testing.T) {
	boolean := fixtureValue(types.NewBoolType(foundation.NonNull))
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	text := fixtureValue(types.NewVarCharType(nil, foundation.NonNull))
	one := fixtureValue(testbedvalues.NewInteger(integer, 1))
	two := fixtureValue(testbedvalues.NewInteger(integer, 2))
	samples := []valueinterfaces.Value{fixtureValue(testbedvalues.NewBool(boolean, false)), fixtureValue(testbedvalues.NewBool(boolean, true)), one, two, fixtureValue(testbedvalues.NewInteger(fixtureValue(types.NewIntType(types.Int32, foundation.NonNull)), 1))}
	for _, s := range []string{"", "a", "a\x00", "\xff"} {
		samples = append(samples, fixtureValue(testbedvalues.NewVarChar(text, s)))
	}
	bound := uint64(8)
	samples = append(samples, fixtureValue(testbedvalues.NewVarChar(fixtureValue(types.NewVarCharType(&bound, foundation.NonNull)), "a")))
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := fixtureValue(types.NewFloatType(width, foundation.NonNull))
		for _, n := range []float64{math.Inf(-1), -1, math.Copysign(0, -1), 0, 1, math.Inf(1), math.NaN()} {
			samples = append(samples, fixtureValue(testbedvalues.NewFloat(typ, n)))
		}
	}
	array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	for _, children := range [][]valueinterfaces.Value{nil, {one}, {one, two}, {two, one}, {one, one}} {
		samples = append(samples, fixtureValue(testbedvalues.NewArray(array, children)))
	}
	nullableInt := fixtureValue(integer.WithNullability(foundation.MayNull))
	nullableArray := fixtureValue(types.NewArrayType(nullableInt, foundation.NonNull))
	nested := fixtureValue(testbedvalues.NewArray(nullableArray, []valueinterfaces.Value{fixtureValue(one.WithNullability(foundation.MayNull)), fixtureValue(testbedvalues.NewNull(nullableInt))}))
	record := fixtureValue(types.NewStructType([]types.StructField{{Name: "items", Type: nullableArray}, {Name: "label", Type: text}}, foundation.NonNull))
	samples = append(samples, nested, fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{nested, fixtureValue(testbedvalues.NewVarChar(text, "tag"))})), fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{nested, fixtureValue(testbedvalues.NewVarChar(text, "other"))})))
	// Exact outer nullability and typed nulls participate in structural identity.
	original := append([]valueinterfaces.Value(nil), samples...)
	for _, v := range original {
		samples = append(samples, fixtureValue(v.WithNullability(foundation.MayNull)))
	}
	for _, typ := range []types.Type{boolean, integer, text, array, record} {
		samples = append(samples, fixtureValue(testbedvalues.NewNull(fixtureValue(typ.WithNullability(foundation.MayNull)))))
	}
	for i, a := range samples {
		for j, b := range samples {
			av, bv := independentCopy(a), independentCopy(b)
			for _, pair := range [][2]valueinterfaces.Value{{a, b}, {a, bv}, {av, b}, {av, bv}} {
				if got := pair[0].EqualsTo(pair[1]); got != (i == j) {
					t.Fatalf("equality mismatch samples %d/%d (%T/%T): %v", i, j, pair[0], pair[1], got)
				}
			}
		}
	}
	// Mixed trees exercise independent equality for children within bundled parents.
	mixed := fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{independentCopy(nested), independentCopy(fixtureValue(testbedvalues.NewVarChar(text, "tag")))}))
	external := independentCopy(mixed)
	if !mixed.EqualsTo(external) || !external.EqualsTo(mixed) {
		t.Fatal("mixed nested equality failed")
	}
}
