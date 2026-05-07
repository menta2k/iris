// Package providers exports the service layer's wire ProviderSet.
package providers

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/wire"

	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
	"github.com/menta2k/iris/backend/pkg/kumomta"
	"github.com/menta2k/iris/backend/pkg/metrics"
	"github.com/menta2k/iris/backend/pkg/promquery"
	"github.com/menta2k/iris/backend/pkg/suppressionindex"
)

// ProviderSet wires every service and the cross-layer adapters that bind
// data-layer concrete types to the service-layer interfaces. Adapters are
// explicit (not `wire.Bind`) so the contract is visible at the call site
// and the import graph stays acyclic.
var ProviderSet = wire.NewSet(
	NewJWTIssuer,
	NewBcryptCost,
	NewKumomtaClient,
	NewMetricSnapshotWriter,
	NewMetrics,
	NewPromQueryAPI,
	NewDashboardServiceProvider,
	NewDkimKeyGenerator,
	NewSuppressionIndex,
	NewPolicyDir,
	NewDkimKeysDir,
	AuthStoreFromUserRepo,
	UserAdminStoreFromUserRepo,
	AuditStoreFromAuditReader,
	SuppressionStoreFromRepo,
	VmtaStoreFromRepo,
	RoutingStoreFromRepo,
	DkimStoreFromRepo,
	FeedbackStoreFromRepo,
	LogStoreFromRepo,
	DsnStoreFromRepo,
	GlobalSettingsStoreFromRepo,
	SnapshotProviderFromRepo,
	PolicyHistoryFromRepo,
	MailClassStoreFromRepo,
	VmtaGroupStoreFromRepo,
	KumoReloaderFromClient,
	service.NewAuthenticationService,
	service.NewAuthenticationGRPC,
	service.NewUserService,
	service.NewAuditService,
	service.NewQueueService,
	service.NewSuppressionService,
	service.NewVirtualMtaService,
	service.NewRoutingService,
	NewDkimServiceProvider,
	NewPolicyServiceProvider,
	service.NewFeedbackService,
	service.NewLogService,
	service.NewDsnService,
	service.NewGlobalSettingsService,
	service.NewMailClassService,
	service.NewVmtaGroupService,
)

// AuthStoreFromUserRepo binds *data.UserRepo to the service.UserStore
// interface (auth read path).
func AuthStoreFromUserRepo(r *data.UserRepo) service.UserStore { return r }

// UserAdminStoreFromUserRepo binds *data.UserRepo to service.UserAdminStore
// (admin CRUD path). Same concrete type, two interfaces — the auth and
// admin slices use disjoint method subsets.
func UserAdminStoreFromUserRepo(r *data.UserRepo) service.UserAdminStore { return r }

// AuditStoreFromAuditReader binds the read-side data type to the service
// interface used by AuditService.
func AuditStoreFromAuditReader(r *data.AuditReader) service.AuditStore { return r }

// SuppressionStoreFromRepo binds the suppression repo to the service iface.
func SuppressionStoreFromRepo(r *data.SuppressionRepo) service.SuppressionStore { return r }

// VmtaStoreFromRepo binds the vmta repo to the service iface.
func VmtaStoreFromRepo(r *data.VmtaRepo) service.VirtualMtaStore { return r }

// RoutingStoreFromRepo binds the routing repo to the service iface.
func RoutingStoreFromRepo(r *data.RoutingRepo) service.RoutingStore { return r }

// DkimStoreFromRepo binds the dkim repo to the service iface.
func DkimStoreFromRepo(r *data.DkimRepo) service.DkimStore { return r }

// FeedbackStoreFromRepo binds the feedback repo to the service iface.
func FeedbackStoreFromRepo(r *data.FeedbackRepo) service.FeedbackStore { return r }

// LogStoreFromRepo binds the log repo to the service iface.
func LogStoreFromRepo(r *data.LogRepo) service.LogStore { return r }

// DsnStoreFromRepo binds the dsn repo to the service iface.
func DsnStoreFromRepo(r *data.DsnRepo) service.DsnStore { return r }

// GlobalSettingsStoreFromRepo binds the global-settings repo to the
// service iface.
func GlobalSettingsStoreFromRepo(r *data.GlobalSettingsRepo) service.GlobalSettingsStore {
	return r
}

// SnapshotProviderFromRepo binds the snapshot repo to the service iface.
func SnapshotProviderFromRepo(r *data.SnapshotRepo) service.SnapshotProvider { return r }

// PolicyHistoryFromRepo binds the policy history repo to the service iface.
func PolicyHistoryFromRepo(r *data.PolicyHistoryRepo) service.PolicyHistoryWriter { return r }

// MailClassStoreFromRepo binds the mail-class repo to the service iface.
func MailClassStoreFromRepo(r *data.MailClassRepo) service.MailClassStore { return r }

