// Per-domain HTTP handlers for the kumo slice. Each register* function is
// called by RegisterKumoHTTP in registrar_kumo.go. Pattern is the same
// across domains: list+create+delete/update with audit wrapping for
// mutating verbs.
package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	"github.com/menta2k/iris/backend/pkg/kumopolicy"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// --- /v1/suppressions ------------------------------------------------------

type httpSuppressionItem struct {
	ID        uint64     `json:"id"`
	Address   string     `json:"address"`
	Scope     string     `json:"scope"`
	Reason    string     `json:"reason"`
	Note      string     `json:"note,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type httpSuppressionListResp struct {
	Items []httpSuppressionItem `json:"items"`
	Total uint32                `json:"total"`
}

type httpSuppressionCreateReq struct {
	Address string `json:"address"`
	Scope   string `json:"scope"`
	Reason  string `json:"reason"`
	Note    string `json:"note"`
}

func registerSuppressions(hs *kratoshttp.Server, s *service.SuppressionService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.SuppressionService/Create",
		resourceType:    "suppression",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.SuppressionService/Delete",
		resourceType:    "suppression",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodDelete},
	})
	hs.HandleFunc("/v1/suppressions", collAudit(func(w http.ResponseWriter, r *http.Request) {
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
			items := make([]httpSuppressionItem, 0, len(rows))
			for _, r := range rows {
				items = append(items, suppToHTTP(&r))
			}
			writeJSON(w, http.StatusOK, httpSuppressionListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpSuppressionCreateReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			row, err := s.Create(ctx, &service.CreateInput{
				Address: body.Address, Scope: body.Scope,
				Reason: body.Reason, Note: body.Note,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, suppToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))
	hs.HandleFunc("/v1/suppressions/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE")
			return
		}
		id, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
		if err != nil || id == 0 {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "id must be a positive integer")
			return
		}
		if err := s.Delete(r.Context(), id); err != nil {
			writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func suppToHTTP(r *service.SuppressionRow) httpSuppressionItem {
	return httpSuppressionItem{
		ID: r.ID, Address: r.Address, Scope: r.Scope,
		Reason: r.Reason, Note: r.Note,
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt,
	}
}

// --- /v1/vmtas -------------------------------------------------------------

type httpVmtaItem struct {
	ID                       uint32    `json:"id"`
	Name                     string    `json:"name"`
	SourceIPs                []string  `json:"source_ips"`
	HeloName                 string    `json:"helo_name,omitempty"`
	MaxConnections           uint32    `json:"max_connections"`
	MaxMessagesPerConnection uint32    `json:"max_messages_per_connection"`
	ConnectTimeout           uint32    `json:"connect_timeout"`
	ProviderProfile          string    `json:"provider_profile"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type httpVmtaListResp struct {
	Items []httpVmtaItem `json:"items"`
	Total uint32         `json:"total"`
}

type httpVmtaCreateReq struct {
	Name                     string   `json:"name"`
	SourceIPs                []string `json:"source_ips"`
	HeloName                 string   `json:"helo_name"`
	MaxConnections           uint32   `json:"max_connections"`
	MaxMessagesPerConnection uint32   `json:"max_messages_per_connection"`
	ConnectTimeout           uint32   `json:"connect_timeout"`
	ProviderProfile          string   `json:"provider_profile"`
}

func registerVmtas(hs *kratoshttp.Server, s *service.VirtualMtaService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.VmtaService/Create",
		resourceType:    "vmta",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.VmtaService/Delete",
		resourceType:    "vmta",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodDelete},
	})
	hs.HandleFunc("/v1/vmtas", collAudit(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit, offset := paginationParams(r, 100, 1000)
			rows, total, err := s.List(r.Context(), limit, offset)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			items := make([]httpVmtaItem, 0, len(rows))
			for _, v := range rows {
				items = append(items, vmtaToHTTP(&v))
			}
			writeJSON(w, http.StatusOK, httpVmtaListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpVmtaCreateReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			row, err := s.Create(r.Context(), &service.VirtualMtaRow{
				Name: body.Name, SourceIPs: body.SourceIPs, HeloName: body.HeloName,
				MaxConnections:           body.MaxConnections,
				MaxMessagesPerConnection: body.MaxMessagesPerConnection,
				ConnectTimeout:           body.ConnectTimeout,
				ProviderProfile:          body.ProviderProfile,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, vmtaToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))
	hs.HandleFunc("/v1/vmtas/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE")
			return
		}
		id, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		if err := s.Delete(r.Context(), uint32(id)); err != nil {
			writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func vmtaToHTTP(v *service.VirtualMtaRow) httpVmtaItem {
	return httpVmtaItem{
		ID: v.ID, Name: v.Name, SourceIPs: v.SourceIPs, HeloName: v.HeloName,
		MaxConnections:           v.MaxConnections,
		MaxMessagesPerConnection: v.MaxMessagesPerConnection,
		ConnectTimeout:           v.ConnectTimeout,
		ProviderProfile:          v.ProviderProfile,
		CreatedAt:                v.CreatedAt,
		UpdatedAt:                v.UpdatedAt,
	}
}

// --- /v1/routing -----------------------------------------------------------

type httpRoutingItem struct {
	ID         uint32                     `json:"id"`
	Name       string                     `json:"name"`
	Priority   int32                      `json:"priority"`
	Enabled    bool                       `json:"enabled"`
	Conditions []kumopolicy.RuleCondition `json:"conditions"`
	Target     kumopolicy.RuleTarget      `json:"target"`
}

type httpRoutingListResp struct {
	Items []httpRoutingItem `json:"items"`
	Total uint32            `json:"total"`
}

type httpRoutingPatchReq struct {
	Enabled *bool `json:"enabled"`
}

func registerRouting(hs *kratoshttp.Server, s *service.RoutingService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.RoutingService/Create",
		resourceType:    "routing_rule",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.RoutingService/Update",
		resourceType:    "routing_rule",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodDelete, http.MethodPatch},
	})
	hs.HandleFunc("/v1/routing", collAudit(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit, offset := paginationParams(r, 100, 1000)
			rows, total, err := s.List(r.Context(), limit, offset)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			items := make([]httpRoutingItem, 0, len(rows))
			for _, ru := range rows {
				items = append(items, routingToHTTP(&ru))
			}
			writeJSON(w, http.StatusOK, httpRoutingListResp{Items: items, Total: total})
		case http.MethodPost:
			var body service.RoutingRow
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			row, err := s.Create(r.Context(), &body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, routingToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))
	hs.HandleFunc("/v1/routing/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		switch r.Method {
		case http.MethodDelete:
			if err := s.Delete(r.Context(), uint32(id)); err != nil {
				writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPatch:
			var body httpRoutingPatchReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			if body.Enabled == nil {
				writeErr(w, http.StatusBadRequest, "MISSING_FIELDS", "enabled field required")
				return
			}
			row, err := s.UpdateEnabled(r.Context(), uint32(id), *body.Enabled)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, routingToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE or PATCH")
		}
	}))
}

