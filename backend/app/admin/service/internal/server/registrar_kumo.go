// HTTP handlers for the kumomta-backed and policy-backed slices:
// /v1/queues, /v1/suppressions, /v1/vmtas, /v1/routing, /v1/dkim,
// /v1/feedback, /v1/logs, /v1/policy. Hand-rolled JSON, same pattern as
// registrar_admin.go — see that file for the rationale and helpers.
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
	auditmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// RegisterKumoHTTP mounts every kumomta/policy-backed route. Each handler
// is wrapped with httpAudit for mutating methods.
func RegisterKumoHTTP(
	hs *kratoshttp.Server,
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
	write auditmw.WriteFunc,
) {
	registerQueuesHTTP(hs, queues, write)
	registerSuppressionsHTTP(hs, suppressions, write)
	registerVmtasHTTP(hs, vmtas, write)
	registerRoutingHTTP(hs, routing, write)
	registerDkimHTTP(hs, dkim, write)
	registerFeedbackHTTP(hs, feedback)
	registerLogsHTTP(hs, logs)
	registerPolicyHTTP(hs, policy, write)
	registerMailClassesHTTP(hs, mailClasses, write)
	registerVmtaGroupsHTTP(hs, vmtaGroups, write)
	RegisterDashboardHTTP(hs, dashboard)
}

// --- /v1/queues -----------------------------------------------------------

type httpQueueItem struct {
	Name      string    `json:"name"`
	QueueSize uint64    `json:"queue_size"`
	Delivered uint64    `json:"delivered"`
	Failed    uint64    `json:"failed"`
	Deferred  uint64    `json:"deferred"`
	Suspended bool      `json:"suspended"`
	SampledAt time.Time `json:"sampled_at"`
}

type httpQueueListResp struct {
	Items []httpQueueItem `json:"items"`
}

func registerQueuesHTTP(hs *kratoshttp.Server, q *service.QueueService, write auditmw.WriteFunc) {
	listAudit := httpAudit(write, httpAuditConfig{
		operation:    "/kumo.service.v1.QueueService/List",
		resourceType: "queue",
		// List is read-only; audit only when caller mutates.
	})
	actionAudit := httpAudit(write, httpAuditConfig{
		operation:       "/kumo.service.v1.QueueService/Action",
		resourceType:    "queue",
		resourceVar:     "name",
		mutatingMethods: []string{http.MethodPost},
	})
	hs.HandleFunc("/v1/queues", listAudit(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		filter := r.URL.Query().Get("filter")
		limit, _ := paginationParams(r, 100, 1000)
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		rows, err := q.List(ctx, filter, limit)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "KUMOMTA_UNREACHABLE", err.Error())
			return
		}
		items := make([]httpQueueItem, 0, len(rows))
		for _, qr := range rows {
			items = append(items, httpQueueItem{
				Name:      qr.Name,
				QueueSize: qr.QueueSize,
				Delivered: qr.Delivered,
				Failed:    qr.Failed,
				Deferred:  qr.Deferred,
				Suspended: qr.Suspended,
				SampledAt: qr.SampledAt,
			})
		}
		writeJSON(w, http.StatusOK, httpQueueListResp{Items: items})
	}))
	hs.HandleFunc("/v1/queues/{name}/messages", listAudit(queueInspectHandler(q)))
	hs.HandleFunc("/v1/queues/{name}/{action:suspend|resume|bounce}",
		actionAudit(queueActionHandler(q)))
}

type httpScheduledMessage struct {
	ID          string         `json:"id"`
	Sender      string         `json:"sender,omitempty"`
	Recipient   string         `json:"recipient,omitempty"`
	DueAt       time.Time      `json:"due_at,omitempty"`
	NumAttempts uint32         `json:"num_attempts"`
	Tenant      string         `json:"tenant,omitempty"`
	Campaign    string         `json:"campaign,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

type httpScheduledMessagesResp struct {
	QueueName string                 `json:"queue_name"`
	Items     []httpScheduledMessage `json:"items"`
}

// queueInspectHandler returns a sample of messages held in the named
// scheduled queue. Backed by kumomta's /api/admin/inspect-sched-q/v1.
func queueInspectHandler(q *service.QueueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		name := mux.Vars(r)["name"]
		if name == "" {
			writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "queue name required")
			return
		}
		// Cap at 500 to match the service-side ceiling — anything larger
		// would risk exhausting the upstream's inspection budget.
		limit, _ := paginationParams(r, 50, 500)
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		msgs, err := q.InspectScheduledQueue(ctx, name, limit)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "KUMOMTA_UNREACHABLE", err.Error())
			return
		}
		items := make([]httpScheduledMessage, 0, len(msgs))
		for _, m := range msgs {
			items = append(items, httpScheduledMessage{
				ID: m.ID, Sender: m.Sender, Recipient: m.Recipient,
				DueAt: m.DueAt, NumAttempts: m.NumAttempts,
				Tenant: m.Tenant, Campaign: m.Campaign, Meta: m.Meta,
			})
		}
		writeJSON(w, http.StatusOK, httpScheduledMessagesResp{
			QueueName: name, Items: items,
		})
	}
}

type httpBounceReq struct {
	Reason string `json:"reason"`
}

func queueActionHandler(q *service.QueueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		vars := mux.Vars(r)
		name, action := vars["name"], vars["action"]
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		var err error
		switch action {
		case "suspend":
			err = q.Suspend(ctx, name)
		case "resume":
			err = q.Resume(ctx, name)
		case "bounce":
			var body httpBounceReq
			_ = decodeJSON(r, &body) // body is optional
			err = q.Bounce(ctx, name, body.Reason)
		default:
			writeErr(w, http.StatusBadRequest, "BAD_ACTION", "action must be suspend|resume|bounce")
			return
		}
		if err != nil {
			writeErr(w, http.StatusBadGateway, "KUMOMTA_UNREACHABLE", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- placeholders for the remaining domains. Each is implemented in its
// own _slice.go file to keep this dispatcher small. The functions below are
// the register-points referenced by RegisterKumoHTTP above.

func registerSuppressionsHTTP(hs *kratoshttp.Server, s *service.SuppressionService, write auditmw.WriteFunc) {
	registerSuppressions(hs, s, write)
}
func registerVmtasHTTP(hs *kratoshttp.Server, s *service.VirtualMtaService, write auditmw.WriteFunc) {
	registerVmtas(hs, s, write)
}
func registerRoutingHTTP(hs *kratoshttp.Server, s *service.RoutingService, write auditmw.WriteFunc) {
	registerRouting(hs, s, write)
}
func registerDkimHTTP(hs *kratoshttp.Server, s *service.DkimService, write auditmw.WriteFunc) {
	registerDkim(hs, s, write)
}
func registerFeedbackHTTP(hs *kratoshttp.Server, s *service.FeedbackService) {
	registerFeedback(hs, s)
}
func registerLogsHTTP(hs *kratoshttp.Server, s *service.LogService) {
	registerLogs(hs, s)
}
func registerPolicyHTTP(hs *kratoshttp.Server, s *service.PolicyService, write auditmw.WriteFunc) {
	registerPolicy(hs, s, write)
}
func registerMailClassesHTTP(hs *kratoshttp.Server, s *service.MailClassService, write auditmw.WriteFunc) {
	registerMailClasses(hs, s, write)
}
func registerVmtaGroupsHTTP(hs *kratoshttp.Server, s *service.VmtaGroupService, write auditmw.WriteFunc) {
	registerVmtaGroups(hs, s, write)
}

// shared helper used by several handlers.
func parseUintParam(r *http.Request, name string) (uint32, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}
