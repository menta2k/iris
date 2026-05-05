package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	appcrypto "github.com/menta2k/iris/backend/pkg/crypto"
)

// UserAdminStore is the data interface for user CRUD.
type UserAdminStore interface {
	List(ctx context.Context, limit, offset int) ([]UserRow, uint32, error)
	Get(ctx context.Context, id uint32) (*UserRow, error)
	Create(ctx context.Context, in UserCreateInput) (*UserRow, error)
	Update(ctx context.Context, id uint32, in UserUpdateInput) (*UserRow, error)
	Delete(ctx context.Context, id uint32) error
	UpdatePassword(ctx context.Context, id uint32, hash string) error
	GetPasswordHash(ctx context.Context, id uint32) (string, error)
}

// UserCreateInput is the storage-shape create payload.
type UserCreateInput struct {
	Username     string
	Email        string
	DisplayName  string
	PasswordHash string
	Roles        []string
	Active       bool
}

// UserUpdateInput is the storage-shape update payload. nil pointers mean
// "do not change".
type UserUpdateInput struct {
	Email       *string
	DisplayName *string
	Active      *bool
	Roles       *[]string
}

// BcryptCost is a wire-friendly named type for the cost parameter so the DI
// graph can supply it without colliding with other plain ints. A zero value
// is replaced with appcrypto.MinBcryptCost on construction.
type BcryptCost int

// UserService implements user CRUD.
type UserService struct {
	store      UserAdminStore
	bcryptCost int
}

// NewUserService constructs the service.
func NewUserService(store UserAdminStore, bcryptCost BcryptCost) *UserService {
	c := int(bcryptCost)
	if c < appcrypto.MinBcryptCost {
		c = appcrypto.MinBcryptCost
	}
	return &UserService{store: store, bcryptCost: c}
}

var (
	reUsername = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`)
	reEmail    = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
)

var (
	ErrInvalidUsername = errors.New("user: invalid username")
	ErrInvalidEmail    = errors.New("user: invalid email")
	ErrPasswordWeak    = errors.New("user: password does not meet policy")
	ErrInvalidPassword = errors.New("user: current password incorrect")
)

// CreateUserRequest is the API-layer payload.
type CreateUserRequest struct {
	Username    string
	Email       string
	DisplayName string
	Password    string
	Roles       []string
}

// Create validates inputs, hashes the password, and inserts.
func (s *UserService) Create(ctx context.Context, req *CreateUserRequest) (*UserRow, error) {
	if !reUsername.MatchString(req.Username) {
		return nil, ErrInvalidUsername
	}
	if !reEmail.MatchString(req.Email) {
		return nil, ErrInvalidEmail
	}
	if err := passwordPolicy(req.Password); err != nil {
		return nil, err
	}
	hash, err := appcrypto.HashPassword(req.Password, s.bcryptCost)
	if err != nil {
		return nil, err
	}
	return s.store.Create(ctx, UserCreateInput{
		Username:     req.Username,
		Email:        strings.ToLower(req.Email),
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		Roles:        req.Roles,
		Active:       true,
	})
}

// List paginates users.
func (s *UserService) List(ctx context.Context, limit, offset int) ([]UserRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get one user.
func (s *UserService) Get(ctx context.Context, id uint32) (*UserRow, error) {
	if id == 0 {
		return nil, errors.New("user: id required")
	}
	return s.store.Get(ctx, id)
}

// Update changes mutable fields.
func (s *UserService) Update(ctx context.Context, id uint32, in UserUpdateInput) (*UserRow, error) {
	if id == 0 {
		return nil, errors.New("user: id required")
	}
	if in.Email != nil && !reEmail.MatchString(*in.Email) {
		return nil, ErrInvalidEmail
	}
	if in.Email != nil {
		lc := strings.ToLower(*in.Email)
		in.Email = &lc
	}
	return s.store.Update(ctx, id, in)
}

// Delete removes (or soft-deletes) a user.
func (s *UserService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return errors.New("user: id required")
	}
	return s.store.Delete(ctx, id)
}

// ChangePassword updates a user's password. If `old` is non-empty the caller
// must demonstrate knowledge of it (self-service flow). When old is empty
// the call is treated as an admin reset (caller authorization is the gRPC
// middleware's responsibility).
func (s *UserService) ChangePassword(ctx context.Context, id uint32, oldPassword, newPassword string) error {
	if id == 0 {
		return errors.New("user: id required")
	}
	if err := passwordPolicy(newPassword); err != nil {
		return err
	}
	if oldPassword != "" {
		stored, err := s.store.GetPasswordHash(ctx, id)
		if err != nil {
			return err
		}
		if err := appcrypto.VerifyPassword(stored, oldPassword); err != nil {
			return ErrInvalidPassword
		}
	}
	hash, err := appcrypto.HashPassword(newPassword, s.bcryptCost)
	if err != nil {
		return err
	}
	return s.store.UpdatePassword(ctx, id, hash)
}

// passwordPolicy enforces a minimum standard. Refuse anything below 12 chars
// or with no character variety.
func passwordPolicy(pw string) error {
	if len(pw) < 12 {
		return ErrPasswordWeak
	}
	if len(pw) > 72 {
		return ErrPasswordWeak
	}
	hasUpper, hasLower, hasDigitOrPunct := false, false, false
	for _, r := range pw {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		default:
			hasDigitOrPunct = true
		}
	}
	if !(hasUpper && hasLower && hasDigitOrPunct) {
		return ErrPasswordWeak
	}
	return nil
}

var _ = time.Time{} // keep import for fmt-stable header