func routingToHTTP(r *service.RoutingRow) httpRoutingItem {
	return httpRoutingItem{
		ID: r.ID, Name: r.Name, Priority: r.Priority, Enabled: r.Enabled,
		Conditions: r.Conditions, Target: r.Target,
	}
}

// --- /v1/dkim --------------------------------------------------------------

type httpDkimItem struct {
	ID           uint32    `json:"id"`
	Domain       string    `json:"domain"`
	Selector     string    `json:"selector"`
	Algorithm    string    `json:"algorithm"`
	PublicKeyPEM string    `json:"public_key_pem"`
	KeyPath      string    `json:"key_path"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type httpDkimListResp struct {
	Items []httpDkimItem `json:"items"`
	Total uint32         `json:"total"`
}

type httpDkimCreateReq struct {
	Domain        string `json:"domain"`
	Selector      string `json:"selector"`
	Algorithm     string `json:"algorithm"`
	PrivateKeyPEM string `json:"private_key_pem"` // optional; if set, import instead of generate
}

func registerDkim(hs *kratoshttp.Server, s *service.DkimService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.DkimService/Create",
		resourceType:    "dkim",
		mutatingMethods: []string{http.MethodPost},
	})
	rotateAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.DkimService/Rotate",
		resourceType:    "dkim",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodPost},
	})
	hs.HandleFunc("/v1/dkim", collAudit(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit, offset := paginationParams(r, 100, 1000)
			rows, total, err := s.List(r.Context(), limit, offset)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			items := make([]httpDkimItem, 0, len(rows))
			for _, d := range rows {
				items = append(items, dkimToHTTP(&d))
			}
			writeJSON(w, http.StatusOK, httpDkimListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpDkimCreateReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			row, err := s.Create(ctx, &service.CreateDkimRequest{
				Domain:        body.Domain,
				Selector:      body.Selector,
				Algorithm:     body.Algorithm,
				PrivateKeyPEM: body.PrivateKeyPEM,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, dkimToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))
	hs.HandleFunc("/v1/dkim/{id:[0-9]+}/rotate", rotateAudit(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		id, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		row, err := s.Rotate(ctx, uint32(id))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "ROTATE_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, dkimToHTTP(row))
	}))
}

func dkimToHTTP(d *service.DkimRow) httpDkimItem {
	return httpDkimItem{
		ID: d.ID, Domain: d.Domain, Selector: d.Selector,
		Algorithm: d.Algorithm, PublicKeyPEM: d.PublicKeyPEM, KeyPath: d.KeyPath,
		Active: d.Active, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

// --- /v1/feedback ----------------------------------------------------------

type httpFeedbackItem struct {
	ID                int64      `json:"id"`
	ReceivedAt        time.Time  `json:"received_at"`
	FeedbackType      string     `json:"feedback_type"`
	UserAgent         string     `json:"user_agent,omitempty"`
	SourceIP          string     `json:"source_ip,omitempty"`
	OriginalRecipient string     `json:"original_recipient,omitempty"`
	OriginalSender    string     `json:"original_sender,omitempty"`
	OriginalMessageID string     `json:"original_message_id,omitempty"`
	ReportingMTA      string     `json:"reporting_mta,omitempty"`
	ArrivalDate       *time.Time `json:"arrival_date,omitempty"`
}

type httpFeedbackListResp struct {
	Items []httpFeedbackItem `json:"items"`
	Total uint32             `json:"total"`
}

func registerFeedback(hs *kratoshttp.Server, s *service.FeedbackService) {
	hs.HandleFunc("/v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		limit, offset := paginationParams(r, 200, 1000)
		rows, total, err := s.List(r.Context(), limit, offset)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
			return
		}
		items := make([]httpFeedbackItem, 0, len(rows))
		for _, f := range rows {
			items = append(items, httpFeedbackItem{
				ID: f.ID, ReceivedAt: f.ReceivedAt,
				FeedbackType: f.FeedbackType, UserAgent: f.UserAgent,
				SourceIP: f.SourceIP, OriginalRecipient: f.OriginalRecipient,
				OriginalSender: f.OriginalSender, OriginalMessageID: f.OriginalMessageID,
				ReportingMTA: f.ReportingMTA, ArrivalDate: f.ArrivalDate,
			})
		}
		writeJSON(w, http.StatusOK, httpFeedbackListResp{Items: items, Total: total})
	})
}

// --- /v1/logs --------------------------------------------------------------

type httpLogItem struct {
	ID           int64     `json:"id"`
	At           time.Time `json:"at"`
	EventType    string    `json:"event_type"`
	Queue        string    `json:"queue,omitempty"`
	Sender       string    `json:"sender,omitempty"`
	Recipient    string    `json:"recipient,omitempty"`
	MessageID    string    `json:"message_id,omitempty"`
	ResponseCode int32     `json:"response_code"`
	ResponseText string    `json:"response_text,omitempty"`
	SourceIP     string    `json:"source_ip,omitempty"`
	Vmta         string    `json:"vmta,omitempty"`
	MailClass    string    `json:"mail_class,omitempty"`
}

type httpLogListResp struct {
	Items []httpLogItem `json:"items"`
	Total uint32        `json:"total"`
}

func registerLogs(hs *kratoshttp.Server, s *service.LogService) {
	hs.HandleFunc("/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		q := r.URL.Query()
		limit, offset := paginationParams(r, 200, 1000)
		f := service.LogFilter{
			EventType: strings.TrimSpace(q.Get("event_type")),
			Queue:     strings.TrimSpace(q.Get("queue")),
			Sender:    strings.TrimSpace(q.Get("sender")),
			Recipient: strings.TrimSpace(q.Get("recipient")),
			MailClass: strings.TrimSpace(q.Get("mail_class")),
			// message_id ties together every event for one submission
			// (Reception → TransientFailure on retries → final
			// Delivery / Bounce). Click-through from the Logs table
			// prefills this so operators can pull up the full timeline.
			MessageID: strings.TrimSpace(q.Get("message_id")),
		}
		// Time range: both ends optional. RFC3339 is the canonical form;
		// the SPA always sends it that way. Bad values are silently ignored
		// so a misformed paste in the URL bar doesn't 400 the page.
		if v := strings.TrimSpace(q.Get("since")); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.Since = t
			}
		}
		if v := strings.TrimSpace(q.Get("until")); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.Until = t
			}
		}
		rows, total, err := s.List(r.Context(), f, limit, offset)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
			return
		}
		items := make([]httpLogItem, 0, len(rows))
		for _, e := range rows {
			items = append(items, httpLogItem{
				ID: e.ID, At: e.At, EventType: e.EventType,
				Queue: e.Queue, Sender: e.Sender, Recipient: e.Recipient,
				MessageID: e.MessageID, ResponseCode: e.ResponseCode,
				ResponseText: e.ResponseText, SourceIP: e.SourceIP,
				Vmta:      e.Vmta,
				MailClass: e.MailClass,
			})
		}
		writeJSON(w, http.StatusOK, httpLogListResp{Items: items, Total: total})
	})
}

// --- /v1/dsns --------------------------------------------------------------

type httpDsnItem struct {
	ID                int64     `json:"id,string"`
	ReceivedAt        time.Time `json:"received_at"`
	VerpToken         string    `json:"verp_token,omitempty"`
	MessageIDRef      string    `json:"message_id_ref,omitempty"`
	OriginalRecipient string    `json:"original_recipient,omitempty"`
	FinalRecipient    string    `json:"final_recipient,omitempty"`
	Action            string    `json:"action,omitempty"`
	Status            string    `json:"status,omitempty"`
	StatusClass       string    `json:"status_class,omitempty"`
	DiagnosticCode    string    `json:"diagnostic_code,omitempty"`
	RemoteMTA         string    `json:"remote_mta,omitempty"`
	Category          string    `json:"category,omitempty"`
	MailClass         string    `json:"mail_class,omitempty"`
	Tenant            string    `json:"tenant,omitempty"`
	Campaign          string    `json:"campaign,omitempty"`
	RawSize           int32     `json:"raw_size,omitempty"`
	ExtraJSON         string    `json:"extra_json,omitempty"`
}

type httpDsnListResp struct {
	Items []httpDsnItem `json:"items"`
	Total uint32        `json:"total"`
}

func registerDsns(hs *kratoshttp.Server, s *service.DsnService) {
	hs.HandleFunc("/v1/dsns", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		q := r.URL.Query()
		limit, offset := paginationParams(r, 200, 1000)
		f := service.DsnFilter{
			Category:    strings.TrimSpace(q.Get("category")),
			StatusClass: strings.TrimSpace(q.Get("status_class")),
			Status:      strings.TrimSpace(q.Get("status")),
			Recipient:   strings.TrimSpace(q.Get("recipient")),
			MailClass:   strings.TrimSpace(q.Get("mail_class")),
			MessageID:   strings.TrimSpace(q.Get("message_id")),
		}
		// Time-range parsing matches /v1/logs: RFC3339, malformed values
		// silently ignored so a busted URL doesn't 400 the page.
		if v := strings.TrimSpace(q.Get("since")); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.Since = t
			}
		}
		if v := strings.TrimSpace(q.Get("until")); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.Until = t
			}
		}
		rows, total, err := s.List(r.Context(), f, limit, offset)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
			return
		}
		items := make([]httpDsnItem, 0, len(rows))
		for _, e := range rows {
			items = append(items, httpDsnItem{
				ID: e.ID, ReceivedAt: e.ReceivedAt,
				VerpToken: e.VerpToken, MessageIDRef: e.MessageIDRef,
				OriginalRecipient: e.OriginalRecipient,
				FinalRecipient:    e.FinalRecipient,
				Action:            e.Action, Status: e.Status,
				StatusClass:    e.StatusClass,
				DiagnosticCode: e.DiagnosticCode, RemoteMTA: e.RemoteMTA,
				Category:  e.Category,
				MailClass: e.MailClass, Tenant: e.Tenant, Campaign: e.Campaign,
				RawSize:   e.RawSize,
				ExtraJSON: e.ExtraJSON,
			})
		}
		writeJSON(w, http.StatusOK, httpDsnListResp{Items: items, Total: total})
	})
}

// --- /v1/policy ------------------------------------------------------------

type httpPolicyRenderResp struct {
	Lua    string `json:"lua"`
	SHA256 string `json:"sha256"`
}

type httpPolicyValidateResp struct {
	Valid  bool     `json:"valid"`
	Issues []string `json:"issues"`
}

type httpPolicyApplyReq struct {
	Note string `json:"note"`
}

type httpPolicyApplyResp struct {
	SHA256    string    `json:"sha256"`
	AppliedAt time.Time `json:"applied_at"`
}

func registerPolicy(hs *kratoshttp.Server, s *service.PolicyService, write auditmw.WriteFunc) {
	applyAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.PolicyService/Apply",
		resourceType:    "policy",
		mutatingMethods: []string{http.MethodPost},
	})
	hs.HandleFunc("/v1/policy/render", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		actor := strings.TrimSpace(r.URL.Query().Get("by"))
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		lua, sha, err := s.Render(ctx, true, actor)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "RENDER_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, httpPolicyRenderResp{Lua: lua, SHA256: sha})
	})
	// /v1/policy/active returns the init.lua currently on disk — i.e. what
	// kumomta is actually running. Distinct from /render which produces a
	// preview from the current DB snapshot; the two diverge whenever the
	// operator edits config without immediately applying. The Policy
	// editor uses /active so operators see effective state, not draft.
	hs.HandleFunc("/v1/policy/active", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		lua, sha, err := s.Active()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "READ_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, httpPolicyRenderResp{Lua: lua, SHA256: sha})
	})
	hs.HandleFunc("/v1/policy/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		issues, err := s.Validate(ctx)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "VALIDATE_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, httpPolicyValidateResp{
			Valid: len(issues) == 0, Issues: issues,
		})
	})
	hs.HandleFunc("/v1/policy/apply", applyAudit(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		var body httpPolicyApplyReq
		_ = decodeJSON(r, &body)
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		// Identity helper isn't yet wired so actor is anonymous; once auth
		// middleware is mounted, identitymw.IdentityFunc will populate it.
		sha, at, err := s.Apply(ctx, body.Note, 0, "")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "APPLY_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, httpPolicyApplyResp{SHA256: sha, AppliedAt: at})
	}))
}

// --- /v1/mail-classes ------------------------------------------------------

type httpMailClassItem struct {
	ID          uint32    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	TargetKind  string    `json:"target_kind"`
	TargetRef   string    `json:"target_ref"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type httpMailClassListResp struct {
	Items []httpMailClassItem `json:"items"`
	Total uint32              `json:"total"`
}

type httpMailClassReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	TargetKind  string `json:"target_kind"`
	TargetRef   string `json:"target_ref"`
}

