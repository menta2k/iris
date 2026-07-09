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
	SSEStreamMailLogs  = "mail-logs"
	SSEStreamBounces   = "bounces"
	SSEStreamDashboard = "dashboard"
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
func NewSSEServer(resolver SSESessionResolver, log *slog.Logger) *sse.Server {
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
	srv.CreateStream(SSEStreamMailLogs)
	srv.CreateStream(SSEStreamBounces)
	srv.CreateStream(SSEStreamDashboard)
	log.Info("SSE endpoint enabled", "path", "/sse",
		"streams", []string{SSEStreamMailLogs, SSEStreamBounces, SSEStreamDashboard})
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
