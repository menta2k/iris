package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/menta2k/iris/backend/pkg/suppressionindex"
)

// recordingIndex captures every call so dual-write tests can assert on
// the exact (op, scope, value) triples the service produced.
type recordingIndex struct {
	mu      sync.Mutex
	addCall []suppressionindex.Entry
	rmCall  []suppressionindex.Entry
	failAdd error
	failRm  error
}

func (r *recordingIndex) Add(_ context.Context, scope, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addCall = append(r.addCall, suppressionindex.Entry{Scope: scope, Value: value})
	return r.failAdd
}
func (r *recordingIndex) Remove(_ context.Context, scope, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rmCall = append(r.rmCall, suppressionindex.Entry{Scope: scope, Value: value})
	return r.failRm
}
func (r *recordingIndex) Resync(_ context.Context, _ []suppressionindex.Entry) error { return nil }
func (r *recordingIndex) Healthy(_ context.Context) error                            { return nil }

type fakeSuppressionStore struct {
	mu   sync.Mutex
	next uint64
	rows []SuppressionRow
}

func (f *fakeSuppressionStore) List(ctx context.Context, limit, offset int) ([]SuppressionRow, uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	end := offset + limit
	if end > len(f.rows) {
		end = len(f.rows)
	}
	if offset > end {
		offset = end
	}
	return append([]SuppressionRow(nil), f.rows[offset:end]...), uint32(len(f.rows)), nil
}
func (f *fakeSuppressionStore) Get(ctx context.Context, id uint64) (*SuppressionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			r := f.rows[i]
			return &r, nil
		}
	}
	return nil, nil
}
func (f *fakeSuppressionStore) Upsert(ctx context.Context, row *SuppressionRow) (*SuppressionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].Address == row.Address && f.rows[i].Scope == row.Scope {
			f.rows[i] = *row
			f.rows[i].ID = uint64(i + 1)
			out := f.rows[i]
			return &out, nil
		}
	}
	atomic.AddUint64(&f.next, 1)
	row.ID = f.next
	f.rows = append(f.rows, *row)
	out := *row
	return &out, nil
}
func (f *fakeSuppressionStore) Delete(ctx context.Context, id uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return nil
}

func TestSuppressionCreateValid(t *testing.T) {
	svc := NewSuppressionService(&fakeSuppressionStore{}, nil)
	row, err := svc.Create(context.Background(), &CreateInput{
		Address: "user@example.com", Scope: "address", Reason: "manual",
	})
	require.NoError(t, err)
	require.Equal(t, "user@example.com", row.Address)
}

func TestSuppressionCreateLowercases(t *testing.T) {
	svc := NewSuppressionService(&fakeSuppressionStore{}, nil)
	row, err := svc.Create(context.Background(), &CreateInput{
		Address: "User@Example.COM", Scope: "address", Reason: "manual",
	})
	require.NoError(t, err)
	require.Equal(t, "user@example.com", row.Address)
}

func TestSuppressionCreateRejectsBadAddress(t *testing.T) {
	svc := NewSuppressionService(&fakeSuppressionStore{}, nil)
	cases := []struct{ a, s string }{
		{"not-an-email", "address"},
		{"a@b\nc", "address"},
		{strings.Repeat("a", 321) + "@x.com", "address"},
		{"bad domain", "domain"},
	}
	for _, c := range cases {
		_, err := svc.Create(context.Background(), &CreateInput{Address: c.a, Scope: c.s, Reason: "manual"})
		require.Error(t, err, "%+v", c)
	}
}

func TestSuppressionCreateRejectsBadReason(t *testing.T) {
	svc := NewSuppressionService(&fakeSuppressionStore{}, nil)
	_, err := svc.Create(context.Background(), &CreateInput{Address: "x@y.com", Scope: "address", Reason: "blocked"})
	require.ErrorIs(t, err, ErrInvalidReason)
}

func TestSuppressionDomainScope(t *testing.T) {
	svc := NewSuppressionService(&fakeSuppressionStore{}, nil)
	row, err := svc.Create(context.Background(), &CreateInput{Address: "spammer.example", Scope: "domain", Reason: "complaint"})
	require.NoError(t, err)
	require.Equal(t, "domain", row.Scope)
}

func TestSuppressionImportCSV(t *testing.T) {
	svc := NewSuppressionService(&fakeSuppressionStore{}, nil)
	csv := `# header comment
a@example.com,address,manual
B@EXAMPLE.com,address,fbl
spam.example,domain,complaint
not-valid
`
	res, err := svc.Import(context.Background(), csv, "manual")
	require.NoError(t, err)
	require.Equal(t, uint32(3), res.Imported)
	require.GreaterOrEqual(t, res.Skipped, uint32(2))
	require.NotEmpty(t, res.Errors)
}

func TestSuppressionImportRefusesBadDefaultReason(t *testing.T) {
	svc := NewSuppressionService(&fakeSuppressionStore{}, nil)
	_, err := svc.Import(context.Background(), "x@y.com", "evil")
	require.ErrorIs(t, err, ErrInvalidReason)
}

func TestSuppressionListPagination(t *testing.T) {
	store := &fakeSuppressionStore{}
	svc := NewSuppressionService(store, nil)
	for i := 0; i < 5; i++ {
		_, err := svc.Create(context.Background(), &CreateInput{
			Address: addressN(i), Scope: "address", Reason: "manual",
		})
		require.NoError(t, err)
	}
	rows, total, err := svc.List(context.Background(), 2, 1)
	require.NoError(t, err)
	require.Equal(t, uint32(5), total)
	require.Len(t, rows, 2)
}

func addressN(i int) string {
	return string(rune('a'+i)) + "@example.com"
}

func TestSuppressionCreatePushesToIndex(t *testing.T) {
	idx := &recordingIndex{}
	svc := NewSuppressionService(&fakeSuppressionStore{}, idx)
	_, err := svc.Create(context.Background(), &CreateInput{
		Address: "User@Example.COM", Scope: "address", Reason: "manual",
	})
	require.NoError(t, err)
	require.Len(t, idx.addCall, 1)
	require.Equal(t, "address", idx.addCall[0].Scope)
	require.Equal(t, "user@example.com", idx.addCall[0].Value)
}

func TestSuppressionDeletePushesToIndex(t *testing.T) {
	idx := &recordingIndex{}
	store := &fakeSuppressionStore{}
	svc := NewSuppressionService(store, idx)
	row, err := svc.Create(context.Background(), &CreateInput{
		Address: "x@example.com", Scope: "address", Reason: "manual",
	})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(context.Background(), row.ID))
	require.Len(t, idx.rmCall, 1)
	require.Equal(t, "x@example.com", idx.rmCall[0].Value)
}

func TestSuppressionCreateSurvivesIndexFailure(t *testing.T) {
	// Index outage MUST NOT block PG inserts: PG is the source of truth,
	// the index is just a hot-path cache. The next resync repairs drift.
	idx := &recordingIndex{failAdd: errors.New("redis unreachable")}
	svc := NewSuppressionService(&fakeSuppressionStore{}, idx)
	row, err := svc.Create(context.Background(), &CreateInput{
		Address: "y@example.com", Scope: "address", Reason: "manual",
	})
	require.NoError(t, err)
	require.NotNil(t, row)
}
