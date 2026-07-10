package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	// Register the "json" codec so sse.WithCodec("json") resolves it (its init()
	// calls encoding.RegisterCodec). Without a codec the SSE server falls back to
	// gob (binary), which the browser's EventSource can't JSON-parse.
	_ "github.com/go-kratos/kratos/v2/encoding/json"

	sse "github.com/tx7do/kratos-transport/transport/sse"

	"github.com/menta2k/iris/backend/internal/biz"
)

// SSE stream ids. Clients subscribe with /sse?stream=<id>&token=<jwt>.
const (
	SSEStreamMailLogs         = "mail-logs"
	SSEStreamBounces          = "bounces"
	SSEStreamDashboard        = "dashboard"
	SSEStreamMonitoringProbes = "monitoring-probes"
)

// SSESessionResolver validates a bearer/session token (the same resolver the
// admin auth middleware uses). EventSource cannot send an Authorization header,
// so the token arrives as the `token` query parameter instead.
type SSESessionResolver interface {
	Resolve(ctx context.Context, token string) (*biz.Identity, error)
}

// NewSSEServer builds the SSE server: it authenticates each subscription with
// the query-param JWT (requiring mail:read, the same permission the mail-log /
// bounce APIs require) and pre-creates the streams so publishes never race
// stream creation. It is served by mounting ServeHTTP on the admin HTTP server
// (same origin as the API) — Start() is never called, so it binds no listener.
func NewSSEServer(ctx context.Context, resolver SSESessionResolver, log *slog.Logger) *sse.Server {
	srv := sse.NewServer(
		sse.WithPath("/sse"),
		sse.WithAutoStream(true),
		// JSON payloads (default is gob/binary, which EventSource can't parse).
		sse.WithCodec("json"),
		sse.WithAuthorizeFunc(func(r *http.Request, token string) error {
			if token == "" {
				return errors.New("missing token") // 401
			}
			id, err := resolver.Resolve(r.Context(), token)
			if err != nil || id == nil {
				return errors.New("invalid token") // 401
			}
			if !id.Permissions.Has(biz.PermMailRead) {
				return sse.ErrForbidden // 403
			}
			return nil
		}),
	)
	streams := []string{SSEStreamMailLogs, SSEStreamBounces, SSEStreamDashboard, SSEStreamMonitoringProbes}
	for _, s := range streams {
		srv.CreateStream(sse.StreamID(s))
	}

	// Heartbeat: send an SSE comment to every stream periodically so a reverse
	// proxy (or the browser) never idles the connection out during quiet periods.
	// Comment-only frames are ignored by EventSource's onmessage and do not break
	// the server's per-subscriber send loop (which only stops on an empty
	// data+comment frame).
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, s := range streams {
					srv.Publish(ctx, sse.StreamID(s), &sse.Event{Comment: []byte("hb")})
				}
			}
		}
	}()

	log.Info("SSE endpoint enabled", "path", "/sse", "streams", streams)
	return srv
}

// ssePublisher adapts an *sse.Server to biz.RealtimePublisher, converting biz
// records to the same camelCase shape the REST API returns so the frontend can
// prepend them without a transform.
type ssePublisher struct {
	srv *sse.Server
	log *slog.Logger
}

// NewSSEPublisher wraps the SSE server as a realtime publisher.
func NewSSEPublisher(srv *sse.Server, log *slog.Logger) biz.RealtimePublisher {
	return &ssePublisher{srv: srv, log: log}
}

// --- wire DTOs (match frontend types in src/types/api.ts) ---

type sseMailRecord struct {
	ID              string `json:"id"`
	MessageID       string `json:"messageId"`
	EventTime       string `json:"eventTime"`
	Mailclass       string `json:"mailclass"`
	Sender          string `json:"sender"`
	FromHeader      string `json:"fromHeader,omitempty"`
	Recipient       string `json:"recipient"`
	RecipientDomain string `json:"recipientDomain"`
	VMTAID          string `json:"vmtaId"`
	EgressSource    string `json:"egressSource,omitempty"`
	Status          string `json:"status"`
	RecordType      string `json:"recordType,omitempty"`
	SMTPStatus      string `json:"smtpStatus,omitempty"`
	Diagnostic      string `json:"diagnostic,omitempty"`
	Classification  string `json:"classification,omitempty"`
}

type sseBounce struct {
	ID              string `json:"id"`
	EventTime       string `json:"eventTime"`
	Recipient       string `json:"recipient"`
	Mailclass       string `json:"mailclass"`
	SMTPStatus      string `json:"smtpStatus"`
	BounceType      string `json:"bounceType"`
	Diagnostic      string `json:"diagnostic"`
	ProcessingState string `json:"processingState"`
	Classification  string `json:"classification,omitempty"`
}

