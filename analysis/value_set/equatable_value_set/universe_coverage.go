package equatable_value_set

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"

	"github.com/lie-yan/predicate-optimization/types"
)

// coversUniverse assumes distinct validated entries from typ's universe. Thus
// coverage is a cardinality comparison, without enumerating missing values.
func coversUniverse(typ types.Type, count uint64) (bool, error) {
	switch t := typ.(type) {
	case *types.BoolType:
		return count == 2, nil
	case *types.IntType:
		if t.IntegerWidth() == types.Int64 {
			// Its 2^64 values cannot fit in a Go slice or in count.
			return false, nil
		}
		return count == uint64(1)<<t.IntegerWidth(), nil
	case *types.FloatType:
		// Remove all NaN bit patterns, add one canonical NaN, and merge signed
		// zeros. For width w and fraction bits m: 2^w - 2^(m+1) + 2.
		if t.FloatWidth() == types.Float32 {
			return count == 0xff000002, nil
		}
		return count == 0xffe0000000000002, nil
	case *types.VarCharType:
		limit, bounded := t.MaxBytes()
		if !bounded || count == 0 {
			return false, nil
		}
		// Number of byte strings of length <= limit: 1 + 256 + ... + 256^limit.
		// Stop as soon as coverage is impossible. This also avoids overflow and
		// work proportional to arbitrarily large declared bounds.
		total := uint64(1)
		for length := uint64(0); length < limit; length++ {
			if total > (count-1)/256 {
				return false, nil
			}
			total = total*256 + 1
		}
		return count == total, nil
	default:
		return false, fmt.Errorf("%w: universe coverage for %T", value_set.ErrUnsupported, typ)
	}
}
