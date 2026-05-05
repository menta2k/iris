// Package jwt issues and verifies HS512 JWTs for access and refresh tokens.
//
// We deliberately avoid asymmetric algorithms here: the admin service is the
// only producer and consumer, a shared secret is the simplest correct option,
// and HS512 produces compact tokens. The `kid` header is included to
// support seamless secret rotation.
package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

var (
	ErrTokenInvalid    = errors.New("jwt: token invalid")
	ErrTokenExpired    = errors.New("jwt: token expired")
	ErrTokenWrongType  = errors.New("jwt: wrong token type")
	ErrSecretTooShort  = errors.New("jwt: secret must be >= 32 bytes")
	ErrUnknownKey      = errors.New("jwt: unknown signing key id")
	ErrUnsupportedAlg  = errors.New("jwt: unsupported signing algorithm")
)

// Claims is the canonical claims body. It embeds jwt.RegisteredClaims so the
// standard fields (iss, aud, exp, iat, nbf, sub) are present.
type Claims struct {
	jwt.RegisteredClaims
	UserID    uint32   `json:"uid"`
	Username  string   `json:"unm"`
	Roles     []string `json:"rls,omitempty"`
	TokenType string   `json:"tt"`
}

// Issuer signs and verifies tokens.
type Issuer struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
	issuer        string
	audience      []string
	keyID         string
}

// Config configures an Issuer.
type Config struct {
	AccessSecret  []byte
	RefreshSecret []byte
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	Issuer        string
	Audience      []string
	KeyID         string
}

// NewIssuer enforces sane defaults and minimum secret size.
func NewIssuer(cfg Config) (*Issuer, error) {
	if len(cfg.AccessSecret) < 32 || len(cfg.RefreshSecret) < 32 {
		return nil, ErrSecretTooShort
	}
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = time.Hour
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 7 * 24 * time.Hour
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "iris"
	}
	if cfg.KeyID == "" {
		cfg.KeyID = "default"
	}
	return &Issuer{
		accessSecret:  cfg.AccessSecret,
		refreshSecret: cfg.RefreshSecret,
		accessTTL:     cfg.AccessTTL,
		refreshTTL:    cfg.RefreshTTL,
		issuer:        cfg.Issuer,
		audience:      cfg.Audience,
		keyID:         cfg.KeyID,
	}, nil
}

// IssueAccess returns a signed access token.
func (i *Issuer) IssueAccess(userID uint32, username string, roles []string, now time.Time) (string, time.Time, error) {
	exp := now.Add(i.accessTTL)
	return i.sign(tokenTypeAccess, userID, username, roles, now, exp, i.accessSecret)
}

// IssueRefresh returns a signed refresh token.
func (i *Issuer) IssueRefresh(userID uint32, username string, now time.Time) (string, time.Time, error) {
	exp := now.Add(i.refreshTTL)
	return i.sign(tokenTypeRefresh, userID, username, nil, now, exp, i.refreshSecret)
}

func (i *Issuer) sign(typ string, uid uint32, username string, roles []string, iat, exp time.Time, secret []byte) (string, time.Time, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Audience:  i.audience,
			Subject:   fmt.Sprintf("user:%d", uid),
			IssuedAt:  jwt.NewNumericDate(iat),
			NotBefore: jwt.NewNumericDate(iat),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        fmt.Sprintf("%d-%s-%d", uid, typ, iat.UnixNano()),
		},
		UserID:    uid,
		Username:  username,
		Roles:     roles,
		TokenType: typ,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	tok.Header["kid"] = i.keyID
	signed, err := tok.SignedString(secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// VerifyAccess parses an access token and returns the claims.
func (i *Issuer) VerifyAccess(token string) (*Claims, error) {
	return i.parse(token, tokenTypeAccess, i.accessSecret)
}

// VerifyRefresh parses a refresh token and returns the claims.
func (i *Issuer) VerifyRefresh(token string) (*Claims, error) {
	return i.parse(token, tokenTypeRefresh, i.refreshSecret)
}

func (i *Issuer) parse(token, expectedType string, secret []byte) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlg, t.Method.Alg())
		}
		// Future hook for multi-key rotation.
		return secret, nil
	},
		jwt.WithValidMethods([]string{"HS512"}),
		jwt.WithIssuer(i.issuer),
	)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrTokenExpired
		default:
			return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
		}
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrTokenInvalid
	}
	if claims.TokenType != expectedType {
		return nil, ErrTokenWrongType
	}
	return claims, nil
}
