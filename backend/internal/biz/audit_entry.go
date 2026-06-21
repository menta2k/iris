package biz

// AuditEntry is a stored audit record returned to security administrators.
type AuditEntry struct {
	ID                string
	OccurredAt        string
	ActorUserID       string
	Operation         string
	TargetType        string
	TargetID          string
	Outcome           string
	IPAddress         string
	RequestID         string
	SafeChangeSummary map[string]any
}

// SafeSummary returns a copy of the change summary with any sensitive keys
// redacted, providing defense-in-depth even if a caller persisted raw data.
func (e *AuditEntry) SafeSummary() map[string]any {
	return RedactMap(e.SafeChangeSummary)
}