// VmtaGroupStoreFromRepo binds the vmta-group repo to the service iface.
func VmtaGroupStoreFromRepo(r *data.VmtaGroupRepo) service.VmtaGroupStore { return r }

// KumoReloaderFromClient binds the kumomta HTTP client to the KumoReloader
// interface (Reload is the only method PolicyService needs).
func KumoReloaderFromClient(c *kumomta.Client) service.KumoReloader { return c }

// NewBcryptCost reports the bcrypt cost used by UserService. Kept as a
// provider so the value can be tuned via env in a follow-up without touching
// wire.go. Default is the package minimum (currently 10).
func NewBcryptCost() service.BcryptCost { return service.BcryptCost(0) }

// NewKumomtaClient builds the HTTP client to the kumomta admin API. The
// endpoint is configured via IRIS_KUMO_API_ENDPOINT (matching the env
// shape used in docker-compose). If unset it defaults to http://kumomta:8000
// which is the in-cluster service name.
func NewKumomtaClient(_ *bootstrap.Context, m *metrics.Metrics) (*kumomta.Client, error) {
	endpoint := strings.TrimSpace(getenv("IRIS_KUMO_API_ENDPOINT"))
	if endpoint == "" {
		endpoint = "http://kumomta:8000"
	}
	token := strings.TrimSpace(getenv("IRIS_KUMO_API_TOKEN"))
	c, err := kumomta.NewClient(kumomta.Config{
		BaseURL:       endpoint,
		BearerToken:   token,
		Timeout:       10 * time.Second,
		AllowInsecure: true, // kumomta is reachable over plain http inside the cluster
	})
	if err != nil {
		return nil, err
	}
	if m != nil {
		c.SetMetrics(kumomtaMetricsAdapter{m: m})
	}
	return c, nil
}

// kumomtaMetricsAdapter implements kumomta.ClientMetrics on top of
// the shared *metrics.Metrics. Same indirection pattern as the
// PolicyMetrics adapter — the kumomta package stays Prometheus-free.
type kumomtaMetricsAdapter struct{ m *metrics.Metrics }

func (a kumomtaMetricsAdapter) ObserveRequest(endpoint, method, result string, duration time.Duration) {
	a.m.KumomtaRequestDuration.WithLabelValues(endpoint, method, result).Observe(duration.Seconds())
}

// NewMetricSnapshotWriter currently returns a nil writer — TimescaleDB
// snapshot persistence is a follow-up. The QueueService accepts nil and
// simply skips persistence in that case, so wire is happy with a typed nil.
func NewMetricSnapshotWriter() service.MetricSnapshotWriter { return nil }

// PolicyDir is a wire-friendly named string for the policy output dir.
type PolicyDir string

// DkimKeysDir is a wire-friendly named string for the DKIM key dir.
type DkimKeysDir string

// NewPolicyDir reads IRIS_KUMO_POLICY_DIR (set in docker-compose) and
// falls back to the canonical kumomta policy dir.
func NewPolicyDir() PolicyDir {
	v := strings.TrimSpace(getenv("IRIS_KUMO_POLICY_DIR"))
	if v == "" {
		v = "/opt/kumomta/etc/policy"
	}
	return PolicyDir(v)
}

// NewDkimKeysDir reads IRIS_DKIM_KEYS_DIR or defaults to the conventional
// kumomta key directory.
func NewDkimKeysDir() DkimKeysDir {
	v := strings.TrimSpace(getenv("IRIS_DKIM_KEYS_DIR"))
	if v == "" {
		v = "/opt/kumomta/etc/dkim"
	}
	return DkimKeysDir(v)
}

// NewDkimKeyGenerator returns the production crypto-rand-backed generator.
// Tests inject their own KeyGenerator directly into the service.
func NewDkimKeyGenerator() service.KeyGenerator { return service.DefaultKeyGenerator{} }

// NewSuppressionIndex builds the hot-path suppression cache. Reuses
// IRIS_LOGSTREAM_REDIS_URL — admin-service already requires Redis for
// the log stream in any non-trivial deployment, and a single env var
// keeps the operational story simple. Falls back to a no-op index when
// Redis isn't configured (dev / single-node), so the rest of the boot
// graph doesn't have to special-case the absence.
//
// A construction error here only logs: the suppression list is one of
// many features and a misconfigured Redis URL shouldn't gate the
// admin-service from booting. The Noop index is the conservative
// fallback (kumomta sees an "empty" list and lets messages through —
// blocking legitimate mail at boot is far worse than missing a few
// suppressions until ops fixes the Redis URL).
func NewSuppressionIndex(m *metrics.Metrics) suppressionindex.Index {
	url := strings.TrimSpace(getenv("IRIS_LOGSTREAM_REDIS_URL"))
	if url == "" {
		return suppressionindex.NewNoop()
	}
	idx, err := suppressionindex.NewRedis(url)
	if err != nil {
		log.Printf("suppressionindex: NewRedis(%q) failed, falling back to noop: %v", url, err)
		return suppressionindex.NewNoop()
	}
	return idx.WithMetrics(m)
}

