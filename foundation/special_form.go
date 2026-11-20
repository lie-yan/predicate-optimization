package foundation

// SpecialForm identifies expression-level semantics and can grow with future
// forms. Currently only scalar IN is supported. Zero is invalid.
// Recognition does not guarantee evaluation support for every argument list.
type SpecialForm uint8

const (
	SpecialFormIn SpecialForm = iota + 1
)

func (form SpecialForm) IsValid() bool { return form == SpecialFormIn }

func (form SpecialForm) String() string {
	switch form {
	case SpecialFormIn:
		return "IN"
	default:
		return "INVALID"
	}
}