// dashboardTick is the lightweight signal the dashboard debounce-refreshes on.
type dashboardTick struct {
	Kind string `json:"kind"` // "mail" | "bounce"
}

// sseProbe mirrors the frontend MonitoringProbe shape (camelCase). Raw headers /
// message are excluded — the live table doesn't need them.
type sseProbe struct {
	ID            string `json:"id"`
	AccountID     string `json:"accountId"`
	ProbeUID      string `json:"probeUid"`
	MessageID     string `json:"messageId"`
	Subject       string `json:"subject"`
	FromAddr      string `json:"fromAddr"`
	Recipient     string `json:"recipient"`
	SentAt        string `json:"sentAt,omitempty"`
	SendStatus    string `json:"sendStatus"`
	MailboxStatus string `json:"mailboxStatus"`
	Placement     string `json:"placement"`
	FoundAt       string `json:"foundAt,omitempty"`
	LatencyMs     int64  `json:"latencyMs"`
	Analysis      string `json:"analysis"`
	Error         string `json:"error"`
	CreatedAt     string `json:"createdAt,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// PublishMailRecord pushes the record to the mail-logs stream and a tick to the
// dashboard. Best-effort: publish failures are logged at debug and never
// propagate to ingestion.
func (p *ssePublisher) PublishMailRecord(ctx context.Context, rec *biz.MailRecord) {
	if rec == nil {
		return
	}
	dto := sseMailRecord{
		ID: rec.ID, MessageID: rec.MessageID, EventTime: rfc3339(rec.EventTime),
		Mailclass: rec.Mailclass, Sender: rec.Sender, FromHeader: rec.FromHeader,
		Recipient: rec.Recipient, RecipientDomain: rec.RecipientDomain, VMTAID: rec.VMTAID,
		EgressSource: rec.EgressSource, Status: rec.Status, RecordType: rec.RecordType,
		SMTPStatus: rec.SMTPStatus, Diagnostic: rec.Diagnostic, Classification: rec.Classification,
	}
	if err := p.srv.PublishData(ctx, SSEStreamMailLogs, dto); err != nil {
		p.log.Debug("sse publish mail record failed", "error", err.Error())
	}
	_ = p.srv.PublishData(ctx, SSEStreamDashboard, dashboardTick{Kind: "mail"})
}

// PublishBounce pushes the bounce to the bounces stream and a tick to the
// dashboard.
func (p *ssePublisher) PublishBounce(ctx context.Context, b *biz.BounceRecord) {
	if b == nil {
		return
	}
	dto := sseBounce{
		ID: b.ID, EventTime: rfc3339(b.EventTime), Recipient: b.Recipient, Mailclass: b.Mailclass,
		SMTPStatus: b.SMTPStatus, BounceType: b.BounceType, Diagnostic: b.Diagnostic,
		ProcessingState: b.ProcessingState, Classification: b.Classification,
	}
	if err := p.srv.PublishData(ctx, SSEStreamBounces, dto); err != nil {
		p.log.Debug("sse publish bounce failed", "error", err.Error())
	}
	_ = p.srv.PublishData(ctx, SSEStreamDashboard, dashboardTick{Kind: "bounce"})
}

// PublishProbe pushes a created/updated inbox-monitoring probe to the
// monitoring-probes stream. Best-effort.
func (p *ssePublisher) PublishProbe(ctx context.Context, probe *biz.MonitoringProbe) {
	if probe == nil {
		return
	}
	dto := sseProbe{
		ID: probe.ID, AccountID: probe.AccountID, ProbeUID: probe.ProbeUID,
		MessageID: probe.MessageID, Subject: probe.Subject, FromAddr: probe.FromAddr,
		Recipient: probe.Recipient, SentAt: rfc3339(probe.SentAt), SendStatus: probe.SendStatus,
		MailboxStatus: probe.MailboxStatus, Placement: probe.Placement, Analysis: probe.Analysis,
		Error: probe.Error, CreatedAt: rfc3339(probe.CreatedAt), UpdatedAt: rfc3339(probe.UpdatedAt),
	}
	if probe.FoundAt != nil {
		dto.FoundAt = rfc3339(*probe.FoundAt)
	}
	if probe.LatencyMs != nil {
		dto.LatencyMs = *probe.LatencyMs
	}
	if err := p.srv.PublishData(ctx, SSEStreamMonitoringProbes, dto); err != nil {
		p.log.Debug("sse publish probe failed", "error", err.Error())
	}
}
