package service

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"
)

// ListenerRow is the data-layer view of one kumomta SMTP listener. It
// mirrors the ListenerConfig ent schema; the renderer (pkg/kumopolicy)
// reads this shape via SnapshotProvider and emits one
// kumo.start_esmtp_listener block per row.
type ListenerRow struct {
	ID             uint32
	Name           string
	ListenAddr     string // "host:port" — operator-supplied verbatim, kumomta parses
	Hostname       string
	TLSEnabled     bool
	TLSCertPath    string
	TLSKeyPath     string
	RequireAuth    bool
	MaxMessageSize uint64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ListenerStore is the data-layer interface.
type ListenerStore interface {
	List(ctx context.Context, limit, offset int) ([]ListenerRow, uint32, error)
	Get(ctx context.Context, id uint32) (*ListenerRow, error)
	Create(ctx context.Context, in ListenerRow) (*ListenerRow, error)
	Update(ctx context.Context, id uint32, in ListenerRow) (*ListenerRow, error)
	Delete(ctx context.Context, id uint32) error
}

// ListenerService applies validation around the store. The renderer
// emits one start_esmtp_listener per row, so a malformed ListenAddr
// would crash kumomta at load time — we catch the obvious mistakes
// here.
type ListenerService struct{ store ListenerStore }

// NewListenerService constructs the service.
func NewListenerService(s ListenerStore) *ListenerService { return &ListenerService{store: s} }

var (
	ErrListenerNameInvalid = errors.New("listener: name must be 1-64 chars [a-zA-Z0-9_.-]")
	ErrListenerAddrInvalid = errors.New("listener: listen_addr must be host:port (e.g. 0.0.0.0:25)")
	ErrListenerHostInvalid = errors.New("listener: hostname is not a valid DNS label")
	ErrListenerTLSPaths    = errors.New("listener: tls_cert_pem_path and tls_key_pem_path are both required when tls_enabled")

	reListenerName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	reListenerHost = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
)

func (s *ListenerService) validate(in *ListenerRow) error {
	if !reListenerName.MatchString(in.Name) {
		return ErrListenerNameInvalid
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(in.ListenAddr))
	if err != nil || port == "" {
		return ErrListenerAddrInvalid
	}
	// Empty host is fine ("0.0.0.0" / "::" implicit); bare numeric port is
	// allowed by SplitHostPort when given as ":25". Reject anything that
	// has whitespace or a path-looking segment in the host.
	if strings.ContainsAny(host, " \t/?#") {
		return ErrListenerAddrInvalid
	}
	if in.Hostname != "" && !reListenerHost.MatchString(in.Hostname) {
		return ErrListenerHostInvalid
	}
	if in.TLSEnabled {
		// kumomta needs both files to exist when TLS is on; we only check
		// "path is non-empty" here. The renderer hands the paths verbatim
		// to make_listener_domain — kumomta's startup will surface the
		// real "file missing" error if the paths are wrong.
		if strings.TrimSpace(in.TLSCertPath) == "" || strings.TrimSpace(in.TLSKeyPath) == "" {
			return ErrListenerTLSPaths
		}
	}
	return nil
}

// List paginates listeners.
func (s *ListenerService) List(ctx context.Context, limit, offset int) ([]ListenerRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get one.
func (s *ListenerService) Get(ctx context.Context, id uint32) (*ListenerRow, error) {
	if id == 0 {
		return nil, errors.New("listener: id required")
	}
	return s.store.Get(ctx, id)
}

// Create validates and inserts. Returns ErrListener* on validation fail.
func (s *ListenerService) Create(ctx context.Context, in *ListenerRow) (*ListenerRow, error) {
	if in == nil {
		return nil, ErrListenerNameInvalid
	}
	if err := s.validate(in); err != nil {
		return nil, err
	}
	return s.store.Create(ctx, *in)
}

// Update validates and saves. Name is immutable after create — operators
// rename listeners by deleting + recreating, same as VMTAs.
func (s *ListenerService) Update(ctx context.Context, id uint32, in *ListenerRow) (*ListenerRow, error) {
	if id == 0 || in == nil {
		return nil, errors.New("listener: id required")
	}
	if err := s.validate(in); err != nil {
		return nil, err
	}
	return s.store.Update(ctx, id, *in)
}

// Delete removes a listener. Cascades to listener_domains via the ent edge.
func (s *ListenerService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return errors.New("listener: id required")
	}
	return s.store.Delete(ctx, id)
}
