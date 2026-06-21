package biz

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ServiceOperation is a KumoMTA service-control verb.
type ServiceOperation string

const (
	ServiceRestart ServiceOperation = "restart"
	ServiceReload  ServiceOperation = "reload"
	ServiceStop    ServiceOperation = "stop"
	ServiceStart   ServiceOperation = "start"
)

// ValidServiceOperation reports whether op is a known service operation.
func ValidServiceOperation(op string) bool {
	switch ServiceOperation(op) {
	case ServiceRestart, ServiceReload, ServiceStop, ServiceStart:
		return true
	default:
		return false
	}
}

// QueueAction is a queue-control verb applied to a mailclass.
type QueueAction string

const (
	QueuePause  QueueAction = "pause"
	QueueResume QueueAction = "resume"
	QueueDrain  QueueAction = "drain"
	QueueFlush  QueueAction = "flush"
)

// ValidQueueAction reports whether action is a known queue action.
func ValidQueueAction(action string) bool {
	switch QueueAction(action) {
	case QueuePause, QueueResume, QueueDrain, QueueFlush:
		return true
	default:
		return false
	}
}

// KumoStatus is a snapshot of KumoMTA service state.
type KumoStatus struct {
	State     string
	CheckedAt time.Time
}

// KumoMTAAdapter isolates all interaction with the KumoMTA service. Every method
// must honor context cancellation and bounded timeouts.
type KumoMTAAdapter interface {
	// Status returns the current service state.
	Status(ctx context.Context) (KumoStatus, error)
	// ApplyServiceControl performs a serialized service-control operation.
	ApplyServiceControl(ctx context.Context, op ServiceOperation) (string, error)
	// ApplyQueueAction performs a queue action for a mailclass.
	ApplyQueueAction(ctx context.Context, mailclass string, action QueueAction) (string, error)
	// ApplyConfig writes the rendered KumoMTA policy and activates it. When
	// restart is true the change touches the init block (listeners, spool, log
	// hook), which a hot reload cannot pick up, so the service must be restarted;
	// otherwise a reload (config-epoch bump) suffices. Returns where the config
	// was written and a human-readable result summary.
	ApplyConfig(ctx context.Context, rendered RenderedConfig, restart bool) (appliedPath, summary string, err error)
}

// stubKumoMTA is an in-memory adapter used for local development and tests. It
// records the most recently applied config so it can be inspected.
type stubKumoMTA struct {
	mu            sync.Mutex
	state         string
	lastConfig    string
	lastChecksum  string
	configApplies int
}

// NewStubKumoMTA returns a deterministic in-memory KumoMTA adapter.
func NewStubKumoMTA() KumoMTAAdapter {
	return &stubKumoMTA{state: "running"}
}

func (s *stubKumoMTA) ApplyConfig(_ context.Context, rendered RenderedConfig, restart bool) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastConfig = rendered.Content
	s.lastChecksum = rendered.Checksum
	s.configApplies++
	action := "reloaded"
	if restart {
		action = "restarted"
	}
	return "memory://kumomta/policy.lua", fmt.Sprintf(
		"applied in-memory (%s): %d sources, %d pools, %d routes, %d dkim, %d suppressions",
		action, rendered.VMTACount, rendered.PoolCount, rendered.RouteCount, rendered.DKIMCount, rendered.SuppressionCount), nil
}

func (s *stubKumoMTA) Status(context.Context) (KumoStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return KumoStatus{State: s.state}, nil
}

func (s *stubKumoMTA) ApplyServiceControl(_ context.Context, op ServiceOperation) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch op {
	case ServiceStop:
		s.state = "stopped"
	default:
		s.state = "running"
	}
	return "ok: " + string(op), nil
}

func (s *stubKumoMTA) ApplyQueueAction(_ context.Context, mailclass string, action QueueAction) (string, error) {
	return "ok: " + string(action) + " " + mailclass, nil
}
