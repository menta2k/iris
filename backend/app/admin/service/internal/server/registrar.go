// Service registration onto the transport servers.
//
// The proto did not declare `option (google.api.http)` annotations, so the
// kratos generator did not produce HTTP stubs. We register the
// AuthenticationService on the gRPC server (proto-driven) and add a
// hand-rolled JSON route on the HTTP server (`POST /v1/auth/{login,refresh,logout}`)
// that calls the same gRPC adapter.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc/status"

	authenticationpb "github.com/menta2k/iris/backend/api/gen/go/authentication/service/v1"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// RegisterServices wires every service onto both the gRPC server and the
// HTTP server. The auditWrite func is plumbed into the HTTP handlers so the
// hand-rolled routes (which bypass kratos middleware) can still emit audit
// entries — see audit_http.go for the per-route wrapper.
func RegisterServices(
	gs *kratosgrpc.Server,
	hs *kratoshttp.Server,
	auth *service.AuthenticationGRPC,
	users *service.UserService,
	audit *service.AuditService,
	queues *service.QueueService,
	suppressions *service.SuppressionService,
	vmtas *service.VirtualMtaService,
	routing *service.RoutingService,
	dkim *service.DkimService,
	feedback *service.FeedbackService,
	logs *service.LogService,
	policy *service.PolicyService,
	mailClasses *service.MailClassService,
	vmtaGroups *service.VmtaGroupService,
	dashboard *service.DashboardService,
	dsns *service.DsnService,
	gsvc *service.GlobalSettingsService,
	listeners *service.ListenerService,
	acme *service.AcmeService,
	auditWrite auditmw.WriteFunc,
) {
	registerAuthGRPC(gs, auth)
	registerAuthHTTP(hs, auth, auditWrite)
	RegisterAdminHTTP(hs, users, audit, auditWrite)
	RegisterKumoHTTP(hs, queues, suppressions, vmtas, routing, dkim, feedback, logs, policy, mailClasses, vmtaGroups, dashboard, dsns, gsvc, listeners, acme, auditWrite)
	// SPA: must be registered LAST so /v1/* and /api/v1/* matchers above
	// take precedence. The fallback handler covers /, /assets/*, and
	// every client-side route the Vue router resolves at runtime.
	registerSPA(hs)
}

func registerAuthGRPC(gs *kratosgrpc.Server, auth *service.AuthenticationGRPC) {
	authenticationpb.RegisterAuthenticationServiceServer(gs, auth)
}

// registerAuthHTTP mounts the JSON endpoints. The body shape mirrors the
// proto field names (snake_case), matching what the SPA already sends.
func registerAuthHTTP(hs *kratoshttp.Server, auth *service.AuthenticationGRPC, write auditmw.WriteFunc) {
	// Both /v1/auth/login and /api/v1/auth/login are accepted because the
	// nginx proxy strips /api when upstream-ing. Local dev clients hitting
	// the admin service directly use /v1/...; SPA hits /api/v1/...
	loginAudit := httpAudit(write, httpAuditConfig{
		operation:       "/authentication.service.v1.AuthenticationService/Login",
		resourceType:    "user",
		mutatingMethods: []string{http.MethodPost},
	})
	refreshAudit := httpAudit(write, httpAuditConfig{
		operation:       "/authentication.service.v1.AuthenticationService/RefreshToken",
		resourceType:    "session",
		mutatingMethods: []string{http.MethodPost},
	})
	logoutAudit := httpAudit(write, httpAuditConfig{
		operation:       "/authentication.service.v1.AuthenticationService/Logout",
		resourceType:    "session",
		mutatingMethods: []string{http.MethodPost},
	})
	hs.HandleFunc("/v1/auth/login", loginAudit(loginHandler(auth)))
	hs.HandleFunc("/v1/auth/refresh", refreshAudit(refreshHandler(auth)))
	hs.HandleFunc("/v1/auth/logout", logoutAudit(logoutHandler()))
	hs.HandleFunc("/v1/auth/whoami", whoamiHandler(auth))
}

type httpLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type httpLoginResp struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	UserID       uint32   `json:"user_id"`
	Username     string   `json:"username"`
	Roles        []string `json:"roles"`
}

type httpRefreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

type httpErrResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const httpReqMaxBytes = 4 * 1024

func loginHandler(auth *service.AuthenticationGRPC) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		var body httpLoginReq
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		if body.Username == "" || body.Password == "" {
			writeErr(w, http.StatusBadRequest, "MISSING_FIELDS", "username and password required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		resp, err := auth.Login(ctx, &authenticationpb.LoginRequest{
			Username: body.Username,
			Password: body.Password,
		})
		if err != nil {
			httpErr, msg := mapStatus(err)
			writeErr(w, httpErr, "AUTH_FAILED", msg)
			return
		}
		var roles []string
		var uid uint32
		var uname string
		if resp.User != nil {
			roles = resp.User.Roles
			uid = resp.User.UserId
			uname = resp.User.Username
		}
		writeJSON(w, http.StatusOK, httpLoginResp{
			AccessToken:  resp.AccessToken,
			RefreshToken: resp.RefreshToken,
			ExpiresIn:    resp.ExpiresIn,
			UserID:       uid,
			Username:     uname,
			Roles:        roles,
		})
	}
}

func refreshHandler(auth *service.AuthenticationGRPC) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		var body httpRefreshReq
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		if body.RefreshToken == "" {
			writeErr(w, http.StatusBadRequest, "MISSING_FIELDS", "refresh_token required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		resp, err := auth.RefreshToken(ctx, &authenticationpb.RefreshTokenRequest{
			RefreshToken: body.RefreshToken,
		})
		if err != nil {
			httpErr, msg := mapStatus(err)
			writeErr(w, httpErr, "AUTH_FAILED", msg)
			return
		}
		var roles []string
		var uid uint32
		var uname string
		if resp.User != nil {
			roles = resp.User.Roles
			uid = resp.User.UserId
			uname = resp.User.Username
		}
		writeJSON(w, http.StatusOK, httpLoginResp{
			AccessToken:  resp.AccessToken,
			RefreshToken: resp.RefreshToken,
			ExpiresIn:    resp.ExpiresIn,
			UserID:       uid,
			Username:     uname,
			Roles:        roles,
		})
	}
}

func logoutHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func whoamiHandler(auth *service.AuthenticationGRPC) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		// Until the kratos auth middleware is mounted, this returns the
		// adapter's stub response — useful as a liveness probe.
		_, err := auth.Whoami(r.Context(), nil)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// decodeJSON enforces a size cap and rejects unknown fields.
func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, httpReqMaxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, statusCode int, code, message string) {
	writeJSON(w, statusCode, httpErrResp{Code: code, Message: message})
}

// mapStatus turns a grpc/status.Error into the right HTTP code.
func mapStatus(err error) (int, string) {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError, errString(err)
	}
	switch {
	case strings.Contains(st.Message(), "invalid credentials"):
		return http.StatusUnauthorized, st.Message()
	case strings.Contains(st.Message(), "inactive"):
		return http.StatusForbidden, st.Message()
	case strings.Contains(st.Message(), "locked"):
		return http.StatusTooManyRequests, st.Message()
	default:
		return http.StatusInternalServerError, st.Message()
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Force compile-time use of errors so future maintenance can wrap with %w.
var _ = errors.New
