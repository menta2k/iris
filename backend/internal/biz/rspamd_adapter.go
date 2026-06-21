package biz

import "context"

// RspamdScan is a filter decision for a message.
type RspamdScan struct {
	Action  string
	Score   float64
	Symbols []string
	Reason  string
}

// RspamdAdapter isolates interaction with the Rspamd filtering service.
type RspamdAdapter interface {
	// Scan submits raw message bytes for filtering. Implementations must apply
	// bounded timeouts and never log raw message content.
	Scan(ctx context.Context, raw []byte) (RspamdScan, error)
}

// stubRspamd returns a deterministic benign result for development and tests.
type stubRspamd struct{}

// NewStubRspamd returns an in-memory Rspamd adapter.
func NewStubRspamd() RspamdAdapter { return stubRspamd{} }

func (stubRspamd) Scan(context.Context, []byte) (RspamdScan, error) {
	return RspamdScan{Action: "no action", Score: 0, Symbols: nil, Reason: "stub"}, nil
}
