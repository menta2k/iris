// Package providers exports the data layer's wire ProviderSet.
package providers

import (
	"github.com/google/wire"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data"
	"github.com/menta2k/iris/backend/pkg/dsnstream"
	"github.com/menta2k/iris/backend/pkg/logstream"
)

// ProviderSet supplies the ent client + repositories + the audit writer
// pipeline. wire threads NewEntClient's and NewAuditWriterDefault's cleanup
// funcs into the app's shutdown sequence automatically — the writer's
// cleanup drains the queue before exit so in-flight entries aren't lost.
var ProviderSet = wire.NewSet(
	data.NewEntClient,
	data.NewUserRepo,
	data.NewAuditReader,
	data.NewAuditEntPersister,
	data.NewAuditWriterDefault,
	data.NewSuppressionRepo,
	data.NewVmtaRepo,
	data.NewRoutingRepo,
	data.NewDkimRepo,
	data.NewFeedbackRepo,
	data.NewLogRepo,
	data.NewDsnRepo,
	data.NewGlobalSettingsRepo,
	data.NewSnapshotRepo,
	data.NewPolicyHistoryRepo,
	data.NewMailClassRepo,
	data.NewVmtaGroupRepo,
	data.NewListenerRepo,
	data.NewAcmeAccountRepo,
	data.NewAcmeCertificateRepo,
	data.NewAcmeDnsProviderConfigRepo,
	data.NewLoginPolicyRepo,
	data.NewLogstreamPersister,
	data.NewDsnstreamPersister,
	AuditPersisterFromEnt,
	LogstreamPersisterIface,
	DsnstreamPersisterIface,
)

// LogstreamPersisterIface binds *data.LogstreamPersister to the
// logstream.Persister interface so wire can satisfy NewLogstreamServer.
func LogstreamPersisterIface(p *data.LogstreamPersister) logstream.Persister { return p }

// DsnstreamPersisterIface binds *data.DsnstreamPersister to the
// dsnstream.Persister interface so wire can satisfy NewDsnstreamServer.
func DsnstreamPersisterIface(p *data.DsnstreamPersister) dsnstream.Persister { return p }

// AuditPersisterFromEnt binds *data.AuditEntPersister to the data.AuditPersister
// interface so wire can satisfy NewAuditWriterDefault's dependency without a
// `wire.Bind` (which would require a type literal at the wire-set level).
func AuditPersisterFromEnt(p *data.AuditEntPersister) data.AuditPersister { return p }
