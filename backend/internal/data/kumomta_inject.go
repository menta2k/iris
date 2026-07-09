package data

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/menta2k/iris/backend/internal/biz"
)

// InjectV1 forwards a built message to KumoMTA's HTTP injection API
// (POST /api/inject/v1 on the kumod HTTP listener). kumod assembles the MIME,
// then the iris-generated policy's http_message_generated hook stamps
// Message-ID/Date and DKIM-signs before the message is queued.
func (k *FileKumoMTA) InjectV1(ctx context.Context, req biz.KumoInjectRequest) error {
	if strings.TrimSpace(k.cfg.BaseURL) == "" {
		return biz.Unavailable("KUMO_INJECT_UNCONFIGURED", "no KumoMTA base URL configured")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return biz.Internal(err, "marshal kumo inject request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, k.adminURL("/api/inject/v1"), bytes.NewReader(body))
	if err != nil {
		return biz.Internal(err, "build kumo inject request")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := k.client.Do(httpReq)
	if err != nil {
		return biz.Unavailable("KUMO_INJECT_UNREACHABLE", "kumod injection endpoint unreachable: %v", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return biz.Unavailable("KUMO_INJECT_FAILED", "kumod /api/inject/v1 returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

// StubInjector is a no-op KumoInjector for local development (KumoMTA stub
// mode): it accepts every message so the injection endpoint can be exercised
// without a live kumod.
type StubInjector struct{}

var _ biz.KumoInjector = StubInjector{}

// InjectV1 discards the message and reports success.
func (StubInjector) InjectV1(context.Context, biz.KumoInjectRequest) error { return nil }
