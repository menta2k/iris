package service

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MailWebhookRow is the data-layer view of one inbound-mail → HTTP webhook.
// The renderer (pkg/kumopolicy) reads this shape via SnapshotProvider and
// emits a lookup table + a custom_lua queue that POSTs matching mail to URL.
type MailWebhookRow struct {
	ID        uint32
	Name      string
	Address   string // exact recipient "support@host" OR a bare domain "host"
	URL       string
	Secret    string // optional HMAC key
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MailWebhookStore is the data-layer interface.
type MailWebhookStore interface {
	List(ctx context.Context, limit, offset int) ([]MailWebhookRow, uint32, error)
	Get(ctx context.Context, id uint32) (*MailWebhookRow, error)
	Create(ctx context.Context, in MailWebhookRow) (*MailWebhookRow, error)
	Update(ctx context.Context, id uint32, in MailWebhookRow) (*MailWebhookRow, error)
	Delete(ctx context.Context, id uint32) error
}

// MailWebhookService applies validation around the store.
type MailWebhookService struct{ store MailWebhookStore }

func NewMailWebhookService(s MailWebhookStore) *MailWebhookService {
	return &MailWebhookService{store: s}
}

var (
	ErrWebhookNameInvalid    = errors.New("mail_webhook: name must be 1-64 chars [a-zA-Z0-9_.-]")
	ErrWebhookAddressInvalid = errors.New("mail_webhook: address must be an email (user@host) or a domain")
	ErrWebhookURLInvalid     = errors.New("mail_webhook: url must be an http(s):// URL")

	reWebhookName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	reWebhookHost = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
)

func (s *MailWebhookService) validate(in *MailWebhookRow) error {
	if !reWebhookName.MatchString(in.Name) {
		return ErrWebhookNameInvalid
	}
	if !validWebhookAddress(in.Address) {
		return ErrWebhookAddressInvalid
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrWebhookURLInvalid
	}
	return nil
}

// validWebhookAddress accepts an exact recipient "local@domain" or a bare
// domain "domain" (catch-all). The domain part must be a plausible hostname.
func validWebhookAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	if at := strings.IndexByte(addr, '@'); at >= 0 {
		local, domain := addr[:at], addr[at+1:]
		return local != "" && reWebhookHost.MatchString(domain) && strings.Contains(domain, ".")
	}
	return reWebhookHost.MatchString(addr) && strings.Contains(addr, ".")
}

func (s *MailWebhookService) List(ctx context.Context, limit, offset int) ([]MailWebhookRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

func (s *MailWebhookService) Get(ctx context.Context, id uint32) (*MailWebhookRow, error) {
	if id == 0 {
		return nil, errors.New("mail_webhook: id required")
	}
	return s.store.Get(ctx, id)
}

func (s *MailWebhookService) Create(ctx context.Context, in *MailWebhookRow) (*MailWebhookRow, error) {
	if in == nil {
		return nil, ErrWebhookNameInvalid
	}
	normaliseWebhook(in)
	if err := s.validate(in); err != nil {
		return nil, err
	}
	return s.store.Create(ctx, *in)
}

func (s *MailWebhookService) Update(ctx context.Context, id uint32, in *MailWebhookRow) (*MailWebhookRow, error) {
	if id == 0 || in == nil {
		return nil, errors.New("mail_webhook: id required")
	}
	normaliseWebhook(in)
	if err := s.validate(in); err != nil {
		return nil, err
	}
	return s.store.Update(ctx, id, *in)
}

func (s *MailWebhookService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return errors.New("mail_webhook: id required")
	}
	return s.store.Delete(ctx, id)
}

func normaliseWebhook(in *MailWebhookRow) {
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.ToLower(strings.TrimSpace(in.Address))
	in.URL = strings.TrimSpace(in.URL)
	in.Secret = strings.TrimSpace(in.Secret)
}
