package service

import (
	"context"
	"encoding/json"
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

// newFakeKumo accepts a list of queue specs and synthesises the two
// upstream-kumomta admin responses that QueueService.List needs:
//
//   GET /api/admin/ready-q-states/v1  → {"states_by_ready_queue": {...}}
//   GET /api/admin/suspend/v1         → []
//
// The 2025+ kumomta admin API returns a map keyed by queue name with
// per-queue counter sub-objects; the older flat-array shape this
// test used to feed is no longer accepted by the client. Tests
// hand us the bare per-queue specs and we wrap them in the right
// envelope.
type fakeQueue struct {
	Name      string `json:"-"`
	QueueSize uint64 `json:"queue_size"`
	Delivered uint64 `json:"delivered"`
	Failed    uint64 `json:"failed"`
	Deferred  uint64 `json:"deferred"`
}

func newFakeKumo(t *testing.T, queues ...fakeQueue) (*kumomta.Client, *httptest.Server) {
	t.Helper()

	// Build the ready-q-states JSON. Using encoding/json keeps the
	// quoting honest if a future test passes a queue name with
	// special characters.
	type body struct {
		StatesByReadyQueue map[string]fakeQueue `json:"states_by_ready_queue"`
	}
	b := body{StatesByReadyQueue: make(map[string]fakeQueue, len(queues))}
	for _, q := range queues {
		b.StatesByReadyQueue[q.Name] = q
	}
	statesJSON, err := json.Marshal(b)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/ready-q-states/v1":
			_, _ = w.Write(statesJSON)
		case "/api/admin/suspend/v1":
			// No suspensions in the test fakes — empty array is the
			// right "nothing suspended" signal.
			_, _ = w.Write([]byte(`[]`))
		default:
			// Anything else (Suspend / Resume / Bounce) just acks
			// with no content; tests that care assert on calls.
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	c, err := kumomta.NewClient(kumomta.Config{BaseURL: srv.URL})
	require.NoError(t, err)
	return c, srv
}

func TestQueueListPersistsSnapshots(t *testing.T) {
	c, srv := newFakeKumo(t,
		fakeQueue{Name: "q1", QueueSize: 3, Delivered: 10, Failed: 1},
		fakeQueue{Name: "q2"},
	)
	defer srv.Close()
	m := &fakeMetrics{}
	svc := NewQueueService(c, m)
	out, err := svc.List(context.Background(), "", 0)
	require.NoError(t, err)
	require.Len(t, out, 2)
	// ListQueues iterates a map under the hood, so the result order
	// isn't stable. Assert by name instead of by index.
	byName := map[string]QueueListItem{}
	for _, q := range out {
		byName[q.Name] = q
	}
	require.Equal(t, uint64(3), byName["q1"].QueueSize)
	require.Len(t, m.rows, 2)
}

func TestQueueListFilters(t *testing.T) {
	c, srv := newFakeKumo(t,
		fakeQueue{Name: "marketing"},
		fakeQueue{Name: "transactional"},
	)
	defer srv.Close()
	svc := NewQueueService(c, nil)
	out, err := svc.List(context.Background(), "trans", 0)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "transactional", out[0].Name)
}

func TestQueueListLimits(t *testing.T) {
	c, srv := newFakeKumo(t,
		fakeQueue{Name: "a"},
		fakeQueue{Name: "b"},
		fakeQueue{Name: "c"},
	)
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
	// Record method+path pairs. Suspend and Resume both hit
	// `/api/admin/suspend-ready-q/v1` — one POST, one DELETE — so
	// recording the path alone would lose the distinction. The
	// upstream kumomta API consolidated the two endpoints in 2025;
	// this test pins the expectation to the new shape.
	type call struct{ method, path string }
	calls := []call{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, call{method: r.Method, path: r.URL.Path})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c, err := kumomta.NewClient(kumomta.Config{BaseURL: srv.URL})
	require.NoError(t, err)
	svc := NewQueueService(c, nil)

	require.NoError(t, svc.Suspend(context.Background(), "q1"))
	require.NoError(t, svc.Resume(context.Background(), "q1"))
	require.NoError(t, svc.Bounce(context.Background(), "q1", ""))
	require.Equal(t, []call{
		{method: http.MethodPost, path: "/api/admin/suspend-ready-q/v1"},
		{method: http.MethodDelete, path: "/api/admin/suspend-ready-q/v1"},
		{method: http.MethodPost, path: "/api/admin/bounce/v1"},
	}, calls)
}
