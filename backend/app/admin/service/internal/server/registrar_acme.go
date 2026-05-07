package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// HTTP shapes for /v1/acme/* endpoints.

type httpAcmeAccount struct {
	Email           string    `json:"email"`
	ServerURL       string    `json:"server_url"`
	HasRegistration bool      `json:"has_registration"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type httpAcmeCertificate struct {
	ID            uint32     `json:"id"`
	Domain        string     `json:"domain"`
	AltNames      []string   `json:"alt_names"`
	ChallengeType string     `json:"challenge_type"`
	DnsProvider   string     `json:"dns_provider,omitempty"`
	CertPemPath   string     `json:"cert_pem_path,omitempty"`
	KeyPemPath    string     `json:"key_pem_path,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	LastRenewedAt *time.Time `json:"last_renewed_at,omitempty"`
	Status        string     `json:"status"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at,omitempty"`
}

type httpAcmeCertList struct {
	Items []httpAcmeCertificate `json:"items"`
}

type httpAcmeIssueReq struct {
	Domain        string   `json:"domain"`
	AltNames      []string `json:"alt_names"`
	ChallengeType string   `json:"challenge_type"`
	DnsProvider   string   `json:"dns_provider"`
}

type httpAcmeDnsCfg struct {
	Provider  string            `json:"provider"`
	Config    map[string]string `json:"config"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
	UpdatedBy string            `json:"updated_by,omitempty"`
}

type httpAcmeDnsCfgList struct {
	Items []httpAcmeDnsCfg `json:"items"`
}

type httpAcmeProviderInfo struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	RequiredFields []string `json:"required_fields"`
	OptionalFields []string `json:"optional_fields"`
}

type httpAcmeRegistry struct {
	Items []httpAcmeProviderInfo `json:"items"`
}

func registerAcmeHTTP(hs *kratoshttp.Server, s *service.AcmeService, write auditmw.WriteFunc) {
	mut := httpAudit(write, httpAuditConfig{
		operation:       "/iris.admin/AcmeService/Update",
		resourceType:    "acme",
		mutatingMethods: []string{http.MethodPost, http.MethodPut, http.MethodDelete},
	})

	// /v1/acme/account — singleton
	hs.HandleFunc("/v1/acme/account", mut(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			row, err := s.GetAccount(r.Context())
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "GET_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, accountToHTTP(row))
		case http.MethodPut:
			var body struct {
				Email     string `json:"email"`
				ServerURL string `json:"server_url"`
			}
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			row, err := s.SaveAccount(r.Context(), service.AcmeAccountRow{
				Email: body.Email, ServerURL: body.ServerURL,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "SAVE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, accountToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or PUT")
		}
	}))

	// /v1/acme/dns-providers/registry — registry metadata (read-only)
	hs.HandleFunc("/v1/acme/dns-providers/registry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		all := s.ListProviderRegistry()
		out := httpAcmeRegistry{Items: make([]httpAcmeProviderInfo, 0, len(all))}
		for _, info := range all {
			out.Items = append(out.Items, httpAcmeProviderInfo{
				Name: info.Name, Description: info.Description,
				RequiredFields: info.RequiredFields, OptionalFields: info.OptionalFields,
			})
		}
		writeJSON(w, http.StatusOK, out)
	})

	// /v1/acme/dns-providers — list saved configs / create new
	hs.HandleFunc("/v1/acme/dns-providers", mut(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rows, err := s.ListDnsProviderConfigs(r.Context())
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			out := httpAcmeDnsCfgList{Items: make([]httpAcmeDnsCfg, 0, len(rows))}
			for _, c := range rows {
				out.Items = append(out.Items, dnsCfgToHTTP(c))
			}
			writeJSON(w, http.StatusOK, out)
		case http.MethodPut:
			var body httpAcmeDnsCfg
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			actor := r.Header.Get("X-Iris-Actor")
			row, err := s.UpsertDnsProviderConfig(r.Context(), service.AcmeDnsProviderConfigRow{
				Provider: body.Provider, Config: body.Config,
			}, actor)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "UPSERT_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, dnsCfgToHTTP(*row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or PUT")
		}
	}))

	// /v1/acme/dns-providers/{name} — delete
	hs.HandleFunc("/v1/acme/dns-providers/{name}", mut(func(w http.ResponseWriter, r *http.Request) {
		name := mux.Vars(r)["name"]
		switch r.Method {
		case http.MethodDelete:
			if err := s.DeleteDnsProviderConfig(r.Context(), name); err != nil {
				writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE")
		}
	}))

	// /v1/acme/certificates — list / issue
	hs.HandleFunc("/v1/acme/certificates", mut(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rows, err := s.ListCertificates(r.Context())
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
				return
			}
			out := httpAcmeCertList{Items: make([]httpAcmeCertificate, 0, len(rows))}
			for _, c := range rows {
				out.Items = append(out.Items, certToHTTP(&c))
			}
			writeJSON(w, http.StatusOK, out)
		case http.MethodPost:
			var body httpAcmeIssueReq
			if err := decodeJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
				return
			}
			ctx, cancel := contextWithIssueTimeout(r.Context())
			defer cancel()
			row, err := s.IssueCertificate(ctx, service.AcmeIssueRequest{
				Domain: body.Domain, AltNames: body.AltNames,
				ChallengeType: body.ChallengeType, DnsProvider: body.DnsProvider,
			})
			if err != nil {
				writeErr(w, http.StatusBadRequest, "ISSUE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, certToHTTP(row))
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
		}
	}))

	// /v1/acme/certificates/{id} — get / delete
	hs.HandleFunc("/v1/acme/certificates/{id:[0-9]+}", mut(func(w http.ResponseWriter, r *http.Request) {
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		id := uint32(id64)
		switch r.Method {
		case http.MethodGet:
			row, err := s.GetCertificate(r.Context(), id)
			if err != nil {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, certToHTTP(row))
		case http.MethodDelete:
			if err := s.DeleteCertificate(r.Context(), id); err != nil {
				writeErr(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or DELETE")
		}
	}))

	// /v1/acme/certificates/{id}/renew
	hs.HandleFunc("/v1/acme/certificates/{id:[0-9]+}/renew", mut(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		id64, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
		ctx, cancel := contextWithIssueTimeout(r.Context())
		defer cancel()
		row, err := s.RenewCertificate(ctx, uint32(id64))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "RENEW_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, certToHTTP(row))
	}))
}

// ACME issuance can take 30-90s with DNS-01 propagation; default 120s.
func contextWithIssueTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 120*time.Second)
}

// --- adapters ---------------------------------------------------------------

func accountToHTTP(r *service.AcmeAccountRow) httpAcmeAccount {
	return httpAcmeAccount{
		Email: r.Email, ServerURL: r.ServerURL,
		HasRegistration: r.HasRegistration, UpdatedAt: r.UpdatedAt,
	}
}

func certToHTTP(r *service.AcmeCertificateRow) httpAcmeCertificate {
	return httpAcmeCertificate{
		ID: r.ID, Domain: r.Domain, AltNames: r.AltNames,
		ChallengeType: r.ChallengeType, DnsProvider: r.DnsProvider,
		CertPemPath: r.CertPemPath, KeyPemPath: r.KeyPemPath,
		ExpiresAt: r.ExpiresAt, LastRenewedAt: r.LastRenewedAt,
		Status: r.Status, LastError: r.LastError,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func dnsCfgToHTTP(r service.AcmeDnsProviderConfigRow) httpAcmeDnsCfg {
	return httpAcmeDnsCfg{
		Provider: r.Provider, Config: r.Config,
		UpdatedAt: r.UpdatedAt, UpdatedBy: r.UpdatedBy,
	}
}