// NewDkimServiceProvider constructs the DkimService with the keys dir; the
// conversion from named string is required because service.NewDkimService
// returns (svc, error) and wire needs an explicit constructor.
func NewDkimServiceProvider(store service.DkimStore, gen service.KeyGenerator, dir DkimKeysDir) (*service.DkimService, error) {
	return service.NewDkimService(store, gen, string(dir))
}

// NewPolicyServiceProvider constructs the PolicyService and attaches
// the metrics sink. The adapter satisfies service.PolicyMetrics
// without leaking the prometheus-client types into the service layer.
func NewPolicyServiceProvider(p service.SnapshotProvider, h service.PolicyHistoryWriter, r service.KumoReloader, dir PolicyDir, m *metrics.Metrics) (*service.PolicyService, error) {
	svc, err := service.NewPolicyService(p, h, r, string(dir))
	if err != nil {
		return nil, err
	}
	if m != nil {
		svc.SetMetrics(policyMetricsAdapter{m: m})
	}
	return svc, nil
}

// policyMetricsAdapter implements service.PolicyMetrics on top of
// the shared *metrics.Metrics. Tiny, but the indirection keeps
// service/ free of a Prometheus dependency.
type policyMetricsAdapter struct{ m *metrics.Metrics }

func (a policyMetricsAdapter) RecordApply(result string) {
	a.m.PolicyApplyTotal.WithLabelValues(result).Inc()
}

// NewJWTIssuer reads JWT secrets from env (IRIS_AUTH_ACCESS_SECRET,
// IRIS_AUTH_REFRESH_SECRET) — set in the docker-compose service
// definition — and falls back to the defaults baked into the issuer.
//
// The issuer requires ≥32-byte secrets; mis-configuration fails fast.
func NewJWTIssuer(_ *bootstrap.Context) (*appjwt.Issuer, error) {
	access := strings.TrimSpace(getenv("IRIS_AUTH_ACCESS_SECRET"))
	refresh := strings.TrimSpace(getenv("IRIS_AUTH_REFRESH_SECRET"))
	if access == "" || refresh == "" {
		return nil, errors.New("auth: IRIS_AUTH_ACCESS_SECRET and IRIS_AUTH_REFRESH_SECRET must be set")
	}
	return appjwt.NewIssuer(appjwt.Config{
		AccessSecret:  []byte(access),
		RefreshSecret: []byte(refresh),
		AccessTTL:     time.Hour,
		RefreshTTL:    7 * 24 * time.Hour,
		Issuer:        "iris",
		Audience:      []string{"kumo-ui-admin"},
		KeyID:         "k1",
	})
}

// getenv is split out so future enhancements (secret manager lookup,
// fallback to file) have a single seam to replace.
func getenv(key string) string {
	v, _ := osLookupEnv(key)
	return v
}

// NewPromQueryAPI returns the API the dashboard service queries.
// IRIS_PROMETHEUS_URL unset → Noop (every endpoint returns 503), so a
// fresh deploy with no Prometheus configured boots cleanly and the
// /analytics page degrades to "metrics not configured" rather than
// failing to load.
func NewPromQueryAPI() promquery.API {
	url := strings.TrimSpace(getenv("IRIS_PROMETHEUS_URL"))
	if url == "" {
		log.Printf("promquery: IRIS_PROMETHEUS_URL unset — dashboard endpoints will return 503")
		return promquery.Noop{}
	}
	c, err := promquery.New(url)
	if err != nil {
		log.Printf("promquery: NewClient(%q) failed, falling back to noop: %v", url, err)
		return promquery.Noop{}
	}
	return c
}

// NewDashboardServiceProvider thin wrapper so wire treats the
// constructor as a provider. service.NewDashboardService is the real
// constructor — the wrapper exists because wire prefers a single
// import path per provider.
func NewDashboardServiceProvider(q promquery.API) *service.DashboardService {
	return service.NewDashboardService(q)
}

// NewMetrics builds the shared *metrics.Metrics passed into the
// log-stream consumer (for per-event-type counters) and the metrics
// HTTP server (for /metrics exposition). Build info comes from
// IRIS_BUILD_VERSION, set by the linker via -X main.version at build
// time and forwarded by main.go via this env var so we don't have to
// thread main.version through the wire graph.
func NewMetrics() *metrics.Metrics {
	return metrics.New(metrics.Build{
		Version:   strings.TrimSpace(getenv("IRIS_BUILD_VERSION")),
		GoVersion: strings.TrimSpace(getenv("IRIS_GO_VERSION")),
	})
}
