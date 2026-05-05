package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/pkg/suppressionindex"
)

// SuppressionRow is the data-layer view of a suppression entry.
type SuppressionRow struct {
	ID        uint64
	Address   string
	Scope     string // "address" | "domain"
	Reason    string
	Note      string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// SuppressionStore is the data-layer interface for suppressions.
type SuppressionStore interface {
	List(ctx context.Context, limit, offset int) ([]SuppressionRow, uint32, error)
	Get(ctx context.Context, id uint64) (*SuppressionRow, error)
	Upsert(ctx context.Context, row *SuppressionRow) (*SuppressionRow, error)
	Delete(ctx context.Context, id uint64) error
}

// SuppressionService implements the gRPC SuppressionService methods.
//
// Storage is two-tiered: the relational `store` is the source of truth
// (audit trail, list/get, history) and `index` is the hot-path cache
// kumomta consults at message-receive time. PG writes happen first; the
// index is updated best-effort after a successful PG commit. Index
// failures DO NOT roll back PG — the service-level resync hook
// reconciles drift on a schedule.
type SuppressionService struct {
	store SuppressionStore
	index suppressionindex.Index
	now   func() time.Time
}

// NewSuppressionService constructs the service. If `idx` is nil, a no-op
// index is used — callers can adopt the new interface incrementally
// without forcing every test fixture to mock Redis.
func NewSuppressionService(store SuppressionStore, idx suppressionindex.Index) *SuppressionService {
	if idx == nil {
		idx = suppressionindex.NewNoop()
	}
	return &SuppressionService{store: store, index: idx, now: time.Now}
}

// allowedReasons closes the reason set; service refuses other values.
var allowedReasons = map[string]struct{}{
	"manual": {}, "fbl": {}, "hard_bounce": {}, "complaint": {},
}

var (
	ErrInvalidAddress = errors.New("suppression: address invalid")
	ErrInvalidScope   = errors.New("suppression: scope must be address|domain")
	ErrInvalidReason  = errors.New("suppression: invalid reason")
)

var reDomain = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?)+$`)

// CreateInput is the validated input for Create.
type CreateInput struct {
	Address   string
	Scope     string
	Reason    string
	Note      string
	ExpiresAt *time.Time
}

// Create validates inputs and upserts (address, scope) — re-creating an
// existing suppression refreshes the reason/note.
func (s *SuppressionService) Create(ctx context.Context, in *CreateInput) (*SuppressionRow, error) {
	if in == nil {
		return nil, ErrInvalidAddress
	}
	if err := validateSuppression(in.Address, in.Scope, in.Reason); err != nil {
		return nil, err
	}
	row := &SuppressionRow{
		Address:   strings.ToLower(strings.TrimSpace(in.Address)),
		Scope:     in.Scope,
		Reason:    in.Reason,
		Note:      clipString(in.Note, 512),
		CreatedAt: s.now().UTC(),
		ExpiresAt: in.ExpiresAt,
	}
	out, err := s.store.Upsert(ctx, row)
	if err != nil {
		return nil, err
	}
	// Best-effort index push. A failure here means the next periodic
	// resync (or the next admin-service boot) will pick the row up; in
	// the meantime kumomta either fails open (no Redis) or sees the
	// previous state (Redis healthy but write dropped). Either way the
	// outcome is "one extra delivery to a suppressed recipient" — the
	// inverse failure mode (blocking legitimate mail because Redis
	// hiccupped) is strictly worse, so we trade availability for
	// strict consistency here on purpose.
	if err := s.index.Add(ctx, out.Scope, out.Address); err != nil {
		log.Printf("suppression: index add failed (resync will repair) addr=%s scope=%s err=%v", out.Address, out.Scope, err)
	}
	return out, nil
}

// List paginates suppressions.
func (s *SuppressionService) List(ctx context.Context, limit, offset int) ([]SuppressionRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get one entry by id.
func (s *SuppressionService) Get(ctx context.Context, id uint64) (*SuppressionRow, error) {
	if id == 0 {
		return nil, errors.New("suppression: id required")
	}
	return s.store.Get(ctx, id)
}

// Delete one entry.
//
// Order matters: we read the row first so we know what (scope, address)
// to evict from the index, then delete from PG, then remove from the
// index. A crash between PG-delete and index-remove leaves a stale
// suppression row in the index — benign (one extra reject), repaired
// at the next resync.
func (s *SuppressionService) Delete(ctx context.Context, id uint64) error {
	if id == 0 {
		return errors.New("suppression: id required")
	}
	row, getErr := s.store.Get(ctx, id)
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	if getErr == nil && row != nil {
		if err := s.index.Remove(ctx, row.Scope, row.Address); err != nil {
			log.Printf("suppression: index remove failed (resync will repair) addr=%s scope=%s err=%v", row.Address, row.Scope, err)
		}
	}
	return nil
}

// ImportResult is the outcome of an Import call.
type ImportResult struct {
	Imported uint32
	Skipped  uint32
	Errors   []string
}

// Import bulk-loads CSV rows. Format per line:
//   address[,scope[,reason]]
// Empty lines and lines starting with `#` are skipped. Default scope is
// "address"; default reason is the supplied DefaultReason or "manual".
func (s *SuppressionService) Import(ctx context.Context, csv, defaultReason string) (*ImportResult, error) {
	if defaultReason == "" {
		defaultReason = "manual"
	}
	if _, ok := allowedReasons[defaultReason]; !ok {
		return nil, ErrInvalidReason
	}
	res := &ImportResult{}
	scanner := bufio.NewScanner(strings.NewReader(csv))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			res.Skipped++
			continue
		}
		parts := strings.SplitN(line, ",", 3)
		address := strings.TrimSpace(parts[0])
		scope := "address"
		reason := defaultReason
		if len(parts) >= 2 {
			scope = strings.TrimSpace(parts[1])
		}
		if len(parts) >= 3 {
			reason = strings.TrimSpace(parts[2])
		}
		if err := validateSuppression(address, scope, reason); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("line %d: %v", lineNo, err))
			res.Skipped++
			continue
		}
		out, err := s.store.Upsert(ctx, &SuppressionRow{
			Address:   strings.ToLower(address),
			Scope:     scope,
			Reason:    reason,
			CreatedAt: s.now().UTC(),
		})
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("line %d: %v", lineNo, err))
			res.Skipped++
			continue
		}
		// Per-row index pushes during a bulk import would issue one
		// SADD per row — at 1M-row scale that's ~6× slower than a
		// single batched Resync would be. We accept the per-row cost
		// here because Import is a rare path; if a future operator
		// pushes million-row imports through this codepath, swap to
		// a buffered batch SADD before the loop returns.
		if err := s.index.Add(ctx, out.Scope, out.Address); err != nil {
			log.Printf("suppression: import index add failed line=%d addr=%s err=%v", lineNo, out.Address, err)
		}
		res.Imported++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("suppression: scan: %w", err)
	}
	return res, nil
}

func validateSuppression(address, scope, reason string) error {
	address = strings.TrimSpace(address)
	if address == "" || len(address) > 320 {
		return ErrInvalidAddress
	}
	if strings.ContainsAny(address, "\r\n\x00") {
		return ErrInvalidAddress
	}
	switch scope {
	case "address":
		if _, err := mail.ParseAddress(address); err != nil {
			return ErrInvalidAddress
		}
	case "domain":
		if !reDomain.MatchString(address) {
			return ErrInvalidAddress
		}
	default:
		return ErrInvalidScope
	}
	if _, ok := allowedReasons[reason]; !ok {
		return ErrInvalidReason
	}
	return nil
}

func clipString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
