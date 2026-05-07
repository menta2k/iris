package server

import (
	"net/http"
	"strconv"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// httpListener is the wire shape for /v1/listeners. tls_cert_pem_path /
// tls_key_pem_path are the on-disk paths kumomta will read when TLS is
// enabled — the API does NOT accept inline PEM material here for the
// same reason it doesn't accept DKIM PEMs over the wire: ent-side
// storage of secret bytes is the wrong place for them.
type httpListenerItem struct {
	ID             uint32    `json:"id"`
	Name           string    `json:"name"`
	ListenAddr     string    `json:"listen_addr"`
	Hostname       string    `json:"hostname"`
	TLSEnabled     bool      `json:"tls_enabled"`
	TLSCertPath    string    `json:"tls_cert_pem_path,omitempty"`
	TLSKeyPath     string    `json:"tls_key_pem_path,omitempty"`
	RequireAuth    bool      `json:"require_auth"`
	MaxMessageSize uint64    `json:"max_message_size,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type httpListenerListResp struct {
	Items []httpListenerItem `json:"items"`
	Total uint32             `json:"total"`
}

type httpListenerCreateReq struct {
	Name           string `json:"name"`
	ListenAddr     string `json:"listen_addr"`
	Hostname       string `json:"hostname"`
	TLSEnabled     bool   `json:"tls_enabled"`
	TLSCertPath    string `json:"tls_cert_pem_path"`
	TLSKeyPath     string `json:"tls_key_pem_path"`
	RequireAuth    bool   `json:"require_auth"`
	MaxMessageSize uint64 `json:"max_message_size"`
}

// httpListenerUpdateReq is identical to create minus the name (immutable)
// — same payload shape so the SPA can reuse one form for both flows.
type httpListenerUpdateReq struct {
	ListenAddr     string `json:"listen_addr"`
	Hostname       string `json:"hostname"`
	TLSEnabled     bool   `json:"tls_enabled"`
	TLSCertPath    string `json:"tls_cert_pem_path"`
	TLSKeyPath     string `json:"tls_key_pem_path"`
	RequireAuth    bool   `json:"require_auth"`
	MaxMessageSize uint64 `json:"max_message_size"`
}

func registerListeners(hs *kratoshttp.Server, s *service.ListenerService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.ListenerService/Create",
		resourceType:    "listener",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.ListenerService/Update",
		resourceType:    "listener",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodPut, http.MethodDelete},
	})

	hs.HandleFunc("/v1/listeners", collAudit(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit, offset := paginationParams(r, 100, 1000)
			rows, total, err := s.List(r.Context(), limit, offset)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			items := make([]httpListenerItem, 0, len(rows))
			for i := range rows {
				items = append(items, listenerToHTTP(&rows[i]))
			}
			writeJSON(w, http.StatusOK, httpListenerListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpListenerCreateReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			row, err := s.Create(r.Context(), &service.ListenerRow{
				Name: body.Name, ListenAddr: body.ListenAddr, Hostname: body.Hostname,
				TLSEnabled: body.TLSEnabled, TLSCertPath: body.TLSCertPath, TLSKeyPath: body.TLSKeyPath,
				RequireAuth: body.RequireAuth, MaxMessageSize: body.MaxMessageSize,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, listenerToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))

	hs.HandleFunc("/v1/listeners/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		id := uint32(id64)
		switch r.Method {
		case http.MethodGet:
			row, err := s.Get(r.Context(), id)
			if err != nil {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, listenerToHTTP(row))
		case http.MethodPut:
			var body httpListenerUpdateReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			// Preserve the existing name — Update validates against it
			// (name is immutable but the validator still requires a
			// non-empty value to pass).
			existing, err := s.Get(r.Context(), id)
			if err != nil {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			row, err := s.Update(r.Context(), id, &service.ListenerRow{
				Name: existing.Name,
				ListenAddr: body.ListenAddr, Hostname: body.Hostname,
				TLSEnabled: body.TLSEnabled, TLSCertPath: body.TLSCertPath, TLSKeyPath: body.TLSKeyPath,
				RequireAuth: body.RequireAuth, MaxMessageSize: body.MaxMessageSize,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "UPDATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, listenerToHTTP(row))
		case http.MethodDelete:
			if err := s.Delete(r.Context(), id); err != nil {
				writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET, PUT or DELETE")
		}
	}))
}

func listenerToHTTP(v *service.ListenerRow) httpListenerItem {
	return httpListenerItem{
		ID: v.ID, Name: v.Name, ListenAddr: v.ListenAddr, Hostname: v.Hostname,
		TLSEnabled: v.TLSEnabled, TLSCertPath: v.TLSCertPath, TLSKeyPath: v.TLSKeyPath,
		RequireAuth: v.RequireAuth, MaxMessageSize: v.MaxMessageSize,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}
