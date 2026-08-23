package autherr

import "errors"

// CredentialsError signals Miru has no usable credentials — missing, expired, or revoked.
// Callers that can recover (the MCP auth tool, miru setup) branch on this instead of
// matching error text.
//
// Kept in a dependency-free package so auth, env, embed, and mcp can share it without
// import cycles.
type CredentialsError struct {
	Message string
	Cause   error
}

func (e *CredentialsError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *CredentialsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// New builds a CredentialsError.
func New(message string, cause error) *CredentialsError {
	return &CredentialsError{Message: message, Cause: cause}
}

// Is reports whether err, or anything in its Unwrap chain, is a CredentialsError.
func Is(err error) bool {
	var ce *CredentialsError
	return errors.As(err, &ce)
}
