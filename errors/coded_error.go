package errors

// CodedError exposes a precise failure code without rendering a diagnostic.
// Standard errors.As can find the first coded error in an error tree. That code
// does not describe all causes or determine precedence among joined categories;
// use standard errors.Is for behavioral classification.
type CodedError interface {
	error
	Code() Code
}
