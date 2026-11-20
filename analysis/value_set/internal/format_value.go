package internal

import (
	"strconv"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// FormatValue renders a supported non-null scalar payload without its Type.
// Floats use their declared width and both signed zeros print as 0; strings use
// Go quoting to preserve arbitrary bytes. Formatting leaves Values unchanged.
// Inputs must be valid Bool, integer, float or VARCHAR Values; misuse asserts.
func FormatValue(value valueinterfaces.Value) string {
	assertions.NotNil(value)
	if value.IsNull() {
		panic("value_set: cannot format a null set element")
	}
	switch typ := value.Type().(type) {
	case *types.BoolType:
		v := value.(valueinterfaces.BoolValue)
		return strconv.FormatBool(v.Bool())
	case *types.IntType:
		v := value.(valueinterfaces.IntegerValue)
		return strconv.FormatInt(v.Integer(), 10)
	case *types.FloatType:
		v := value.(valueinterfaces.FloatValue)
		if v.Float() == 0 {
			return "0"
		}
		return strconv.FormatFloat(v.Float(), 'g', -1, int(typ.FloatWidth()))
	case *types.VarCharType:
		v := value.(valueinterfaces.VarCharValue)
		return strconv.Quote(v.VarChar())
	default:
		panic("value_set: unsupported formatting domain")
	}
}
