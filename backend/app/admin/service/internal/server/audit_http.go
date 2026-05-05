// HTTP-side audit wrapper. The kratos middleware chain only sees operations
// that flow through generated service stubs, so the hand-rolled HandleFunc
// routes (auth, users, audit) are invisible to it. This file plugs that gap
// by providing a per-route wrapper that emits an audit.Entry directly.
//
// Once protos grow `option (google.api.http)` annotations and the hand-rolled
// handlers are replaced by generated stubs, this whole file can be deleted.
package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	identitymw "github.com/menta2k/iris/backend/pkg/middleware/auth"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// httpAuditConfig describes how to audit one route.
type httpAuditConfig struct {
	// operation is the canonical name written to audit_entry.operation. Use
	// the same `/<service>/<Method>` shape kratos emits so logs unify.
	operation string
	// resourceType is recorded on the entry; resourceID is filled at request
	// time from the {id}/{name} path variable (if any).
	resourceType string
	// resourceVar is the gorilla-mux path variable to read for the resource
	// id, e.g. "id" or "name". Empty string skips the lookup.
	resourceVar string
	// methods that count as mutating. Reads are not audited at all (matches
	// the gRPC-side IsMutating predicate to keep the table compact).
	mutatingMethods []string
}

// httpAudit returns a wrapper that records an audit row after the handler
// runs. Read operations and unmatched methods pass through untouched.
func httpAudit(write auditmw.WriteFunc, cfg httpAuditConfig) func(http.HandlerFunc) http.HandlerFunc {
	if write == nil {
		return func(h http.HandlerFunc) http.HandlerFunc { return h }
	}
	return func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !methodIsMutating(r.Method, cfg.mutatingMethods) {
				h(w, r)
				return
			}
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			h(rec, r)
			uid, uname := identitymw.IdentityFunc(r.Context())
			resID := ""
			if cfg.resourceVar != "" {
				resID = mux.Vars(r)[cfg.resourceVar]
			}
			entry := &auditmw.Entry{
				At:            start.UTC(),
				Operation:     cfg.operation,
				ResourceType:  cfg.resourceType,
				ResourceID:    resID,
				ActorUserID:   uid,
				ActorUsername: uname,
				ClientIP:      clientIP(r),
				UserAgent:     r.Header.Get("User-Agent"),
				RequestID:     firstNonEmpty(r.Header.Get("X-Request-Id"), r.Header.Get("Request-Id")),
				StatusCode:    int32(rec.status),
				DurationMS:    time.Since(start).Milliseconds(),
			}
			_ = write(r.Context(), entry)
		}
	}
}

// statusRecorder captures the response status code so the audit entry can
// record it. We deliberately do not buffer the body — it can be megabytes
// (logs, audit reads) and would balloon memory.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wroteHeader {
		return
	}
	s.status = code
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

func methodIsMutating(method string, allow []string) bool {
	for _, m := range allow {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-For"); h != "" {
		if i := strings.IndexByte(h, ','); i > 0 {
			return strings.TrimSpace(h[:i])
		}
		return strings.TrimSpace(h)
	}
	if h := r.Header.Get("X-Real-Ip"); h != "" {
		return h
	}
	if i := strings.LastIndexByte(r.RemoteAddr, ':'); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
