package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
)

// fakeInjectUC records the request and returns a preset error.
type fakeInjectUC struct {
	err  error
	req  *biz.GAInjectRequest
	seen bool
}

func (f *fakeInjectUC) Inject(_ context.Context, req *biz.GAInjectRequest) error {
	f.seen = true
	f.req = req
	return f.err
}

// The exact JSON an existing GreenArrow client sends (mirrors the PHP payload).
const greenArrowBody = `{
  "username": "apiuser",
  "password": "s3cret",
  "message": {
    "html": "<p>hello</p>",
    "text": "hello",
    "subject": "Your order",
    "to": [{"email": "buyer@gmail.com", "name": "Buyer"}],
    "from_email": "news@acme.example.com",
    "from_name": "Example",
    "mailclass": "acme_k",
    "headers": [{"X-Feedback-ID": "buyer@gmail.com:1:1:acme"}]
  }
}`

func doInject(t *testing.T, uc MailInjector, method, body string) (*http.Response, biz.GAResponse) {
	t.Helper()
	h := injectHandler(uc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(method, "/api/inject", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	res := rec.Result()
	var out biz.GAResponse
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res, out
}

func TestInjectHandlerSuccess(t *testing.T) {
	uc := &fakeInjectUC{}
	res, out := doInject(t, uc, http.MethodPost, greenArrowBody)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if out.Success != 1 || out.Error != "" {
		t.Fatalf("response = %+v, want {success:1}", out)
	}
	if !uc.seen || uc.req.Username != "apiuser" || uc.req.Message.Mailclass != "acme_k" {
		t.Fatalf("usecase did not receive the decoded request: %+v", uc.req)
	}
}

func TestInjectHandlerUnauthorized(t *testing.T) {
	uc := &fakeInjectUC{err: biz.Unauthorized("INJECT_UNAUTHORIZED", "invalid API credentials")}
	res, out := doInject(t, uc, http.MethodPost, greenArrowBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
	if out.Success != 0 || out.Error == "" {
		t.Fatalf("response = %+v, want {success:0, error}", out)
	}
}

func TestInjectHandlerValidationError(t *testing.T) {
	uc := &fakeInjectUC{err: biz.Invalid("INJECT_FROM_REQUIRED", "from_email is required")}
	res, out := doInject(t, uc, http.MethodPost, greenArrowBody)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	if out.Success != 0 || !strings.Contains(out.Error, "from_email") {
		t.Fatalf("response = %+v", out)
	}
}

func TestInjectHandlerUnavailable(t *testing.T) {
	uc := &fakeInjectUC{err: biz.Unavailable("KUMO_INJECT_UNREACHABLE", "kumod down")}
	res, out := doInject(t, uc, http.MethodPost, greenArrowBody)
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", res.StatusCode)
	}
	if out.Success != 0 {
		t.Fatalf("response = %+v", out)
	}
}

func TestInjectHandlerMalformedJSON(t *testing.T) {
	uc := &fakeInjectUC{}
	res, out := doInject(t, uc, http.MethodPost, "{not json")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	if out.Success != 0 || uc.seen {
		t.Fatalf("malformed body must not reach the usecase: %+v", out)
	}
}

func TestInjectHandlerRejectsGet(t *testing.T) {
	uc := &fakeInjectUC{}
	res, out := doInject(t, uc, http.MethodGet, "")
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.StatusCode)
	}
	if out.Success != 0 || uc.seen {
		t.Fatalf("GET must be rejected: %+v", out)
	}
}
