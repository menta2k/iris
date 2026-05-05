package service

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"
)

// VirtualMtaRow is the data-layer view of a virtual MTA.
type VirtualMtaRow struct {
	ID                       uint32
	Name                     string
	SourceIPs                []string
	HeloName                 string
	MaxConnections           uint32
	MaxMessagesPerConnection uint32
	ConnectTimeout           uint32
	ProviderProfile          string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// VirtualMtaStore is the data-layer interface.
type VirtualMtaStore interface {
	List(ctx context.Context, limit, offset int) ([]VirtualMtaRow, uint32, error)
	Get(ctx context.Context, id uint32) (*VirtualMtaRow, error)
	Create(ctx context.Context, in VirtualMtaRow) (*VirtualMtaRow, error)
	Update(ctx context.Context, id uint32, in VirtualMtaRow) (*VirtualMtaRow, error)
	Delete(ctx context.Context, id uint32) error
}

// VirtualMtaService implements the gRPC VmtaService methods.
type VirtualMtaService struct{ store VirtualMtaStore }

// NewVirtualMtaService constructs the service.
func NewVirtualMtaService(store VirtualMtaStore) *VirtualMtaService {
	return &VirtualMtaService{store: store}
}

var (
	ErrVmtaNameInvalid  = errors.New("vmta: name must be 1-64 chars [a-zA-Z0-9_.-]")
	ErrVmtaSourceIP     = errors.New("vmta: source_ips contains an invalid address")
	ErrVmtaHeloName     = errors.New("vmta: helo_name not a valid hostname")
	reVmtaName          = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	reHeloName          = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
)

// validate checks name + source IPs + helo (if set). Source IPs are passed
// in normalized (already split + trimmed by the handler).
func (s *VirtualMtaService) validate(in *VirtualMtaRow) error {
	if !reVmtaName.MatchString(in.Name) {
		return ErrVmtaNameInvalid
	}
	for _, ip := range in.SourceIPs {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) == nil {
			return ErrVmtaSourceIP
		}
	}
	if in.HeloName != "" && !reHeloName.MatchString(in.HeloName) {
		return ErrVmtaHeloName
	}
	return nil
}

// List paginates VMTAs.
func (s *VirtualMtaService) List(ctx context.Context, limit, offset int) ([]VirtualMtaRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get one VMTA.
func (s *VirtualMtaService) Get(ctx context.Context, id uint32) (*VirtualMtaRow, error) {
	if id == 0 {
		return nil, errors.New("vmta: id required")
	}
	return s.store.Get(ctx, id)
}

// Create validates and inserts.
func (s *VirtualMtaService) Create(ctx context.Context, in *VirtualMtaRow) (*VirtualMtaRow, error) {
	if in == nil {
		return nil, ErrVmtaNameInvalid
	}
	if err := s.validate(in); err != nil {
		return nil, err
	}
	return s.store.Create(ctx, *in)
}

// Update validates and saves.
func (s *VirtualMtaService) Update(ctx context.Context, id uint32, in *VirtualMtaRow) (*VirtualMtaRow, error) {
	if id == 0 || in == nil {
		return nil, errors.New("vmta: id required")
	}
	if err := s.validate(in); err != nil {
		return nil, err
	}
	return s.store.Update(ctx, id, *in)
}

// Delete removes by id.
func (s *VirtualMtaService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return errors.New("vmta: id required")
	}
	return s.store.Delete(ctx, id)
}
