// HTTP handlers for the login firewall (/v1/login-policies). Hand-rolled
// JSON, same pattern as registrar_listeners.go. JSON field names mirror the
// login_policy.proto json_name values (camelCase).
package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
	identitymw "github.com/menta2k/iris/backend/pkg/middleware/auth"
)

type httpTimeWindow struct {
	Days     []uint32 `json:"days"`
	Start    string   `json:"start"`
	End      string   `json:"end"`
	Timezone string   `json:"timezone"`
}

// httpLoginPolicy is the wire shape. Enabled is a pointer so an absent field
// on create defaults to true (proto3 bool zero-value would otherwise force
// false).
type httpLoginPolicy struct {
	ID         uint32          `json:"id,omitempty"`
	TargetID   uint32          `json:"targetId,omitempty"`
	Type       string          `json:"type"`
	Method     string          `json:"method"`
	Value      string          `json:"value,omitempty"`
	TimeWindow *httpTimeWindow `json:"timeWindow,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Enabled    *bool           `json:"enabled,omitempty"`
	CreatedBy  uint32          `json:"createdBy,omitempty"`
	UpdatedBy  uint32          `json:"updatedBy,omitempty"`
	CreatedAt  time.Time       `json:"createdAt,omitempty"`
	UpdatedAt  time.Time       `json:"updatedAt,omitempty"`
	DeletedAt  *time.Time      `json:"deletedAt,omitempty"`
}

type httpLoginPolicyListResp struct {
	Items []httpLoginPolicy `json:"items"`
	Total int               `json:"total"`
}

func registerLoginPolicies(hs *kratoshttp.Server, s *service.LoginPolicyService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/authentication.service.v1.LoginPolicyService/Create",
		resourceType:    "login_policy",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/authentication.service.v1.LoginPolicyService/Update",
		resourceType:    "login_policy",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodPut, http.MethodDelete},
	})

	hs.HandleFunc("/v1/login-policies", collAudit(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit, offset := paginationParams(r, 100, 1000)
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			rows, total, err := s.List(ctx, limit, offset)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			items := make([]httpLoginPolicy, 0, len(rows))
			for i := range rows {
				items = append(items, loginPolicyToHTTP(&rows[i]))
			}
			writeJSON(w, http.StatusOK, httpLoginPolicyListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpLoginPolicy
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			uid, _ := identitymw.IdentityFunc(r.Context())
			row, err := s.Create(ctx, httpToLoginPolicyRow(body), uid, clientIP(r), acknowledged(r))
			if err != nil {
				writeLoginPolicyErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, loginPolicyToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))

	hs.HandleFunc("/v1/login-policies/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		id := uint32(id64)
		switch r.Method {
		case http.MethodGet:
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			row, err := s.Get(ctx, id)
			if err != nil {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, loginPolicyToHTTP(row))
		case http.MethodPut:
			var body httpLoginPolicy
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			uid, _ := identitymw.IdentityFunc(r.Context())
			row, err := s.Update(ctx, id, httpToLoginPolicyRow(body), uid, clientIP(r), acknowledged(r))
			if err != nil {
				writeLoginPolicyErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, loginPolicyToHTTP(row))
		case http.MethodDelete:
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			uid, _ := identitymw.IdentityFunc(r.Context())
			if err := s.Delete(ctx, id, uid); err != nil {
				writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET, PUT or DELETE")
		}
	}))
}

// acknowledged reports whether the caller accepted the self-lockout risk.
func acknowledged(r *http.Request) bool {
	return r.URL.Query().Get("acknowledge") == "true"
}

// writeLoginPolicyErr maps the self-lockout guard to 409 (so the SPA can
// prompt "apply anyway") and everything else (validation) to 400.
func writeLoginPolicyErr(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrWouldLockOutSelf) {
		writeErr(w, http.StatusConflict, "WOULD_LOCK_OUT_SELF", err.Error())
		return
	}
	writeErr(w, http.StatusBadRequest, "SAVE_FAILED", err.Error())
}

func loginPolicyToHTTP(r *service.LoginPolicyRow) httpLoginPolicy {
	enabled := r.Enabled
	out := httpLoginPolicy{
		ID:        r.ID,
		TargetID:  r.TargetID,
		Type:      r.Type,
		Method:    r.Method,
		Value:     r.Value,
		Reason:    r.Reason,
		Enabled:   &enabled,
		CreatedBy: r.CreatedBy,
		UpdatedBy: r.UpdatedBy,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		DeletedAt: r.DeletedAt,
	}
	if r.TimeWindow != nil {
		days := make([]uint32, 0, len(r.TimeWindow.Days))
		for _, d := range r.TimeWindow.Days {
			days = append(days, uint32(d))
		}
		out.TimeWindow = &httpTimeWindow{
			Days:     days,
			Start:    r.TimeWindow.Start,
			End:      r.TimeWindow.End,
			Timezone: r.TimeWindow.Timezone,
		}
	}
	return out
}

func httpToLoginPolicyRow(h httpLoginPolicy) service.LoginPolicyRow {
	enabled := true // absent => active
	if h.Enabled != nil {
		enabled = *h.Enabled
	}
	row := service.LoginPolicyRow{
		TargetID: h.TargetID,
		Type:     h.Type,
		Method:   h.Method,
		Value:    h.Value,
		Reason:   h.Reason,
		Enabled:  enabled,
	}
	if h.TimeWindow != nil {
		days := make([]time.Weekday, 0, len(h.TimeWindow.Days))
		for _, d := range h.TimeWindow.Days {
			days = append(days, time.Weekday(d))
		}
		row.TimeWindow = &service.TimeWindow{
			Days:     days,
			Start:    h.TimeWindow.Start,
			End:      h.TimeWindow.End,
			Timezone: h.TimeWindow.Timezone,
		}
	}
	return row
}
