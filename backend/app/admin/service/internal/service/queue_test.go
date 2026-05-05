package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/menta2k/iris/backend/pkg/kumomta"
)

type fakeMetrics struct {
	mu   sync.Mutex
	rows []QueueSnapshot
}

func (f *fakeMetrics) WriteSnapshots(ctx context.Context, snaps []QueueSnapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, snaps...)
	return nil
}

func newFakeKumo(t *testing.T, body string) (*kumomta.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	c, err := kumomta.NewClient(kumomta.Config{BaseURL: srv.URL})
	require.NoError(t, err)
	return c, srv
}

func TestQueueListPersistsSnapshots(t *testing.T) {
	c, srv := newFakeKumo(t, `[{"name":"q1","queue_size":3,"delivered_total":10,"failed_total":1},{"name":"q2","queue_size":0}]`)
	defer srv.Close()
	m := &fakeMetrics{}
	svc := NewQueueService(c, m)
	out, err := svc.List(context.Background(), "", 0)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, "q1", out[0].Name)
	require.Equal(t, uint64(3), out[0].QueueSize)
	require.Len(t, m.rows, 2)
}

func TestQueueListFilters(t *testing.T) {
	c, srv := newFakeKumo(t, `[{"name":"marketing"},{"name":"transactional"}]`)
	defer srv.Close()
	svc := NewQueueService(c, nil)
	out, err := svc.List(context.Background(), "trans", 0)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "transactional", out[0].Name)
}

func TestQueueListLimits(t *testing.T) {
	c, srv := newFakeKumo(t, `[{"name":"a"},{"name":"b"},{"name":"c"}]`)
	defer srv.Close()
	svc := NewQueueService(c, nil)
	out, err := svc.List(context.Background(), "", 2)
	require.NoError(t, err)
	require.Len(t, out, 2)
}

func TestQueueSuspendRequiresName(t *testing.T) {
	svc := NewQueueService(nil, nil)
	require.ErrorIs(t, svc.Suspend(context.Background(), ""), ErrQueueNameRequired)
	require.ErrorIs(t, svc.Resume(context.Background(), ""), ErrQueueNameRequired)
	require.ErrorIs(t, svc.Bounce(context.Background(), "", ""), ErrQueueNameRequired)
}

func TestQueueActionsHitClient(t *testing.T) {
	calls := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c, err := kumomta.NewClient(kumomta.Config{BaseURL: srv.URL})
	require.NoError(t, err)
	svc := NewQueueService(c, nil)

	require.NoError(t, svc.Suspend(context.Background(), "q1"))
	require.NoError(t, svc.Resume(context.Background(), "q1"))
	require.NoError(t, svc.Bounce(context.Background(), "q1", ""))
	require.Equal(t, []string{
		"/api/admin/queue/suspend/v1",
		"/api/admin/queue/resume/v1",
		"/api/admin/bounce/v1",
	}, calls)
}
