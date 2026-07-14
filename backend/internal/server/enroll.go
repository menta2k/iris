package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/menta2k/iris/backend/internal/biz"
)

// EnrollPath is the unauthenticated agent-enrollment endpoint. It sits outside
// the admin JWT middleware (like /metrics): the bootstrap has no certificate
// yet, so the single-use bcrypt-hashed token IS the authentication.
const EnrollPath = "/cluster/enroll/v1"

// maxEnrollBody bounds the enrollment request (node name + token + CSR).
const maxEnrollBody = 32 << 10

// Enroller is the enrollment use case surface. Satisfied by
// biz.ClusterEnrollUsecase.
type Enroller interface {
	Enroll(ctx context.Context, nodeName, token string, csrPEM []byte) (certPEM, caPEM string, err error)
}

type enrollRequest struct {
	Node  string `json:"node"`
	Token string `json:"token"`
	CSR   string `json:"csr"`
}

type enrollReply struct {
	Cert string `json:"cert"`
	CA   string `json:"ca"`
}

// NewEnrollHandler builds the enrollment HTTP handler.
func NewEnrollHandler(enroller Enroller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req enrollRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxEnrollBody)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		cert, ca, err := enroller.Enroll(r.Context(), req.Node, req.Token, []byte(req.CSR))
		if err != nil {
			var de *biz.DomainError
			status := http.StatusBadRequest
			msg := "enrollment failed"
			if errors.As(err, &de) {
				msg = de.Message
				switch de.Kind {
				case biz.KindUnauthorized:
					status = http.StatusUnauthorized
				case biz.KindFailedPrecondition, biz.KindUnavailable:
					status = http.StatusServiceUnavailable
				}
			}
			writeJSON(w, status, map[string]string{"error": msg})
			return
		}
		writeJSON(w, http.StatusOK, enrollReply{Cert: cert, CA: ca})
	})
}
