package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	appcrypto "github.com/menta2k/iris/backend/pkg/crypto"
)

type fakeUserAdminStore struct {
	mu   sync.Mutex
	rows []UserRow
	next uint32
}

func (f *fakeUserAdminStore) List(ctx context.Context, limit, offset int) ([]UserRow, uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	end := offset + limit
	if end > len(f.rows) {
		end = len(f.rows)
	}
	if offset > end {
		offset = end
	}
	out := append([]UserRow(nil), f.rows[offset:end]...)
	return out, uint32(len(f.rows)), nil
}

func (f *fakeUserAdminStore) Get(ctx context.Context, id uint32) (*UserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			r := f.rows[i]
			return &r, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeUserAdminStore) Create(ctx context.Context, in UserCreateInput) (*UserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	row := UserRow{
		ID: f.next, Username: in.Username, PasswordHash: in.PasswordHash,
		Active: in.Active, Roles: in.Roles,
	}
	f.rows = append(f.rows, row)
	return &row, nil
}

func (f *fakeUserAdminStore) Update(ctx context.Context, id uint32, in UserUpdateInput) (*UserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			if in.Roles != nil {
				f.rows[i].Roles = *in.Roles
			}
			if in.Active != nil {
				f.rows[i].Active = *in.Active
			}
			r := f.rows[i]
			return &r, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeUserAdminStore) Delete(ctx context.Context, id uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (f *fakeUserAdminStore) UpdatePassword(ctx context.Context, id uint32, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].PasswordHash = hash
			return nil
		}
	}
	return errors.New("not found")
}

func (f *fakeUserAdminStore) GetPasswordHash(ctx context.Context, id uint32) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			return f.rows[i].PasswordHash, nil
		}
	}
	return "", errors.New("not found")
}

func TestUserCreateValid(t *testing.T) {
	svc := NewUserService(&fakeUserAdminStore{}, appcrypto.MinBcryptCost)
	row, err := svc.Create(context.Background(), &CreateUserRequest{
		Username: "alice", Email: "alice@example.com", Password: "Strong1Pwxxx",
		Roles: []string{"admin"},
	})
	require.NoError(t, err)
	require.Equal(t, uint32(1), row.ID)
	require.NotEmpty(t, row.PasswordHash)
}

func TestUserCreateRejectsBadUsername(t *testing.T) {
	svc := NewUserService(&fakeUserAdminStore{}, appcrypto.MinBcryptCost)
	_, err := svc.Create(context.Background(), &CreateUserRequest{
		Username: "ab", Email: "a@b.com", Password: "Strong1Pwxxx",
	})
	require.ErrorIs(t, err, ErrInvalidUsername)
}

func TestUserCreateRejectsBadEmail(t *testing.T) {
	svc := NewUserService(&fakeUserAdminStore{}, appcrypto.MinBcryptCost)
	_, err := svc.Create(context.Background(), &CreateUserRequest{
		Username: "alice", Email: "not-an-email", Password: "Strong1Pwxxx",
	})
	require.ErrorIs(t, err, ErrInvalidEmail)
}

func TestUserCreateRejectsWeakPassword(t *testing.T) {
	svc := NewUserService(&fakeUserAdminStore{}, appcrypto.MinBcryptCost)
	weak := []string{"short", "alllowercase!", "ALLUPPERCASE!", "12345678901234"}
	for _, p := range weak {
		_, err := svc.Create(context.Background(), &CreateUserRequest{
			Username: "alice", Email: "a@b.com", Password: p,
		})
		require.ErrorIs(t, err, ErrPasswordWeak, p)
	}
}

func TestUserChangePasswordSelfService(t *testing.T) {
	store := &fakeUserAdminStore{}
	svc := NewUserService(store, appcrypto.MinBcryptCost)
	_, err := svc.Create(context.Background(), &CreateUserRequest{
		Username: "alice", Email: "a@b.com", Password: "Strong1Pwxxx",
	})
	require.NoError(t, err)
	require.NoError(t, svc.ChangePassword(context.Background(), 1, "Strong1Pwxxx", "Stronger2Pwxxx"))
}

func TestUserChangePasswordRefusesWrongOld(t *testing.T) {
	store := &fakeUserAdminStore{}
	svc := NewUserService(store, appcrypto.MinBcryptCost)
	_, err := svc.Create(context.Background(), &CreateUserRequest{
		Username: "alice", Email: "a@b.com", Password: "Strong1Pwxxx",
	})
	require.NoError(t, err)
	err = svc.ChangePassword(context.Background(), 1, "Wrong1Pwxxxx", "Stronger2Pwxxx")
	require.ErrorIs(t, err, ErrInvalidPassword)
}

func TestUserChangePasswordAdminResetSkipsOld(t *testing.T) {
	store := &fakeUserAdminStore{}
	svc := NewUserService(store, appcrypto.MinBcryptCost)
	_, err := svc.Create(context.Background(), &CreateUserRequest{
		Username: "alice", Email: "a@b.com", Password: "Strong1Pwxxx",
	})
	require.NoError(t, err)
	require.NoError(t, svc.ChangePassword(context.Background(), 1, "", "BrandNew2Pwxxxx"))
}

func TestUserUpdate(t *testing.T) {
	store := &fakeUserAdminStore{}
	svc := NewUserService(store, appcrypto.MinBcryptCost)
	_, err := svc.Create(context.Background(), &CreateUserRequest{
		Username: "alice", Email: "a@b.com", Password: "Strong1Pwxxx",
		Roles: []string{"admin"},
	})
	require.NoError(t, err)
	off := false
	row, err := svc.Update(context.Background(), 1, UserUpdateInput{Active: &off})
	require.NoError(t, err)
	require.False(t, row.Active)
}
