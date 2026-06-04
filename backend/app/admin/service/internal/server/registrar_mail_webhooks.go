// HTTP handlers for inbound-mail → HTTP webhooks (/v1/mail-webhooks).
// Hand-rolled JSON, same shape as registrar_listeners.go.
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

type httpMailWebhook struct {
	ID      uint32    `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
	URL     string    `json:"url"`
	// Secret is write-only: accepted on create/update, never returned. The
	// response reports only whether one is set.
	SecretSet bool      `json:"secret_set"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type httpMailWebhookListResp struct {
	Items []httpMailWebhook `json:"items"`
	Total uint32            `json:"total"`
}

type httpMailWebhookReq struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
	Enabled *bool  `json:"enabled"`
}

func registerMailWebhooks(hs *kratoshttp.Server, s *service.MailWebhookService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.MailWebhookService/Create",
		resourceType:    "mail_webhook",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.MailWebhookService/Update",
		resourceType:    "mail_webhook",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodPut, http.MethodDelete},
	})

	hs.HandleFunc("/v1/mail-webhooks", collAudit(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit, offset := paginationParams(r, 100, 1000)
			rows, total, err := s.List(r.Context(), limit, offset)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			items := make([]httpMailWebhook, 0, len(rows))
			for i := range rows {
				items = append(items, mailWebhookToHTTP(&rows[i]))
			}
			writeJSON(w, http.StatusOK, httpMailWebhookListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpMailWebhookReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			row, err := s.Create(r.Context(), webhookReqToRow(body, true))
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, mailWebhookToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))

	hs.HandleFunc("/v1/mail-webhooks/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		id := uint32(id64)
		switch r.Method {
		case http.MethodGet:
			row, err := s.Get(r.Context(), id)
			if err != nil {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, mailWebhookToHTTP(row))
		case http.MethodPut:
			var body httpMailWebhookReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			existing, err := s.Get(r.Context(), id)
			if err != nil {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			in := webhookReqToRow(body, false)
			in.Name = existing.Name // name is immutable
			// Blank secret on update = keep the stored one (write-only field).
			if body.Secret == "" {
				in.Secret = existing.Secret
			}
			row, err := s.Update(r.Context(), id, in)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "UPDATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, mailWebhookToHTTP(row))
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

func webhookReqToRow(b httpMailWebhookReq, create bool) *service.MailWebhookRow {
	enabled := true
	if b.Enabled != nil {
		enabled = *b.Enabled
	}
	row := &service.MailWebhookRow{
		Address: b.Address,
		URL:     b.URL,
		Secret:  b.Secret,
		Enabled: enabled,
	}
	if create {
		row.Name = b.Name
	}
	return row
}

func mailWebhookToHTTP(v *service.MailWebhookRow) httpMailWebhook {
	return httpMailWebhook{
		ID: v.ID, Name: v.Name, Address: v.Address, URL: v.URL,
		SecretSet: v.Secret != "", Enabled: v.Enabled,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}
