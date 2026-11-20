package values

type nonNull struct{}

func (nonNull) IsNull() bool { return false }
