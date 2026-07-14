package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// enrollTokenTTL bounds how long an issued bootstrap token stays redeemable.
const enrollTokenTTL = time.Hour

// maxCSRBytes bounds the enrollment CSR payload.
const maxCSRBytes = 16 << 10

// CSRSigner signs an agent certificate request with the cluster CA.
// Implemented by the clusterca-backed signer; nil when no CA is configured.
type CSRSigner interface {
	SignCSR(csrPEM []byte) (certPEM, fingerprint, caPEM string, err error)
}

// ClusterEnrollUsecase issues single-use bootstrap tokens (operator side) and
// redeems them for CA-signed agent certificates (node side). The redeem path
// is unauthenticated by design — it is the bootstrap — so it is guarded by the
// bcrypt-hashed, expiring, single-use token bound to one node.
type ClusterEnrollUsecase struct {
	repo    MTANodeRepo
	signer  CSRSigner
	auditor *Auditor
}

// NewClusterEnrollUsecase constructs the use case. signer may be nil (no CA
// configured); enrollment then fails with a clear precondition error.
func NewClusterEnrollUsecase(repo MTANodeRepo, signer CSRSigner, auditor *Auditor) *ClusterEnrollUsecase {
	return &ClusterEnrollUsecase{repo: repo, signer: signer, auditor: auditor}
}

// IssueToken mints a bootstrap token for the node. The plaintext is returned
// exactly once; only its bcrypt hash is stored.
func (uc *ClusterEnrollUsecase) IssueToken(ctx context.Context, nodeID string) (plaintext string, expiresAt time.Time, err error) {
	if _, err := RequirePermission(ctx, PermClusterWrite); err != nil {
		return "", time.Time{}, err
	}
	if uc.signer == nil {
		return "", time.Time{}, FailedPrecondition("CLUSTER_CA_UNCONFIGURED", "cluster.ca_dir is not configured; create a CA with `iris cluster init-ca` and point cluster.ca_dir at it")
	}
	node, err := uc.repo.GetNode(ctx, nodeID)
	if err != nil {
		return "", time.Time{}, err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, Internal(err, "generate enrollment token")
	}
	plaintext = hex.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", time.Time{}, Internal(err, "hash enrollment token")
	}
	id := IdentityFrom(ctx)
	createdBy := ""
	if id != nil {
		createdBy = id.Email
	}
	tok, err := uc.repo.CreateEnrollToken(ctx, &MTANodeEnrollToken{
		NodeID:    node.ID,
		TokenHash: string(hash),
		ExpiresAt: time.Now().Add(enrollTokenTTL),
		CreatedBy: createdBy,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	uc.audit(ctx, "cluster.node.enroll_token", node.ID, AuditSuccess, map[string]any{"name": node.Name})
	return plaintext, tok.ExpiresAt, nil
}

// Enroll redeems a bootstrap token: it verifies the token against the node's
// open tokens, consumes it (single use), signs the CSR with the cluster CA,
// and pins the issued certificate's fingerprint on the node.
func (uc *ClusterEnrollUsecase) Enroll(ctx context.Context, nodeName, token string, csrPEM []byte) (certPEM, caPEM string, err error) {
	if uc.signer == nil {
		return "", "", FailedPrecondition("CLUSTER_CA_UNCONFIGURED", "enrollment is not enabled on this iris (no cluster CA configured)")
	}
	nodeName = strings.TrimSpace(nodeName)
	if nodeName == "" || strings.TrimSpace(token) == "" {
		return "", "", Invalid("ENROLL_REQUEST_INVALID", "node and token are required")
	}
	if len(csrPEM) == 0 || len(csrPEM) > maxCSRBytes {
		return "", "", Invalid("ENROLL_CSR_INVALID", "csr is required and must be at most %d bytes", maxCSRBytes)
	}

	node, err := uc.nodeByName(ctx, nodeName)
	if err != nil {
		return "", "", err
	}
	tokens, err := uc.repo.OpenEnrollTokens(ctx, node.ID)
	if err != nil {
		return "", "", err
	}
	var matched *MTANodeEnrollToken
	for _, t := range tokens {
		if bcrypt.CompareHashAndPassword([]byte(t.TokenHash), []byte(token)) == nil {
			matched = t
			break
		}
	}
	if matched == nil {
		uc.audit(ctx, "cluster.node.enroll", node.ID, AuditFailure, map[string]any{"name": node.Name, "reason": "token mismatch"})
		return "", "", Unauthorized("ENROLL_TOKEN_INVALID", "enrollment token is invalid, expired, or already used")
	}
	// Consume BEFORE signing: a token must never be redeemable twice, even if
	// signing subsequently fails (the operator issues a fresh token instead).
	if err := uc.repo.ConsumeEnrollToken(ctx, matched.ID); err != nil {
		return "", "", err
	}

	certPEM, fingerprint, caPEM, err := uc.signer.SignCSR(csrPEM)
	if err != nil {
		uc.audit(ctx, "cluster.node.enroll", node.ID, AuditFailure, map[string]any{"name": node.Name, "reason": err.Error()})
		return "", "", Invalid("ENROLL_CSR_REJECTED", "certificate request rejected: %v", err)
	}
	if err := uc.repo.SetNodeCertFingerprint(ctx, node.ID, fingerprint); err != nil {
		return "", "", err
	}
	uc.audit(ctx, "cluster.node.enroll", node.ID, AuditSuccess, map[string]any{
		"name": node.Name, "fingerprint": fingerprint,
	})
	return certPEM, caPEM, nil
}

func (uc *ClusterEnrollUsecase) nodeByName(ctx context.Context, name string) (*MTANode, error) {
	nodes, err := uc.repo.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		if n.Name == name {
			return n, nil
		}
	}
	// Deliberately the same error as a token mismatch: the unauthenticated
	// endpoint must not confirm which node names exist.
	return nil, Unauthorized("ENROLL_TOKEN_INVALID", "enrollment token is invalid, expired, or already used")
}

func (uc *ClusterEnrollUsecase) audit(ctx context.Context, op, id string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, "mta_node", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
