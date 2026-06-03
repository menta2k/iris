package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DkimRow is the data-layer view of a DKIM identity.
type DkimRow struct {
	ID           uint32
	Domain       string
	Selector     string
	Algorithm    string // "ed25519" | "rsa-1024" | "rsa-2048" | "rsa-4096"
	PublicKeyPEM string
	KeyPath      string
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DkimStore is the data-layer interface.
type DkimStore interface {
	List(ctx context.Context, limit, offset int) ([]DkimRow, uint32, error)
	Get(ctx context.Context, id uint32) (*DkimRow, error)
	Create(ctx context.Context, in DkimRow) (*DkimRow, error)
	UpdateKey(ctx context.Context, id uint32, publicPEM, keyPath, algorithm string) (*DkimRow, error)
	Delete(ctx context.Context, id uint32) error
}

// KeyMaterial is the result of generating a fresh DKIM keypair.
type KeyMaterial struct {
	PublicPEM  string
	PrivatePEM string
	Algorithm  string
}

// KeyGenerator produces DKIM keypairs. Split out so tests can supply
// deterministic keys without touching crypto/rand.
type KeyGenerator interface {
	Generate(algorithm string) (*KeyMaterial, error)
}

// DkimService implements DKIM identity CRUD + key rotation.
type DkimService struct {
	store   DkimStore
	keygen  KeyGenerator
	keysDir string
}

// NewDkimService constructs the service. keysDir is where private keys are
// written on rotation; must be an absolute path that the kumomta process
// can read.
func NewDkimService(store DkimStore, keygen KeyGenerator, keysDir string) (*DkimService, error) {
	if !filepath.IsAbs(keysDir) {
		return nil, fmt.Errorf("dkim: keys_dir must be absolute, got %q", keysDir)
	}
	if filepath.Clean(keysDir) != keysDir {
		return nil, errors.New("dkim: keys_dir must be in canonical form")
	}
	return &DkimService{store: store, keygen: keygen, keysDir: keysDir}, nil
}

var (
	ErrDkimDomain     = errors.New("dkim: domain invalid")
	ErrDkimSelector   = errors.New("dkim: selector invalid")
	ErrDkimAlgorithm  = errors.New("dkim: algorithm must be ed25519|rsa-1024|rsa-2048|rsa-4096")
	ErrDkimPrivateKey = errors.New("dkim: private_key_pem invalid")
	ErrDkimMismatch   = errors.New("dkim: private key does not match declared algorithm")
)

var (
	reDkimDomain   = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
	reDkimSelector = regexp.MustCompile(`^[A-Za-z0-9._-]{1,63}$`)
)

// CreateDkimRequest is the API-layer payload. Either provide PrivateKeyPEM
// to import an existing key, or leave it empty to generate a fresh one.
type CreateDkimRequest struct {
	Domain        string
	Selector      string
	Algorithm     string
	PrivateKeyPEM string // optional; if set, the service imports instead of generating
}

// List paginates DKIM identities.
func (s *DkimService) List(ctx context.Context, limit, offset int) ([]DkimRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get one identity.
func (s *DkimService) Get(ctx context.Context, id uint32) (*DkimRow, error) {
	if id == 0 {
		return nil, errors.New("dkim: id required")
	}
	return s.store.Get(ctx, id)
}

// Create either generates a fresh keypair or imports the supplied PEM. The
// imported path is taken when req.PrivateKeyPEM is non-empty; the public key
// is derived from the supplied private key (we never trust caller-supplied
// public material). The on-disk key file is the imported PEM verbatim so
// kumomta sees byte-identical input across import → use.
func (s *DkimService) Create(ctx context.Context, req *CreateDkimRequest) (*DkimRow, error) {
	if req == nil || !reDkimDomain.MatchString(req.Domain) {
		return nil, ErrDkimDomain
	}
	if !reDkimSelector.MatchString(req.Selector) {
		return nil, ErrDkimSelector
	}
	var mat *KeyMaterial
	if strings.TrimSpace(req.PrivateKeyPEM) != "" {
		imported, err := importPrivateKey(req.PrivateKeyPEM, req.Algorithm)
		if err != nil {
			return nil, err
		}
		mat = imported
	} else {
		generated, err := s.keygen.Generate(req.Algorithm)
		if err != nil {
			return nil, fmt.Errorf("dkim: generate: %w", err)
		}
		mat = generated
	}
	keyPath, err := s.writePrivate(req.Domain, req.Selector, mat.PrivatePEM)
	if err != nil {
		return nil, err
	}
	return s.store.Create(ctx, DkimRow{
		Domain:       req.Domain,
		Selector:     req.Selector,
		Algorithm:    mat.Algorithm,
		PublicKeyPEM: mat.PublicPEM,
		KeyPath:      keyPath,
		Active:       true,
	})
}

// importPrivateKey parses a PEM-encoded private key, derives the matching
// public key, and confirms the algorithm matches what the caller declared.
// PKCS#8 ("PRIVATE KEY") is the canonical format; legacy "RSA PRIVATE KEY"
// (PKCS#1) is also accepted because openssl ships it as the default.
func importPrivateKey(pemStr, declaredAlgo string) (*KeyMaterial, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, ErrDkimPrivateKey
	}
	var (
		actualAlgo string
		pubPKIX    []byte
		privPKCS8  []byte
		err        error
	)
	switch block.Type {
	case "PRIVATE KEY":
		key, perr := x509.ParsePKCS8PrivateKey(block.Bytes)
		if perr != nil {
			return nil, fmt.Errorf("%w: %v", ErrDkimPrivateKey, perr)
		}
		switch k := key.(type) {
		case ed25519.PrivateKey:
			actualAlgo = "ed25519"
			pub := k.Public().(ed25519.PublicKey)
			pubPKIX, err = x509.MarshalPKIXPublicKey(pub)
			if err == nil {
				privPKCS8, err = x509.MarshalPKCS8PrivateKey(k)
			}
		case *rsa.PrivateKey:
			actualAlgo = rsaAlgoForBits(k.N.BitLen())
			if actualAlgo == "" {
				return nil, fmt.Errorf("%w: rsa key bit-size %d not supported (use 1024, 2048 or 4096)", ErrDkimPrivateKey, k.N.BitLen())
			}
			pubPKIX, err = x509.MarshalPKIXPublicKey(&k.PublicKey)
			if err == nil {
				privPKCS8, err = x509.MarshalPKCS8PrivateKey(k)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported key type %T", ErrDkimPrivateKey, key)
		}
	case "RSA PRIVATE KEY":
		k, perr := x509.ParsePKCS1PrivateKey(block.Bytes)
		if perr != nil {
			return nil, fmt.Errorf("%w: %v", ErrDkimPrivateKey, perr)
		}
		actualAlgo = rsaAlgoForBits(k.N.BitLen())
		if actualAlgo == "" {
			return nil, fmt.Errorf("%w: rsa key bit-size %d not supported (use 1024, 2048 or 4096)", ErrDkimPrivateKey, k.N.BitLen())
		}
		pubPKIX, err = x509.MarshalPKIXPublicKey(&k.PublicKey)
		if err == nil {
			privPKCS8, err = x509.MarshalPKCS8PrivateKey(k)
		}
	default:
		return nil, fmt.Errorf("%w: unexpected PEM type %q (expected PRIVATE KEY or RSA PRIVATE KEY)", ErrDkimPrivateKey, block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDkimPrivateKey, err)
	}
	if declaredAlgo != "" && declaredAlgo != actualAlgo {
		// Special case: caller may declare just "rsa" — accept any RSA size
		// the key actually is. Otherwise mismatch is fatal.
		if !(declaredAlgo == "rsa" && strings.HasPrefix(actualAlgo, "rsa-")) {
			return nil, fmt.Errorf("%w (declared %q, key is %q)", ErrDkimMismatch, declaredAlgo, actualAlgo)
		}
	}
	return &KeyMaterial{
		PrivatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privPKCS8})),
		PublicPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubPKIX})),
		Algorithm:  actualAlgo,
	}, nil
}

