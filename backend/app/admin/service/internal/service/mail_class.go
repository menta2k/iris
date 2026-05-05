package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MailClassRow is the data-layer view of a mail class. The renderer reads
// the global header (X-Kumo-Mail-Class by default) at message reception and
// uses the value to look up a class by name, then routes to TargetKind+Ref.
type MailClassRow struct {
	ID          uint32    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	TargetKind  string    `json:"target_kind"` // "vmta" | "vmta_group"
	TargetRef   string    `json:"target_ref"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MailClassStore is the data-layer interface implemented by the ent repo.
type MailClassStore interface {
	List(ctx context.Context, limit, offset int) ([]MailClassRow, uint32, error)
	Get(ctx context.Context, id uint32) (*MailClassRow, error)
	Create(ctx context.Context, in MailClassRow) (*MailClassRow, error)
	Update(ctx context.Context, id uint32, in MailClassRow) (*MailClassRow, error)
	Delete(ctx context.Context, id uint32) error
}

// MailClassService implements CRUD with input validation.
type MailClassService struct {
	store MailClassStore
	now   func() time.Time
}

// NewMailClassService constructs the service.
func NewMailClassService(store MailClassStore) *MailClassService {
	return &MailClassService{store: store, now: time.Now}
}

var (
	// reMailClassName matches names that are safe to embed in Lua identifiers
	// and to use as a routing target ref. Same shape kumopolicy enforces.
	reMailClassName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

	// reMailClassRef mirrors reMailClassName: the target_ref points at a
	// VMTA or VMTA group by name, both of which use the same character set.
	reMailClassRef = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

	// allowedMailClassTargetKinds closes the set of legal targets.
	allowedMailClassTargetKinds = map[string]struct{}{
		"vmta":       {},
		"vmta_group": {},
	}

	ErrMailClassName       = errors.New("mail_class: name must match [A-Za-z0-9_.-]{1,64}")
	ErrMailClassTargetKind = errors.New("mail_class: target_kind must be vmta or vmta_group")
	ErrMailClassTargetRef  = errors.New("mail_class: target_ref must match [A-Za-z0-9_.-]{1,64}")
	ErrMailClassID         = errors.New("mail_class: id required")
)

// CreateMailClassInput is the validated input for Create / Update.
type CreateMailClassInput struct {
	Name        string
	Description string
	Enabled     bool
	TargetKind  string
	TargetRef   string
}

// List paginates classes.
func (s *MailClassService) List(ctx context.Context, limit, offset int) ([]MailClassRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get one class by id.
func (s *MailClassService) Get(ctx context.Context, id uint32) (*MailClassRow, error) {
	if id == 0 {
		return nil, ErrMailClassID
	}
	return s.store.Get(ctx, id)
}

// Create persists a new class. Names must be unique (enforced by ent index).
func (s *MailClassService) Create(ctx context.Context, in *CreateMailClassInput) (*MailClassRow, error) {
	if in == nil {
		return nil, ErrMailClassName
	}
	row, err := buildMailClassRow(in, s.now)
	if err != nil {
		return nil, err
	}
	out, err := s.store.Create(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("mail_class: create: %w", err)
	}
	return out, nil
}

// Update overwrites an existing class.
func (s *MailClassService) Update(ctx context.Context, id uint32, in *CreateMailClassInput) (*MailClassRow, error) {
	if id == 0 {
		return nil, ErrMailClassID
	}
	if in == nil {
		return nil, ErrMailClassName
	}
	row, err := buildMailClassRow(in, s.now)
	if err != nil {
		return nil, err
	}
	out, err := s.store.Update(ctx, id, row)
	if err != nil {
		return nil, fmt.Errorf("mail_class: update: %w", err)
	}
	return out, nil
}

// Delete removes a class.
func (s *MailClassService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return ErrMailClassID
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return fmt.Errorf("mail_class: delete: %w", err)
	}
	return nil
}

func buildMailClassRow(in *CreateMailClassInput, now func() time.Time) (MailClassRow, error) {
	name := strings.TrimSpace(in.Name)
	if !reMailClassName.MatchString(name) {
		return MailClassRow{}, ErrMailClassName
	}
	if _, ok := allowedMailClassTargetKinds[in.TargetKind]; !ok {
		return MailClassRow{}, ErrMailClassTargetKind
	}
	ref := strings.TrimSpace(in.TargetRef)
	if !reMailClassRef.MatchString(ref) {
		return MailClassRow{}, ErrMailClassTargetRef
	}
	return MailClassRow{
		Name:        name,
		Description: clipString(in.Description, 512),
		Enabled:     in.Enabled,
		TargetKind:  in.TargetKind,
		TargetRef:   ref,
		CreatedAt:   now().UTC(),
		UpdatedAt:   now().UTC(),
	}, nil
}
