// HTTP handlers for the admin-CRUD slices that don't yet have proto HTTP
// annotations: /v1/users and /v1/audit. Same hand-rolled JSON pattern as the
// auth handlers in registrar.go — see the comment block at the top of that
// file for context. When the protos finally grow `option (google.api.http)`
// these can be replaced by generated gateway code.
package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// RegisterAdminHTTP mounts /v1/users/* and /v1/audit on the HTTP server.
// auditWrite is non-nil at runtime; it is the WriteFunc plumbed in from DI.
func RegisterAdminHTTP(hs *kratoshttp.Server, users *service.UserService, audit *service.AuditService, write auditmw.WriteFunc) {
	usersCollAudit := httpAudit(write, httpAuditConfig{
		operation:       "/identity.service.v1.UserService/Create",
		resourceType:    "user",
		mutatingMethods: []string{http.MethodPost},
	})
	userItemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/identity.service.v1.UserService/Delete",
		resourceType:    "user",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodDelete, http.MethodPatch, http.MethodPut},
	})
	hs.HandleFunc("/v1/users", usersCollAudit(usersCollectionHandler(users)))
	hs.HandleFunc("/v1/users/{id:[0-9]+}", userItemAudit(userItemHandler(users)))
	hs.HandleFunc("/v1/audit", auditListHandler(audit))
}

// --- /v1/users -------------------------------------------------------------

type httpUserItem struct {
	ID          uint32     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name,omitempty"`
	Active      bool       `json:"active"`
	Roles       []string   `json:"roles"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type httpUserListResp struct {
	Items []httpUserItem `json:"items"`
	Total uint32         `json:"total"`
}

type httpUserCreateReq struct {
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Password    string   `json:"password"`
	Roles       []string `json:"roles"`
}

func usersCollectionHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listUsers(w, r, users)
		case http.MethodPost:
			createUser(w, r, users)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}
}

func userItemHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseIDParam(w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			row, err := users.Get(r.Context(), id)
			if err != nil {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, userRowToHTTP(row))
		case http.MethodDelete:
			if err := users.Delete(r.Context(), id); err != nil {
				writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or DELETE")
		}
	}
}

func listUsers(w http.ResponseWriter, r *http.Request, users *service.UserService) {
	limit, offset := paginationParams(r, 100, 1000)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, total, err := users.List(ctx, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}
	items := make([]httpUserItem, 0, len(rows))
	for i := range rows {
		items = append(items, userRowToHTTP(&rows[i]))
	}
	writeJSON(w, http.StatusOK, httpUserListResp{Items: items, Total: total})
}

func createUser(w http.ResponseWriter, r *http.Request, users *service.UserService) {
	var body httpUserCreateReq
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	row, err := users.Create(ctx, &service.CreateUserRequest{
		Username:    body.Username,
		Email:       body.Email,
		DisplayName: body.DisplayName,
		Password:    body.Password,
		Roles:       body.Roles,
	})
	if err != nil {
		writeErr(w, mapUserErr(err), "CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, userRowToHTTP(row))
}

func userRowToHTTP(r *service.UserRow) httpUserItem {
	return httpUserItem{
		ID:          r.ID,
		Username:    r.Username,
		Email:       r.Email,
		DisplayName: r.DisplayName,
		Active:      r.Active,
		Roles:       r.Roles,
		LastLoginAt: r.LastLoginAt,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// mapUserErr maps service-layer validation errors to 4xx; everything else is
// treated as 5xx. The user.go service emits these via errors.Is-friendly
// sentinels so we can distinguish them.
func mapUserErr(err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidUsername),
		errors.Is(err, service.ErrInvalidEmail),
		errors.Is(err, service.ErrPasswordWeak):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// --- /v1/audit -------------------------------------------------------------

type httpAuditItem struct {
	ID            int64     `json:"id"`
	At            time.Time `json:"at"`
	Operation     string    `json:"operation"`
	ResourceType  string    `json:"resource_type"`
	ResourceID    string    `json:"resource_id"`
	ActorUserID   uint32    `json:"actor_user_id"`
	ActorUsername string    `json:"actor_username"`
	ClientIP      string    `json:"client_ip"`
	UserAgent     string    `json:"user_agent"`
	RequestID     string    `json:"request_id"`
	StatusCode    int32     `json:"status_code"`
	StatusMessage string    `json:"status_message"`
	DurationMS    int64     `json:"duration_ms"`
}

type httpAuditListResp struct {
	Items []httpAuditItem `json:"items"`
	Total uint32          `json:"total"`
}

func auditListHandler(audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		limit, offset := paginationParams(r, 200, 1000)
		in := service.AuditListInput{
			Operation: strings.TrimSpace(r.URL.Query().Get("operation")),
			Limit:     limit,
			Offset:    offset,
		}
		if v := r.URL.Query().Get("actor_user_id"); v != "" {
			id, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_PARAM", "actor_user_id must be uint")
				return
			}
			in.ActorUserID = uint32(id)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, total, err := audit.List(ctx, in)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
			return
		}
		items := make([]httpAuditItem, 0, len(rows))
		for _, e := range rows {
			items = append(items, httpAuditItem{
				ID:            e.ID,
				At:            e.At,
				Operation:     e.Operation,
				ResourceType:  e.ResourceType,
				ResourceID:    e.ResourceID,
				ActorUserID:   e.ActorUserID,
				ActorUsername: e.ActorUsername,
				ClientIP:      e.ClientIP,
				UserAgent:     e.UserAgent,
				RequestID:     e.RequestID,
				StatusCode:    e.StatusCode,
				StatusMessage: e.StatusMessage,
				DurationMS:    e.DurationMS,
			})
		}
		writeJSON(w, http.StatusOK, httpAuditListResp{Items: items, Total: total})
	}
}

// --- helpers ---------------------------------------------------------------

func parseIDParam(w http.ResponseWriter, r *http.Request) (uint32, bool) {
	raw := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id must be a positive integer")
		return 0, false
	}
	return uint32(id), true
}

func paginationParams(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	q := r.URL.Query()
	limit := defaultLimit
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
