package biz

import (
	"errors"
	"fmt"
)

// ErrorKind classifies domain errors so the service layer can map them to the
// correct HTTP status and gRPC code without leaking internal detail.
type ErrorKind int

const (
	// KindInternal is an unexpected server-side failure.
	KindInternal ErrorKind = iota
	// KindInvalidArgument is a validation or bad-input failure.
	KindInvalidArgument
	// KindNotFound is a missing entity.
	KindNotFound
	// KindConflict is a uniqueness or state-conflict failure.
	KindConflict
	// KindUnauthorized means the caller is not authenticated.
	KindUnauthorized
	// KindForbidden means the caller lacks the required permission.
	KindForbidden
	// KindFailedPrecondition means a state requirement was not met.
	KindFailedPrecondition
	// KindUnavailable means a downstream dependency is unavailable.
	KindUnavailable
)

// DomainError is a typed error carrying a machine-readable reason and a kind.
type DomainError struct {
	Kind    ErrorKind
	Reason  string
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *DomainError) Unwrap() error { return e.Err }

// newErr builds a DomainError.
func newErr(kind ErrorKind, reason, format string, args ...any) *DomainError {
	return &DomainError{Kind: kind, Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// Invalid builds an invalid-argument error.
func Invalid(reason, format string, args ...any) *DomainError {
	return newErr(KindInvalidArgument, reason, format, args...)
}

// NotFound builds a not-found error.
func NotFound(reason, format string, args ...any) *DomainError {
	return newErr(KindNotFound, reason, format, args...)
}

// Conflict builds a conflict error.
func Conflict(reason, format string, args ...any) *DomainError {
	return newErr(KindConflict, reason, format, args...)
}

// Forbidden builds a permission-denied error.
func Forbidden(reason, format string, args ...any) *DomainError {
	return newErr(KindForbidden, reason, format, args...)
}

// Unauthorized builds an unauthenticated error.
func Unauthorized(reason, format string, args ...any) *DomainError {
	return newErr(KindUnauthorized, reason, format, args...)
}

// FailedPrecondition builds a failed-precondition error.
func FailedPrecondition(reason, format string, args ...any) *DomainError {
	return newErr(KindFailedPrecondition, reason, format, args...)
}

// Unavailable builds a dependency-unavailable error.
func Unavailable(reason, format string, args ...any) *DomainError {
	return newErr(KindUnavailable, reason, format, args...)
}

// Internal wraps an unexpected error.
func Internal(err error, format string, args ...any) *DomainError {
	return &DomainError{Kind: KindInternal, Reason: "INTERNAL", Message: fmt.Sprintf(format, args...), Err: err}
}

// AsDomainError extracts a *DomainError from err, if present.
func AsDomainError(err error) (*DomainError, bool) {
	var de *DomainError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}
