// Package service implements the Iris admin API handlers for both HTTP and gRPC
// transports, delegating business logic to the biz use cases.
package service

import (
	"github.com/go-kratos/kratos/v2/errors"

	"github.com/menta2k/iris/backend/internal/biz"
)

// mapError converts a domain or arbitrary error into a Kratos transport error,
// which carries an HTTP status and a gRPC code consistently across transports.
// It never leaks internal detail for unexpected errors.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	de, ok := biz.AsDomainError(err)
	if !ok {
		return errors.InternalServer("INTERNAL", "internal error")
	}
	switch de.Kind {
	case biz.KindInvalidArgument:
		return errors.BadRequest(de.Reason, de.Message)
	case biz.KindNotFound:
		return errors.NotFound(de.Reason, de.Message)
	case biz.KindConflict:
		return errors.Conflict(de.Reason, de.Message)
	case biz.KindUnauthorized:
		return errors.Unauthorized(de.Reason, de.Message)
	case biz.KindForbidden:
		return errors.Forbidden(de.Reason, de.Message)
	case biz.KindFailedPrecondition:
		return errors.New(412, de.Reason, de.Message)
	case biz.KindUnavailable:
		return errors.ServiceUnavailable(de.Reason, de.Message)
	default:
		// Internal errors are logged upstream; return a generic message.
		return errors.InternalServer("INTERNAL", "internal error")
	}
}
