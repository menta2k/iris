package clusterca

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnrollOptions drive a node's online enrollment against iris.
type EnrollOptions struct {
	// IrisURL is the iris admin base URL (https://iris-host:port).
	IrisURL string
	// NodeName must match the node's registry entry in iris.
	NodeName string
	// Token is the single-use bootstrap token issued by an operator.
	Token string
	// DNSNames/IPs become the certificate SANs; they must cover the host in
	// the node's agent_url or iris will refuse the TLS handshake later.
	DNSNames []string
	IPs      []net.IP
	// OutDir receives agent.crt / agent.key / ca.crt.
	OutDir string
	// ServerCA optionally pins the CA bundle used to verify the iris HTTPS
	// endpoint during enrollment. Insecure skips verification entirely — the
	// exchanged material is then protected only by the single-use token.
	ServerCA string
	Insecure bool
}

type enrollWireRequest struct {
	Node  string `json:"node"`
	Token string `json:"token"`
	CSR   string `json:"csr"`
}

type enrollWireReply struct {
	Cert  string `json:"cert"`
	CA    string `json:"ca"`
	Error string `json:"error"`
}

// EnrollPath mirrors the iris server's enrollment route.
const EnrollPath = "/cluster/enroll/v1"

// Enroll generates a fresh agent key + CSR, redeems the bootstrap token at
// iris, and writes agent.crt / agent.key / ca.crt into OutDir (key 0600). It
// returns the paths written.
func Enroll(opts EnrollOptions) (certPath, keyPath, caPath string, err error) {
	if opts.IrisURL == "" || opts.NodeName == "" || opts.Token == "" {
		return "", "", "", fmt.Errorf("iris URL, node name, and token are required")
	}
	if len(opts.DNSNames) == 0 && len(opts.IPs) == 0 {
		return "", "", "", fmt.Errorf("at least one SAN (DNS name or IP) is required: it must cover the agent URL host")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("generate key: %w", err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:     pkix.Name{CommonName: opts.NodeName, Organization: []string{"iris"}},
		DNSNames:    opts.DNSNames,
		IPAddresses: opts.IPs,
	}, key)
	if err != nil {
		return "", "", "", fmt.Errorf("create CSR: %w", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	client, err := enrollHTTPClient(opts)
	if err != nil {
		return "", "", "", err
	}
	body, err := json.Marshal(enrollWireRequest{Node: opts.NodeName, Token: opts.Token, CSR: string(csrPEM)})
	if err != nil {
		return "", "", "", fmt.Errorf("encode enrollment request: %w", err)
	}
	url := strings.TrimRight(opts.IrisURL, "/") + EnrollPath
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", "", fmt.Errorf("enrollment request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", "", fmt.Errorf("read enrollment response: %w", err)
	}
	var reply enrollWireReply
	if jerr := json.Unmarshal(raw, &reply); jerr != nil {
		return "", "", "", fmt.Errorf("enrollment returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("enrollment rejected (%d): %s", resp.StatusCode, reply.Error)
	}
	if reply.Cert == "" || reply.CA == "" {
		return "", "", "", fmt.Errorf("enrollment reply is missing certificate material")
	}

	if err := os.MkdirAll(opts.OutDir, 0o700); err != nil {
		return "", "", "", fmt.Errorf("create output directory: %w", err)
	}
	certPath = filepath.Join(opts.OutDir, "agent.crt")
	keyPath = filepath.Join(opts.OutDir, "agent.key")
	caPath = filepath.Join(opts.OutDir, "ca.crt")
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal key: %w", err)
	}
	if err := os.WriteFile(certPath, []byte(reply.Cert), 0o644); err != nil {
		return "", "", "", fmt.Errorf("write agent.crt: %w", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return "", "", "", fmt.Errorf("write agent.key: %w", err)
	}
	if err := os.WriteFile(caPath, []byte(reply.CA), 0o644); err != nil {
		return "", "", "", fmt.Errorf("write ca.crt: %w", err)
	}
	return certPath, keyPath, caPath, nil
}

func enrollHTTPClient(opts EnrollOptions) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if opts.Insecure {
		tlsCfg.InsecureSkipVerify = true // #nosec G402 -- explicit operator opt-in; token remains the auth
	} else if opts.ServerCA != "" {
		raw, err := os.ReadFile(opts.ServerCA)
		if err != nil {
			return nil, fmt.Errorf("read server CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(raw) {
			return nil, fmt.Errorf("server CA %s contains no certificates", opts.ServerCA)
		}
		tlsCfg.RootCAs = pool
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}, nil
}
