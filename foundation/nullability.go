package foundation

// Nullability describes whether null belongs to a type's value domain.
// Its zero value conservatively permits null.
type Nullability uint8

const (
	MayNull Nullability = iota
	NonNull
)

// Suffix returns the type-display suffix: ! for NonNull and ? for MayNull.
// The receiver must be a valid Nullability.
func (n Nullability) Suffix() string {
	if n == NonNull {
		return "!"
	}
	return "?"
}
