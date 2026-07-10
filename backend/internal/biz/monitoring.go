package biz

import (
	"context"
	"net/mail"
	"strings"
	"time"
)

// Monitoring protocols.
const (
	MonitorProtocolIMAP = "imap"
	MonitorProtocolPOP3 = "pop3"
)

// Probe send-status values (KumoMTA delivery outcome).
const (
	ProbeSendQueued   = "queued"
	ProbeSendSent     = "sent"
	ProbeSendDeferred = "deferred"
	ProbeSendBounced  = "bounced"
	ProbeSendError    = "error"
)

// Probe mailbox-status values (phase 2 fetch outcome).
const (
	ProbeMailboxPending  = "pending"
	ProbeMailboxFound    = "found"
	ProbeMailboxNotFound = "not_found"
	ProbeMailboxTimeout  = "timeout"
	ProbeMailboxSkipped  = "skipped"
)

// Probe placement values (which mailbox area the probe landed in). Phase 2 sets
// this from the IMAP folder; phase 3 refines it via header analysis.
const (
	PlacementInbox   = "inbox"
	PlacementSpam    = "spam"
	PlacementMissing = "missing"
	PlacementUnknown = "unknown"
)

// Probe event phases + levels for the per-probe detail log.
const (
	ProbePhaseSend    = "send"
	ProbePhaseFetch   = "fetch"
	ProbePhaseAnalyze = "analyze"

	ProbeEventInfo  = "info"
	ProbeEventError = "error"
)

// ProbeEvent is one timestamped entry in a probe's lifecycle log.
type ProbeEvent struct {
	ID      string
	ProbeID string
	At      time.Time
	Phase   string
	Level   string
	Message string
}

// MailboxProbeResult is the outcome of searching a mailbox for a probe.
type MailboxProbeResult struct {
	Found bool
	// Folder is the IMAP folder the probe was found in (empty for POP3, which has
	// no folders). Drives the initial placement classification.
	Folder     string
	RawHeaders string
	// RawMessage is the full fetched message (headers + body), stored for manual
	// analysis / .eml download.
	RawMessage string
}

// MailboxFetcher connects to a monitored mailbox and searches it for a probe by
// its unique id. Implemented by internal/mailbox for IMAP and POP3.
type MailboxFetcher interface {
	Fetch(ctx context.Context, acc *MonitoringAccount, password, probeUID string) (MailboxProbeResult, error)
	// Verify connects + authenticates (no search) to check the account's
	// parameters and credentials. Returns nil on success.
	Verify(ctx context.Context, acc *MonitoringAccount, password string) error
}

// MonitoringPolicy is the operator-tunable inbox-monitoring policy read at
// runtime from global settings. Durations are 0 when unset; the monitoring
// usecase applies its built-in defaults.
type MonitoringPolicy struct {
	// From is the fallback probe sender for accounts with no from_address.
	From              string
	ReconcileLookback time.Duration
	FetchTimeout      time.Duration
	FetchGiveUp       time.Duration
}

// MonitoringSettingsProvider supplies the live monitoring policy. Satisfied by
// GlobalSettingsUsecase; nil in the usecase means "use built-in defaults".
type MonitoringSettingsProvider interface {
	MonitoringPolicyNow(ctx context.Context) MonitoringPolicy
}

// ProbeFetchCandidate pairs a probe due for a mailbox fetch with the account
// that owns it (connection details + folders), so the fetch worker has
// everything except the decrypted password (fetched separately).
type ProbeFetchCandidate struct {
	Probe   *MonitoringProbe
	Account *MonitoringAccount
}