func rsaAlgoForBits(bits int) string {
	switch bits {
	case 1024:
		return "rsa-1024"
	case 2048:
		return "rsa-2048"
	case 4096:
		return "rsa-4096"
	default:
		return ""
	}
}

// Rotate replaces the on-disk private key and the stored public PEM. The
// algorithm is preserved unless the row was empty (legacy) — in that case
// we default to ed25519.
func (s *DkimService) Rotate(ctx context.Context, id uint32) (*DkimRow, error) {
	row, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	algo := row.Algorithm
	if algo == "" {
		algo = "ed25519"
	}
	mat, err := s.keygen.Generate(algo)
	if err != nil {
		return nil, fmt.Errorf("dkim: generate: %w", err)
	}
	keyPath, err := s.writePrivate(row.Domain, row.Selector, mat.PrivatePEM)
	if err != nil {
		return nil, err
	}
	return s.store.UpdateKey(ctx, id, mat.PublicPEM, keyPath, mat.Algorithm)
}

// Delete removes the identity. The on-disk private key is *not* deleted —
// active mail in flight may still need it; admins clean up out-of-band.
func (s *DkimService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return errors.New("dkim: id required")
	}
	return s.store.Delete(ctx, id)
}

// writePrivate writes the private key PEM to <keysDir>/<domain>.<selector>.key
// with mode 0600 and atomic rename. Returns the absolute path.
//
// The keys dir is created on demand (mode 0700) — this lets dev/CI runs work
// without a pre-provisioned mount, and lets prod operators rely on the
// service rather than container init for the directory.
func (s *DkimService) writePrivate(domain, selector, pemStr string) (string, error) {
	name := fmt.Sprintf("%s.%s.key", domain, selector)
	dest := filepath.Join(s.keysDir, name)
	if filepath.Clean(dest) != dest {
		return "", errors.New("dkim: dest path not canonical")
	}
	if err := os.MkdirAll(s.keysDir, 0o700); err != nil {
		return "", fmt.Errorf("dkim: mkdir keys dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.keysDir, ".dkim.tmp.*")
	if err != nil {
		return "", fmt.Errorf("dkim: tmp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	// 0o644: kumomta runs as a different UID across the docker bind-mount
	// and group memberships don't propagate. The keys are private to the
	// deployment volume; readability inside that volume is acceptable.
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if _, err := tmp.WriteString(pemStr); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", fmt.Errorf("dkim: rename: %w", err)
	}
	return dest, nil
}

// DefaultKeyGenerator generates real keypairs using crypto/rand. Used in
// production; tests inject a deterministic implementation.
type DefaultKeyGenerator struct{}

// Generate produces a keypair for the given algorithm.
func (DefaultKeyGenerator) Generate(algorithm string) (*KeyMaterial, error) {
	switch algorithm {
	case "", "ed25519":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		privPKCS8, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, err
		}
		pubPKIX, err := x509.MarshalPKIXPublicKey(pub)
		if err != nil {
			return nil, err
		}
		return &KeyMaterial{
			PrivatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privPKCS8})),
			PublicPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubPKIX})),
			Algorithm:  "ed25519",
		}, nil
	case "rsa-1024", "rsa-2048", "rsa-4096":
		bits := 2048
		switch algorithm {
		case "rsa-1024":
			bits = 1024
		case "rsa-4096":
			bits = 4096
		}
		key, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return nil, err
		}
		privPKCS8, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, err
		}
		pubPKIX, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			return nil, err
		}
		return &KeyMaterial{
			PrivatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privPKCS8})),
			PublicPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubPKIX})),
			Algorithm:  algorithm,
		}, nil
	default:
		return nil, ErrDkimAlgorithm
	}
}
