// HTTP handlers for MFA (/v1/auth/mfa/*). Hand-rolled JSON, same style as the
// other registrars. Two groups:
//   - self-service (Bearer access token): enroll/manage a user's own factors.
//   - login step (mfa_token from the password response): verify a factor and
//     receive tokens. These are public (no access token yet).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

var errMFAUnauthorized = errors.New("mfa: missing or invalid access token")

func registerMFA(hs *kratoshttp.Server, mfa *service.MFAService, issuer *appjwt.Issuer, write auditmw.WriteFunc) {
	audit := func(op string) func(http.HandlerFunc) http.HandlerFunc {
		return httpAudit(write, httpAuditConfig{
			operation:       op,
			resourceType:    "mfa",
			mutatingMethods: []string{http.MethodPost, http.MethodDelete},
		})
	}

	// ----- self-service (Bearer-authed) -----
	hs.HandleFunc("/v1/auth/mfa", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		uid, _, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		st, err := mfa.Status(ctx, uid)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "MFA_STATUS_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, mfaStatusToHTTP(st))
	})

	hs.HandleFunc("/v1/auth/mfa/totp/enroll", audit("/iris.admin/MFA/EnrollTOTP")(func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		uid, username, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		res, err := mfa.EnrollTOTPStart(ctx, uid, username)
		if err != nil {
			writeMFAErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"secret": res.Secret, "otpauth_url": res.OTPAuthURL,
			"qr_code_data_uri": res.QRCodeDataURI, "operation_id": res.OperationID,
		})
	}))

	hs.HandleFunc("/v1/auth/mfa/totp/confirm", audit("/iris.admin/MFA/ConfirmTOTP")(func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		uid, _, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		var body struct {
			OperationID string `json:"operation_id"`
			Code        string `json:"code"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		codes, err := mfa.EnrollTOTPConfirm(ctx, uid, body.OperationID, body.Code)
		if err != nil {
			writeMFAErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
	}))

	hs.HandleFunc("/v1/auth/mfa/passkey/enroll/start", audit("/iris.admin/MFA/EnrollPasskeyStart")(func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		uid, username, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		options, opID, err := mfa.EnrollPasskeyStart(ctx, uid, username)
		if err != nil {
			writeMFAErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"options": json.RawMessage(options), "operation_id": opID,
		})
	}))

	hs.HandleFunc("/v1/auth/mfa/passkey/enroll/finish", audit("/iris.admin/MFA/EnrollPasskeyFinish")(func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		uid, username, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		var body struct {
			OperationID string          `json:"operation_id"`
			Response    json.RawMessage `json:"response"`
			Label       string          `json:"label"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		if err := mfa.EnrollPasskeyFinish(ctx, uid, username, body.OperationID, string(body.Response), body.Label); err != nil {
			writeMFAErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	hs.HandleFunc("/v1/auth/mfa/passkey/{id:[0-9]+}", audit("/iris.admin/MFA/RemovePasskey")(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE")
			return
		}
		uid, _, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		ctx, cancel := mfaCtx(r)
		defer cancel()
		if err := mfa.RemovePasskey(ctx, uid, uint32(id64)); err != nil {
			writeMFAErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	hs.HandleFunc("/v1/auth/mfa/backup-codes/regenerate", audit("/iris.admin/MFA/RegenerateBackupCodes")(func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		uid, _, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		codes, err := mfa.RegenerateBackupCodes(ctx, uid)
		if err != nil {
			writeMFAErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
	}))

	hs.HandleFunc("/v1/auth/mfa/disable", audit("/iris.admin/MFA/Disable")(func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		uid, _, err := mfaRequireUser(r, issuer)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		if err := mfa.Disable(ctx, uid); err != nil {
			writeMFAErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// ----- login step (public; authenticated by the mfa_token) -----
	hs.HandleFunc("/v1/auth/mfa/verify", func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		var body struct {
			MFAToken   string `json:"mfa_token"`
			Code       string `json:"code"`
			BackupCode string `json:"backup_code"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		resp, err := mfa.VerifyChallenge(ctx, body.MFAToken, body.Code, body.BackupCode, clientIP(r))
		if err != nil {
			writeMFAErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, loginRespToHTTP(resp))
	})

	hs.HandleFunc("/v1/auth/mfa/passkey/login/start", func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		var body struct {
			MFAToken string `json:"mfa_token"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		options, opID, err := mfa.WebAuthnLoginStart(ctx, body.MFAToken)
		if err != nil {
			writeMFAErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"options": json.RawMessage(options), "operation_id": opID,
		})
	})

	hs.HandleFunc("/v1/auth/mfa/passkey/login/finish", func(w http.ResponseWriter, r *http.Request) {
		if !mustPOST(w, r) {
			return
		}
		var body struct {
			OperationID string          `json:"operation_id"`
			Response    json.RawMessage `json:"response"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		ctx, cancel := mfaCtx(r)
		defer cancel()
		resp, err := mfa.WebAuthnLoginFinish(ctx, body.OperationID, string(body.Response), clientIP(r))
		if err != nil {
			writeMFAErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, loginRespToHTTP(resp))
	})
}

func mfaCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 10*time.Second)
}

// mfaRequireUser verifies the Bearer access token and returns the caller.
func mfaRequireUser(r *http.Request, issuer *appjwt.Issuer) (uint32, string, error) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return 0, "", errMFAUnauthorized
	}
	claims, err := issuer.VerifyAccess(strings.TrimSpace(h[len(prefix):]))
	if err != nil {
		return 0, "", errMFAUnauthorized
	}
	return claims.UserID, claims.Username, nil
}

func mustPOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return false
	}
	return true
}

type httpMFAStatus struct {
	TOTPEnabled     bool              `json:"totp_enabled"`
	WebAuthnEnabled bool              `json:"webauthn_enabled"`
	BackupRemaining int               `json:"backup_remaining"`
	Passkeys        []httpMFAPasskey  `json:"passkeys"`
}

type httpMFAPasskey struct {
	ID        uint32    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

func mfaStatusToHTTP(st *service.MFAStatus) httpMFAStatus {
	out := httpMFAStatus{
		TOTPEnabled:     st.TOTPEnabled,
		WebAuthnEnabled: st.WebAuthnEnabled,
		BackupRemaining: st.BackupRemaining,
		Passkeys:        make([]httpMFAPasskey, 0, len(st.Passkeys)),
	}
	for _, p := range st.Passkeys {
		out.Passkeys = append(out.Passkeys, httpMFAPasskey{ID: p.ID, Label: p.Label, CreatedAt: p.CreatedAt})
	}
	return out
}

func writeMFAErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrMFAInvalidCode), errors.Is(err, service.ErrMFAChallengeInvalid):
		writeErr(w, http.StatusUnauthorized, "MFA_INVALID", err.Error())
	case errors.Is(err, service.ErrMFAEnrollExpired):
		writeErr(w, http.StatusConflict, "MFA_ENROLL_EXPIRED", err.Error())
	case errors.Is(err, service.ErrMFANotConfigured), errors.Is(err, service.ErrMFAWebAuthnOff):
		writeErr(w, http.StatusServiceUnavailable, "MFA_NOT_CONFIGURED", err.Error())
	default:
		writeErr(w, http.StatusBadRequest, "MFA_FAILED", err.Error())
	}
}
