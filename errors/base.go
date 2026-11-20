package errors

import stderrs "errors"

// Base holds immutable classification and an optional cause. Domain errors embed
// it by value and override Error to format their own typed payload on demand.
// The zero value has an unknown code and no category or cause. Referenced errors
// are shared; their owners must preserve any required concurrency guarantees.
type Base struct {
	code     Code
	category error
	cause    error
}

var _ CodedError = Base{}

// NewBase stores its arguments without formatting or copying referenced errors.
// Categories should be leaf sentinels. A nil category or cause is allowed.
func NewBase(code Code, category error, cause error) Base {
	return Base{code: code, category: category, cause: cause}
}

// Code returns the precise failure identifier without traversing causes.
func (b Base) Code() Code { return b.code }

// Is matches the category only. Standard errors.Is traverses the cause via
// Unwrap. Codes are not error identities, and nil never matches a Base.
func (b Base) Is(target error) bool {
	return target != nil && b.category != nil && stderrs.Is(b.category, target)
}

// Unwrap retains the original cause, including its concrete type and error tree.
func (b Base) Unwrap() error { return b.cause }

// Error provides a static fallback name. Domain errors override this method to
// include diagnostic details. It does not format or inspect the cause.
func (b Base) Error() string { return b.code.String() }
