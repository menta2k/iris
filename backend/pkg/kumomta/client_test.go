package kumomta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewClientRejectsBadScheme(t *testing.T) {
	_, err := NewClient(Config{BaseURL: "ftp://x"})
	require.ErrorIs(t, err, ErrInvalidBaseURL)
}

func TestNewClientRefusesTokenOverPlainHTTP(t *testing.T) {
	_, err := NewClient(Config{BaseURL: "http://example.com", BearerToken: "x"})
	require.ErrorIs(t, err, ErrInsecureToken)
}

func TestNewClientAllowsTokenOverPlainHTTPWithOptIn(t *testing.T) {
	c, err := NewClient(Config{BaseURL: "http://example.com", BearerToken: "x", AllowInsecure: true})
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestListQueues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/admin/ready-q-states/v1":
			_, _ = w.Write([]byte(`{"states_by_ready_queue":{"q1":{"queue_size":3,"delivered":7,"failed":1}}}`))
		case "/api/admin/suspend/v1":
			_, _ = w.Write([]byte(`[{"name":"q1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := NewClient(Config{BaseURL: srv.URL})
	require.NoError(t, err)
	out, err := c.ListQueues(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "q1", out[0].Name)
	require.Equal(t, uint64(3), out[0].QueueSize)
	require.Equal(t, uint64(7), out[0].Delivered)
	require.Equal(t, uint64(1), out[0].Failed)
	require.True(t, out[0].Suspended)
}

func TestSuspendQueueSendsBody(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/admin/suspend-ready-q/v1", r.URL.Path)
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		got = string(buf[:n])
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{BaseURL: srv.URL})
	require.NoError(t, err)
	require.NoError(t, c.SuspendQueue(context.Background(), "marketing"))
	require.Contains(t, got, `"name":"marketing"`)
	require.Contains(t, got, `"reason":"operator-initiated suspension"`)
}

func TestRespectsResponseSizeCap(t *testing.T) {
	big := strings.Repeat("x", MaxResponseBytes+10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL})
	require.NoError(t, err)
	_, err = c.ListQueues(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeded")
}

func TestSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL})
	require.NoError(t, err)
	err = c.Reload(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "status=403")
}
