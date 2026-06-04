package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appcrypto "github.com/menta2k/iris/backend/pkg/crypto"
	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
)

type fakeUsers struct {
	mu sync.Mutex
	m  map[string]*UserRow
	locked map[uint32]bool
	successes []string
	failures  []string
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{m: map[string]*UserRow{}, locked: map[uint32]bool{}}
}

func (f *fakeUsers) FindByUsername(ctx context.Context, u string) (*UserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.m[u]
	if !ok {
		return nil, errors.New("not found")
	}
	return r, nil
}
func (f *fakeUsers) RecordLoginSuccess(ctx context.Context, uid uint32, ip string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.successes = append(f.successes, ip)
	return nil
}
func (f *fakeUsers) RecordLoginFailure(ctx context.Context, u string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, u)
	return nil
}
func (f *fakeUsers) IsLockedOut(ctx context.Context, uid uint32, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.locked[uid], nil
}

func newSvc(t *testing.T) (*AuthenticationService, *fakeUsers) {
	t.Helper()
	iss, err := appjwt.NewIssuer(appjwt.Config{
		AccessSecret:  []byte(strings.Repeat("a", 32)),
		RefreshSecret: []byte(strings.Repeat("b", 32)),
		AccessTTL:     time.Hour,
		RefreshTTL:    24 * time.Hour,
	})
	require.NoError(t, err)
	users := newFakeUsers()

	hash, err := appcrypto.HashPassword("super-secret-password!", appcrypto.MinBcryptCost)
	require.NoError(t, err)
	users.m["alice"] = &UserRow{ID: 1, Username: "alice", PasswordHash: hash, Active: true, Roles: []string{"admin"}}
	users.m["bob"] = &UserRow{ID: 2, Username: "bob", PasswordHash: hash, Active: false}

	return NewAuthenticationService(users, iss, nil, nil), users
}

// newSvcWithFirewall is like newSvc but installs a login firewall over the
// given rule source (geo nil).
func newSvcWithFirewall(t *testing.T, rules RuleSource) (*AuthenticationService, *fakeUsers) {
	t.Helper()
	svc, users := newSvc(t)
	svc.firewall = NewLoginFirewall(rules, nil)
	return svc, users
}

func TestLoginBlockedByFirewall(t *testing.T) {
	rules := &fakeRuleSource{rows: []LoginPolicyRow{
		{ID: 7, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "1.2.3.0/24", Enabled: true},
	}}
	svc, users := newSvcWithFirewall(t, rules)
	_, err := svc.Login(context.Background(),
		&LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.ErrorIs(t, err, ErrLoginBlocked)
	// A blocked source must not reach the password / lockout path.
	require.Empty(t, users.failures, "blocked login must not record a credential failure")
	require.Empty(t, users.successes, "blocked login must not record a success")
}

func TestLoginAllowedWhenFirewallNoMatch(t *testing.T) {
	rules := &fakeRuleSource{rows: []LoginPolicyRow{
		{ID: 7, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "10.0.0.0/8", Enabled: true},
	}}
	svc, _ := newSvcWithFirewall(t, rules)
	resp, err := svc.Login(context.Background(),
		&LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
}

func TestLoginFirewallFailsOpenOnStoreError(t *testing.T) {
	rules := &fakeRuleSource{err: errors.New("db down")}
	svc, _ := newSvcWithFirewall(t, rules)
	resp, err := svc.Login(context.Background(),
		&LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.NoError(t, err, "a rule-store error must fail open, not block login")
	require.NotEmpty(t, resp.AccessToken)
}

func TestLoginSuccess(t *testing.T) {
	svc, users := newSvc(t)
	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.NotEmpty(t, resp.RefreshToken)
	require.Equal(t, uint32(1), resp.UserID)
	require.Contains(t, resp.Roles, "admin")
	require.Equal(t, []string{"1.2.3.4"}, users.successes)
}

func TestLoginWrongPassword(t *testing.T) {
	svc, users := newSvc(t)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "alice", Password: "wrong"}, "1.2.3.4")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	require.Equal(t, []string{"alice"}, users.failures)
}

func TestLoginUnknownUser(t *testing.T) {
	svc, users := newSvc(t)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "ghost", Password: "any-password-1234"}, "1.2.3.4")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	// Failure should be recorded under the supplied username for forensic
	// reasons even though the user does not exist — we never want to leak
	// existence via differential logging.
	require.Equal(t, []string{"ghost"}, users.failures)
}

func TestLoginInactive(t *testing.T) {
	svc, _ := newSvc(t)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "bob", Password: "super-secret-password!"}, "1.2.3.4")
	require.ErrorIs(t, err, ErrAccountInactive)
}

func TestLoginLockedOut(t *testing.T) {
	svc, users := newSvc(t)
	users.locked[1] = true
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.ErrorIs(t, err, ErrAccountLocked)
}

func TestRefreshTokenSuccess(t *testing.T) {
	svc, _ := newSvc(t)
	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.NoError(t, err)
	resp2, err := svc.RefreshToken(context.Background(), resp.RefreshToken)
	require.NoError(t, err)
	require.NotEmpty(t, resp2.AccessToken)
	require.NotEqual(t, resp.AccessToken, resp2.AccessToken, "new access token should differ")
}

func TestRefreshTokenRejectsBad(t *testing.T) {
	svc, _ := newSvc(t)
	_, err := svc.RefreshToken(context.Background(), "garbage")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestRefreshTokenRejectsWhenUserDeactivated(t *testing.T) {
	svc, users := newSvc(t)
	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.NoError(t, err)
	users.mu.Lock()
	users.m["alice"].Active = false
	users.mu.Unlock()
	_, err = svc.RefreshToken(context.Background(), resp.RefreshToken)
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLoginRequiresMFAWhenEnrolled(t *testing.T) {
	iss, err := appjwt.NewIssuer(appjwt.Config{
		AccessSecret:  []byte(strings.Repeat("a", 32)),
		RefreshSecret: []byte(strings.Repeat("b", 32)),
	})
	require.NoError(t, err)
	users := newFakeUsers()
	hash, err := appcrypto.HashPassword("super-secret-password!", appcrypto.MinBcryptCost)
	require.NoError(t, err)
	users.m["alice"] = &UserRow{ID: 1, Username: "alice", PasswordHash: hash, Active: true, Roles: []string{"admin"}}

	mfa := &memMFAStore{}
	mfa.rows = append(mfa.rows, MFACredentialRow{ID: 1, UserID: 1, Kind: MFAKindTOTP, Status: "active"})
	svc := NewAuthenticationService(users, iss, nil, mfa)

	resp, err := svc.Login(context.Background(),
		&LoginRequest{Username: "alice", Password: "super-secret-password!"}, "1.2.3.4")
	require.NoError(t, err)
	require.True(t, resp.MFARequired, "login must require a second factor when enrolled")
	require.NotEmpty(t, resp.MFAToken)
	require.Empty(t, resp.AccessToken, "no access token before the factor is verified")
	require.Contains(t, resp.MFAMethods, MFAKindTOTP)
	require.Empty(t, users.successes, "RecordLoginSuccess must not run on a half-finished login")
}
