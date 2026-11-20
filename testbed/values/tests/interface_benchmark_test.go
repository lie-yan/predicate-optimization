package values_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

var (
	interfaceValueSink    valueinterfaces.Value
	interfaceTypeSink     types.Type
	interfaceIntegerSink  int64
	interfaceFloatSink    float64
	interfaceStringSink   string
	interfaceBoolSink     bool
	interfaceChildrenSink []valueinterfaces.Value
)

// Opaque boundaries prevent compile-time devirtualization without adding a
// helper call inside the timed loop. Production optimization remains enabled.
//
//go:noinline
func opaqueValue(v valueinterfaces.Value) valueinterfaces.Value { return v }

//go:noinline
func opaqueInteger(v valueinterfaces.IntegerValue) valueinterfaces.IntegerValue { return v }

//go:noinline
func opaqueArray(v valueinterfaces.ArrayValue) valueinterfaces.ArrayValue { return v }

//go:noinline
func opaqueFactory(f valueinterfaces.ValueFactory) valueinterfaces.ValueFactory { return f }

func BenchmarkValueInterface(b *testing.B) {
	typ := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	concrete := fixtureValue(testbedvalues.NewInteger(typ, 42))
	other := fixtureValue(testbedvalues.NewInteger(typ, 42))
	dynamic := opaqueValue(concrete)
	payload := opaqueInteger(concrete)
	b.Run("Integer/Concrete", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceIntegerSink = concrete.Integer()
		}
	})
	b.Run("Integer/Interface", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceIntegerSink = payload.Integer()
		}
	})
	b.Run("Integer/AssertAndRead", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceIntegerSink = dynamic.(valueinterfaces.IntegerValue).Integer()
		}
	})
	b.Run("Type/Concrete", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceTypeSink = concrete.Type()
		}
	})
	b.Run("Type/Interface", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceTypeSink = dynamic.Type()
		}
	})
	b.Run("IsNull/Concrete", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceBoolSink = concrete.IsNull()
		}
	})
	b.Run("IsNull/Interface", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceBoolSink = dynamic.IsNull()
		}
	})
	b.Run("EqualsTo/Concrete", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceBoolSink = concrete.EqualsTo(other)
		}
	})
	b.Run("EqualsTo/Interface", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceBoolSink = dynamic.EqualsTo(other)
		}
	})
	b.Run("WithNullability/Concrete", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.WithNullability(foundation.MayNull)
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("WithNullability/Interface", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.WithNullability(foundation.MayNull)
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	arrayType := fixtureValue(types.NewArrayType(typ, foundation.NonNull))
	children := make([]valueinterfaces.Value, 64)
	for i := range children {
		children[i] = concrete
	}
	array := fixtureValue(testbedvalues.NewArray(arrayType, children))
	arrayView := opaqueArray(array)
	b.Run("Elements64/Concrete", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceChildrenSink = array.Elements()
		}
	})
	b.Run("Elements64/Interface", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			interfaceChildrenSink = arrayView.Elements()
		}
	})
}

// Escaping results use the same interface sink in all three paths, so boxing
// and retained construction are common costs. Consume variants intentionally
// expose escape-analysis differences when a result is only used locally.
func BenchmarkValueFactoryInterface(b *testing.B) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	float32Type := fixtureValue(types.NewFloatType(types.Float32, foundation.NonNull))
	textType := fixtureValue(types.NewVarCharType(nil, foundation.NonNull))
	floating := fixtureValue(types.NewFloatType(types.Float64, foundation.NonNull))
	arrayType := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	child := fixtureValue(testbedvalues.NewInteger(integer, 42))
	children := []valueinterfaces.Value{child, child, child, child}
	concrete := testbedvalues.ValueFactory{}
	dynamic := opaqueFactory(concrete)
	b.Run("Integer/Escape/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewInteger(integer, int64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Integer/Escape/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewInteger(integer, int64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Integer/Escape/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewInteger(integer, int64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Float64/Escape/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewFloat(floating, float64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Float64/Escape/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewFloat(floating, float64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Float64/Escape/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewFloat(floating, float64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Array4/Escape/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewArray(arrayType, children)
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Array4/Escape/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewArray(arrayType, children)
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Array4/Escape/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewArray(arrayType, children)
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})
	b.Run("Integer/Consume/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewInteger(integer, int64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceIntegerSink = v.Integer()
		}
	})
	b.Run("Integer/Consume/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewInteger(integer, int64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceIntegerSink = v.(valueinterfaces.IntegerValue).Integer()
		}
	})
	b.Run("Integer/Consume/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewInteger(integer, int64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceIntegerSink = v.(valueinterfaces.IntegerValue).Integer()
		}
	})
	b.Run("Float32/Escape/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewFloat(float32Type, float64(i%1024))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})

	b.Run("Float32/Escape/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewFloat(float32Type, float64(i%1024))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})

	b.Run("Float32/Escape/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewFloat(float32Type, float64(i%1024))
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})

	b.Run("Float32/Consume/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewFloat(float32Type, float64(i%1024))
			if err != nil {
				b.Fatal(err)
			}
			interfaceFloatSink = v.Float()
		}
	})

	b.Run("Float32/Consume/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewFloat(float32Type, float64(i%1024))
			if err != nil {
				b.Fatal(err)
			}
			interfaceFloatSink = v.(valueinterfaces.FloatValue).Float()
		}
	})

	b.Run("Float32/Consume/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewFloat(float32Type, float64(i%1024))
			if err != nil {
				b.Fatal(err)
			}
			interfaceFloatSink = v.(valueinterfaces.FloatValue).Float()
		}
	})

	b.Run("VarChar/Escape/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewVarChar(textType, "payload")
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})

	b.Run("VarChar/Escape/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewVarChar(textType, "payload")
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})

	b.Run("VarChar/Escape/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewVarChar(textType, "payload")
			if err != nil {
				b.Fatal(err)
			}
			interfaceValueSink = v
		}
	})

	b.Run("VarChar/Consume/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewVarChar(textType, "payload")
			if err != nil {
				b.Fatal(err)
			}
			interfaceStringSink = v.VarChar()
		}
	})

	b.Run("VarChar/Consume/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewVarChar(textType, "payload")
			if err != nil {
				b.Fatal(err)
			}
			interfaceStringSink = v.(valueinterfaces.VarCharValue).VarChar()
		}
	})

	b.Run("VarChar/Consume/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewVarChar(textType, "payload")
			if err != nil {
				b.Fatal(err)
			}
			interfaceStringSink = v.(valueinterfaces.VarCharValue).VarChar()
		}
	})

	b.Run("Float64/Consume/Constructor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := testbedvalues.NewFloat(floating, float64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceFloatSink = v.Float()
		}
	})

	b.Run("Float64/Consume/ConcreteFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := concrete.NewFloat(floating, float64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceFloatSink = v.(valueinterfaces.FloatValue).Float()
		}
	})

	b.Run("Float64/Consume/InterfaceFactory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := dynamic.NewFloat(floating, float64(i))
			if err != nil {
				b.Fatal(err)
			}
			interfaceFloatSink = v.(valueinterfaces.FloatValue).Float()
		}
	})
}