// MonitoringAccount is a mailbox iris sends probe mail to and later inspects for
// inbox placement. Password is write-only: it is set via Password (encrypted at
// the repo boundary) and never read back.
type MonitoringAccount struct {
	ID       string
	Label    string
	Provider string // gmail|outlook|yahoo|custom
	Email    string // probe recipient
	Protocol string // imap|pop3
	Host     string
	Port     int
	TLS      bool
	Username string
	// Password is the plaintext password on write only; it is never populated on
	// reads (the stored value is encrypted).
	Password     string
	CheckFolders []string
	// FromAddress is the sender iris uses for this account's probes. Must be a
	// domain iris can send/DKIM-sign from.
	FromAddress string
	// Recurring schedule.
	ScheduleEnabled  bool
	ScheduleInterval string // duration form, e.g. "6h"
	FetchDelay       string // duration form, e.g. "10m"
	Enabled          bool
	// HasPassword reports whether an encrypted mailbox password is stored. Set on
	// reads; the password value itself is never returned.
	HasPassword bool
	LastProbeAt *time.Time
	// Last-probe summary — derived (read-only), populated by ListAccounts from the
	// account's most recent probe; empty when there are no probes yet.
	LastProbeSendStatus    string
	LastProbeMailboxStatus string
	LastProbePlacement     string
	// LastProbeVerdict is the phase-3 spam-risk verdict (clean|suspicious|spam)
	// from the latest probe's analysis JSON; empty when not yet analyzed.
	LastProbeVerdict string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// MonitoringProbe is one probe message sent to a MonitoringAccount.
type MonitoringProbe struct {
	ID            string
	AccountID     string
	ProbeUID      string
	MessageID     string
	Subject       string
	FromAddr      string
	Recipient     string
	SentAt        time.Time
	SendStatus    string
	MailboxStatus string
	Placement     string
	FoundAt       *time.Time
	LatencyMs     *int64
	Analysis      string // JSON
	RawHeaders    string
	Error         string
	// FetchAttempts / NextFetchAt drive per-probe fetch backoff. Internal
	// (populated only for the fetch worker); not returned by list/get.
	FetchAttempts int
	NextFetchAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// MonitoringRepo is the persistence boundary for monitoring accounts + probes.
type MonitoringRepo interface {
	ListAccounts(ctx context.Context) ([]*MonitoringAccount, error)
	CreateAccount(ctx context.Context, a *MonitoringAccount) (*MonitoringAccount, error)
	UpdateAccount(ctx context.Context, a *MonitoringAccount) (*MonitoringAccount, error)
	// SetAccountPassword stores a new encrypted password for the account.
	SetAccountPassword(ctx context.Context, id, password string) error
	DeleteAccount(ctx context.Context, id string) error
	GetAccount(ctx context.Context, id string) (*MonitoringAccount, error)
	// AccountSecret returns the decrypted password for an account (fetch worker).
	AccountSecret(ctx context.Context, id string) (string, error)
	// ScheduledAccounts returns enabled accounts whose recurring schedule is due.
	ScheduledAccounts(ctx context.Context, now time.Time) ([]*MonitoringAccount, error)
	TouchLastProbe(ctx context.Context, id string, at time.Time) error

	ListProbes(ctx context.Context, accountID string, page Page) ([]*MonitoringProbe, error)
	CreateProbe(ctx context.Context, p *MonitoringProbe) (*MonitoringProbe, error)
	GetProbe(ctx context.Context, id string) (*MonitoringProbe, error)
	// UpdateProbeSend records the KumoMTA send outcome and (when discovered) the
	// KumoMTA message id. Called by the send-status reconciler.
	UpdateProbeSend(ctx context.Context, id, status, messageID string) error
	// ProbesAwaitingSend returns recently-sent probes still in a non-terminal
	// send state, for the reconciler to correlate against mail records.
	ProbesAwaitingSend(ctx context.Context, since time.Time) ([]*MonitoringProbe, error)
	// CorrelateSend finds the KumoMTA send outcome for a probe by matching its
	// uid-tagged From header and recipient against mail records since sentAt.
	// Found is false when no matching record exists yet.
	CorrelateSend(ctx context.Context, fromAddr, recipient string, sentAt time.Time) (ProbeSendMatch, error)
	// ProbesAwaitingFetch returns probes whose mailbox has not yet been checked
	// and whose fetch delay has elapsed, paired with their account.
	ProbesAwaitingFetch(ctx context.Context, now time.Time) ([]*ProbeFetchCandidate, error)
	// UpdateProbeMailbox records the phase-2 mailbox fetch outcome.
	UpdateProbeMailbox(ctx context.Context, id string, u ProbeMailboxUpdate) error
	// ScheduleNextFetch records a failed/not-found fetch attempt: bumps the
	// attempt count, sets when the next attempt is eligible (backoff), and stores
	// the last error, keeping the probe pending.
	ScheduleNextFetch(ctx context.Context, id string, attempts int, nextAt time.Time, errMsg string) error
	// ProbeRaw returns the stored raw headers + full message for a probe (loaded
	// on demand — the raw message is excluded from the probe list).
	ProbeRaw(ctx context.Context, id string) (*ProbeRawMessage, error)
	// AppendProbeEvent records one lifecycle event for a probe (best-effort log).
	AppendProbeEvent(ctx context.Context, probeID, phase, level, message string) error
	// ListProbeEvents returns a probe's lifecycle events, oldest first.
	ListProbeEvents(ctx context.Context, probeID string) ([]*ProbeEvent, error)
}

// ProbeRawMessage is a probe's stored raw content for manual analysis / download.
type ProbeRawMessage struct {
	ID         string
	ProbeUID   string
	Subject    string
	Recipient  string
	RawHeaders string
	RawMessage string
}

// ProbeMailboxUpdate carries the phase-2 fetch outcome for a probe (and the
// phase-3 header analysis, when produced in the same pass).
type ProbeMailboxUpdate struct {
	MailboxStatus string
	Placement     string
	FoundAt       *time.Time
	LatencyMs     *int64
	RawHeaders    string
	// RawMessage is the full fetched message; empty leaves the stored value.
	RawMessage string
	// Analysis is the phase-3 ProbeAnalysis serialized to JSON; empty leaves the
	// stored analysis untouched.
	Analysis string
	Error    string
}

// ProbeSendMatch is the correlated send outcome for a probe.
type ProbeSendMatch struct {
	Found     bool
	Status    string // one of the ProbeSend* values
	MessageID string
}

// ValidateForConnection normalizes and checks only the fields needed to open a
// mailbox connection (protocol, host, port, and a login username), for the
// "Test connection" action. It deliberately does NOT require label, a valid
// email, from_address, or schedule — those are set when the account is saved.
func (a *MonitoringAccount) ValidateForConnection() error {
	a.Email = strings.ToLower(strings.TrimSpace(a.Email))
	a.Protocol = strings.ToLower(strings.TrimSpace(a.Protocol))
	a.Host = strings.TrimSpace(a.Host)
	a.Username = strings.TrimSpace(a.Username)

	if a.Protocol == "" {
		a.Protocol = MonitorProtocolIMAP
	}
	if a.Protocol != MonitorProtocolIMAP && a.Protocol != MonitorProtocolPOP3 {
		return Invalid("MONITOR_PROTOCOL_INVALID", "protocol must be imap or pop3")
	}
	if a.Host == "" {
		return Invalid("MONITOR_HOST_REQUIRED", "host is required")
	}
	if a.Port <= 0 || a.Port > 65535 {
		return Invalid("MONITOR_PORT_RANGE", "port must be between 1 and 65535")
	}
	if a.Username == "" {
		a.Username = a.Email
	}
	if a.Username == "" {
		return Invalid("MONITOR_USERNAME_REQUIRED", "username or mailbox address is required")
	}
	return nil
}

// Validate normalizes and checks a monitoring account.
func (a *MonitoringAccount) Validate() error {
	a.Label = strings.TrimSpace(a.Label)
	a.Email = strings.ToLower(strings.TrimSpace(a.Email))
	a.Provider = strings.ToLower(strings.TrimSpace(a.Provider))
	a.Protocol = strings.ToLower(strings.TrimSpace(a.Protocol))
	a.Host = strings.TrimSpace(a.Host)
	a.Username = strings.TrimSpace(a.Username)
	a.FromAddress = strings.ToLower(strings.TrimSpace(a.FromAddress))
	a.ScheduleInterval = strings.TrimSpace(a.ScheduleInterval)
	a.FetchDelay = strings.TrimSpace(a.FetchDelay)

	if a.Label == "" {
		return Invalid("MONITOR_LABEL_REQUIRED", "label is required")
	}
	if _, err := mail.ParseAddress(a.Email); err != nil {
		return Invalid("MONITOR_EMAIL_INVALID", "email %q is not a valid address", a.Email)
	}
	if a.Provider == "" {
		a.Provider = "custom"
	}
	if a.Protocol == "" {
		a.Protocol = MonitorProtocolIMAP
	}
	if a.Protocol != MonitorProtocolIMAP && a.Protocol != MonitorProtocolPOP3 {
		return Invalid("MONITOR_PROTOCOL_INVALID", "protocol must be imap or pop3")
	}
	if a.Host == "" {
		return Invalid("MONITOR_HOST_REQUIRED", "host is required")
	}
	if a.Port <= 0 || a.Port > 65535 {
		return Invalid("MONITOR_PORT_RANGE", "port must be between 1 and 65535")
	}
	if a.Username == "" {
		a.Username = a.Email
	}
	if len(a.CheckFolders) == 0 {
		a.CheckFolders = []string{"INBOX"}
	}
	if a.FromAddress != "" {
		if _, err := mail.ParseAddress(a.FromAddress); err != nil {
			return Invalid("MONITOR_FROM_INVALID", "from_address %q is not a valid address", a.FromAddress)
		}
	}
	if a.ScheduleEnabled {
		if _, ok := ParseFlexDuration(a.ScheduleInterval); !ok {
			return Invalid("MONITOR_SCHEDULE_INVALID", "schedule_interval %q is not a valid duration", a.ScheduleInterval)
		}
	}
	if a.FetchDelay != "" {
		if _, ok := ParseFlexDuration(a.FetchDelay); !ok {
			return Invalid("MONITOR_FETCH_DELAY_INVALID", "fetch_delay %q is not a valid duration", a.FetchDelay)
		}
	}
	return nil
}
