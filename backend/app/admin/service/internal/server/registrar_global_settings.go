package server

import (
	"context"
	"net/http"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// httpGlobalSettings is the wire shape for /v1/global-settings. JSON
// field names are snake_case to match the rest of the kumo HTTP surface.
// Lists are emitted explicitly even when empty so the SPA can bind the
// form fields without nullability gymnastics.
type httpGlobalSettings struct {
	KumoHTTPListen      string    `json:"kumo_http_listen"`
	EsmtpRelayHosts     []string  `json:"esmtp_relay_hosts"`
	HTTPTrustedHosts    []string  `json:"http_trusted_hosts"`
	BounceDomain        string    `json:"bounce_domain"`
	BounceSenderDomains []string  `json:"bounce_sender_domains"`
	BouncePrefix        string    `json:"bounce_prefix"`
	MailClassHeader     string    `json:"mail_class_header"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
	UpdatedBy           string    `json:"updated_by,omitempty"`
}

func registerGlobalSettingsHTTP(hs *kratoshttp.Server, s *service.GlobalSettingsService, write auditmw.WriteFunc) {
	updateAudit := httpAudit(write, httpAuditConfig{
		operation:       "/iris.admin/GlobalSettings/Update",
		resourceType:    "global_settings",
		mutatingMethods: []string{http.MethodPut},
	})

	hs.HandleFunc("/v1/global-settings", updateAudit(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			row, err := s.Get(ctx)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "GET_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, rowToHTTP(row))
		case http.MethodPut:
			var body httpGlobalSettings
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			actor := r.Header.Get("X-Iris-Actor") // best-effort; auth middleware will fill this when wired
			row, err := s.Update(ctx, httpToRow(body), actor)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "UPDATE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, rowToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or PUT")
		}
	}))
}

func rowToHTTP(r *service.GlobalSettingsRow) httpGlobalSettings {
	if r == nil {
		return httpGlobalSettings{
			EsmtpRelayHosts:     []string{},
			HTTPTrustedHosts:    []string{},
			BounceSenderDomains: []string{},
		}
	}
	out := httpGlobalSettings{
		KumoHTTPListen:      r.KumoHTTPListen,
		EsmtpRelayHosts:     append([]string{}, r.EsmtpRelayHosts...),
		HTTPTrustedHosts:    append([]string{}, r.HTTPTrustedHosts...),
		BounceDomain:        r.BounceDomain,
		BounceSenderDomains: append([]string{}, r.BounceSenderDomains...),
		BouncePrefix:        r.BouncePrefix,
		MailClassHeader:     r.MailClassHeader,
		UpdatedAt:           r.UpdatedAt,
		UpdatedBy:           r.UpdatedBy,
	}
	return out
}

func httpToRow(h httpGlobalSettings) service.GlobalSettingsRow {
	return service.GlobalSettingsRow{
		KumoHTTPListen:      h.KumoHTTPListen,
		EsmtpRelayHosts:     append([]string(nil), h.EsmtpRelayHosts...),
		HTTPTrustedHosts:    append([]string(nil), h.HTTPTrustedHosts...),
		BounceDomain:        h.BounceDomain,
		BounceSenderDomains: append([]string(nil), h.BounceSenderDomains...),
		BouncePrefix:        h.BouncePrefix,
		MailClassHeader:     h.MailClassHeader,
	}
}