func registerMailClasses(hs *kratoshttp.Server, s *service.MailClassService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.MailClassService/Create",
		resourceType:    "mail_class",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.MailClassService/Update",
		resourceType:    "mail_class",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodPut, http.MethodDelete},
	})
	hs.HandleFunc("/v1/mail-classes", collAudit(func(w http.ResponseWriter, r *http.Request) {
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
			items := make([]httpMailClassItem, 0, len(rows))
			for _, m := range rows {
				items = append(items, mailClassToHTTP(&m))
			}
			writeJSON(w, http.StatusOK, httpMailClassListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpMailClassReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			row, err := s.Create(ctx, &service.CreateMailClassInput{
				Name:        body.Name,
				Description: body.Description,
				Enabled:     body.Enabled,
				TargetKind:  body.TargetKind,
				TargetRef:   body.TargetRef,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, mailClassToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))
	hs.HandleFunc("/v1/mail-classes/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		id := uint32(id64)
		switch r.Method {
		case http.MethodPut:
			var body httpMailClassReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			row, err := s.Update(ctx, id, &service.CreateMailClassInput{
				Name:        body.Name,
				Description: body.Description,
				Enabled:     body.Enabled,
				TargetKind:  body.TargetKind,
				TargetRef:   body.TargetRef,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "UPDATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, mailClassToHTTP(row))
		case http.MethodDelete:
			if err := s.Delete(r.Context(), id); err != nil {
				writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use PUT or DELETE")
		}
	}))
}

func mailClassToHTTP(r *service.MailClassRow) httpMailClassItem {
	return httpMailClassItem{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Enabled:     r.Enabled,
		TargetKind:  r.TargetKind,
		TargetRef:   r.TargetRef,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// --- /v1/vmta-groups -------------------------------------------------------

type httpVmtaGroupMember struct {
	VmtaID   uint32 `json:"vmta_id"`
	VmtaName string `json:"vmta_name,omitempty"`
	Weight   uint32 `json:"weight"`
	Priority uint32 `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

type httpVmtaGroupItem struct {
	ID          uint32                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Enabled     bool                  `json:"enabled"`
	Members     []httpVmtaGroupMember `json:"members,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

type httpVmtaGroupListResp struct {
	Items []httpVmtaGroupItem `json:"items"`
	Total uint32              `json:"total"`
}

type httpVmtaGroupReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type httpVmtaGroupSetMembersReq struct {
	Members []httpVmtaGroupMember `json:"members"`
}

func registerVmtaGroups(hs *kratoshttp.Server, s *service.VmtaGroupService, write auditmw.WriteFunc) {
	collAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.VmtaGroupService/Create",
		resourceType:    "vmta_group",
		mutatingMethods: []string{http.MethodPost},
	})
	itemAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.VmtaGroupService/Update",
		resourceType:    "vmta_group",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodPut, http.MethodDelete},
	})
	memberAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.VmtaGroupService/SetMembers",
		resourceType:    "vmta_group",
		resourceVar:     "id",
		mutatingMethods: []string{http.MethodPut},
	})

	hs.HandleFunc("/v1/vmta-groups", collAudit(func(w http.ResponseWriter, r *http.Request) {
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
			items := make([]httpVmtaGroupItem, 0, len(rows))
			for _, g := range rows {
				items = append(items, vmtaGroupToHTTP(&g))
			}
			writeJSON(w, http.StatusOK, httpVmtaGroupListResp{Items: items, Total: total})
		case http.MethodPost:
			var body httpVmtaGroupReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			row, err := s.Create(ctx, &service.CreateVmtaGroupInput{
				Name:        body.Name,
				Description: body.Description,
				Enabled:     body.Enabled,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, vmtaGroupToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))

	hs.HandleFunc("/v1/vmta-groups/{id:[0-9]+}", itemAudit(func(w http.ResponseWriter, r *http.Request) {
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
			writeJSON(w, http.StatusOK, vmtaGroupToHTTP(row))
		case http.MethodPut:
			var body httpVmtaGroupReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			row, err := s.Update(ctx, id, &service.CreateVmtaGroupInput{
				Name:        body.Name,
				Description: body.Description,
				Enabled:     body.Enabled,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "UPDATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, vmtaGroupToHTTP(row))
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

	hs.HandleFunc("/v1/vmta-groups/{id:[0-9]+}/members", memberAudit(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use PUT")
			return
		}
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		id := uint32(id64)
		var body httpVmtaGroupSetMembersReq
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
			return
		}
		members := make([]service.VmtaGroupMemberRow, 0, len(body.Members))
		for _, m := range body.Members {
			members = append(members, service.VmtaGroupMemberRow{
				VmtaID: m.VmtaID, Weight: m.Weight,
				Priority: m.Priority, Enabled: m.Enabled,
			})
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		row, err := s.SetMembers(ctx, id, members)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "SET_MEMBERS_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, vmtaGroupToHTTP(row))
	}))
}

func vmtaGroupToHTTP(r *service.VmtaGroupRow) httpVmtaGroupItem {
	members := make([]httpVmtaGroupMember, 0, len(r.Members))
	for _, m := range r.Members {
		members = append(members, httpVmtaGroupMember{
			VmtaID:   m.VmtaID,
			VmtaName: m.VmtaName,
			Weight:   m.Weight,
			Priority: m.Priority,
			Enabled:  m.Enabled,
		})
	}
	return httpVmtaGroupItem{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Enabled:     r.Enabled,
		Members:     members,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// --- helpers ---------------------------------------------------------------

func splitCSV(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
