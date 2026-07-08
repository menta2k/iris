package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// maxInjectBody caps the injection request body (HTML mail can be large but is
// bounded); requests above this are rejected to protect the listener.
const maxInjectBody = 25 << 20 // 25 MiB

// MailInjector is the injection use case the handler drives.
type MailInjector interface {
	Inject(ctx context.Context, req *biz.GAInjectRequest) error
}

// NewInjectionServer builds the DEDICATED injection listener: a separate HTTP
// server on its own port (conf.Injection.Addr), carrying ONLY the
// GreenArrow-compatible injection route and a health check. It deliberately
// omits the admin service and the JWT auth middleware — callers authenticate
// with the body credentials — so this surface can be firewalled independently
// of the admin API. When tlsConf is non-nil the listener serves HTTPS. Returns
// nil when injection is disabled.
func NewInjectionServer(c conf.Injection, uc MailInjector, tlsConf *tls.Config, log *slog.Logger) *kratoshttp.Server {
	if !c.Enabled {
		return nil
	}
	path := c.Path
	if path == "" {
		path = "/api/inject"
	}

	opts := []kratoshttp.ServerOption{
		kratoshttp.Address(c.Addr),
		kratoshttp.Middleware(recovery.Recovery()),
	}
	if c.Timeout > 0 {
		opts = append(opts, kratoshttp.Timeout(c.Timeout))
	}
	if tlsConf != nil {
		opts = append(opts, kratoshttp.TLSConfig(tlsConf))
	}
	srv := kratoshttp.NewServer(opts...)

	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	srv.HandleFunc(path, injectHandler(uc, log))

	scheme := "http"
	if tlsConf != nil {
		scheme = "https"
	}
	log.Info("injection listener enabled", "addr", c.Addr, "path", path, "scheme", scheme)
	return srv
}

// injectHandler decodes a GreenArrow injection request, forwards it via the use
// case, and replies with the GreenArrow success envelope. The response body is
// always {success:1} or {success:0, error:"…"} so existing GreenArrow clients
// work unchanged; the HTTP status additionally reflects the outcome.
func injectHandler(uc MailInjector, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, biz.GAResponse{Success: 0, Error: "method not allowed"})
			return
		}
		var req biz.GAInjectRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxInjectBody))
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, biz.GAResponse{Success: 0, Error: "invalid request body: " + err.Error()})
			return
		}

		if err := uc.Inject(r.Context(), &req); err != nil {
			status, msg := injectErrorResponse(err)
			// Log server-side detail but never echo internals to the caller.
			log.Warn("injection failed", "error", err.Error(), "status", status)
			writeJSON(w, status, biz.GAResponse{Success: 0, Error: msg})
			return
		}
		writeJSON(w, http.StatusOK, biz.GAResponse{Success: 1})
	}
}

// injectErrorResponse maps a domain error to an HTTP status and a client-safe
// message.
func injectErrorResponse(err error) (int, string) {
	de, ok := biz.AsDomainError(err)
	if !ok {
		return http.StatusInternalServerError, "internal error"
	}
	switch de.Kind {
	case biz.KindUnauthorized:
		return http.StatusUnauthorized, de.Message
	case biz.KindForbidden:
		return http.StatusForbidden, de.Message
	case biz.KindInvalidArgument:
		return http.StatusBadRequest, de.Message
	case biz.KindNotFound:
		return http.StatusNotFound, de.Message
	case biz.KindUnavailable:
		return http.StatusBadGateway, strings.TrimSpace(de.Message)
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
