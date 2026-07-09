package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ProbeUIDHeader carries the unique probe identifier on the outbound message so
// the phase-2 mailbox fetch can locate the exact probe in the inbox.
const ProbeUIDHeader = "X-Iris-Probe-Id"

// probeSubjectPrefix labels probe messages and embeds the uid so the mailbox
// search can match on subject when the header is stripped by a provider.
const probeSubjectPrefix = "[iris-probe]"

// MonitoringClock returns the current time; overridable in tests.
type MonitoringClock func() time.Time

// MonitoringUsecase is the operator-facing API for inbox-placement monitoring:
// mailbox account CRUD and probe sending. Reads require monitoring:read,
// mutations monitoring:write. Mailbox passwords are write-only (never returned).
type MonitoringUsecase struct {
	repo     MonitoringRepo
	injector KumoInjector
	auditor  *Auditor
	now MonitoringClock
	// fetcher performs the phase-2 mailbox search; nil disables mailbox fetching.
	fetcher MailboxFetcher
	// settings supplies the live monitoring policy (fallback sender + tuning
	// durations); nil uses built-in defaults.
	settings MonitoringSettingsProvider
	// analyzer performs the phase-3 LLM header analysis; nil falls back to the
	// deterministic heuristic verdict.
	analyzer ProbeHeaderAnalyzer
}

// Built-in defaults applied when a monitoring policy value is unset.
const (
	defaultReconcileLookback = time.Hour
	defaultFetchTimeout      = 30 * time.Second
	defaultFetchGiveUp       = 2 * time.Hour
)

// NewMonitoringUsecase constructs the use case.
func NewMonitoringUsecase(repo MonitoringRepo, injector KumoInjector, auditor *Auditor) *MonitoringUsecase {
	return &MonitoringUsecase{
		repo:     repo,
		injector: injector,
		auditor:  auditor,
		now:      time.Now,
	}
}

// WithClock overrides the clock (tests).
func (uc *MonitoringUsecase) WithClock(c MonitoringClock) *MonitoringUsecase {
	uc.now = c
	return uc
}

// WithSettings attaches the live monitoring-policy provider (global settings).
func (uc *MonitoringUsecase) WithSettings(s MonitoringSettingsProvider) *MonitoringUsecase {
	uc.settings = s
	return uc
}

// WithFetcher attaches the phase-2 mailbox fetcher. Without a fetcher, mailbox
// fetching is disabled and probes stay at mailbox_status=pending.
func (uc *MonitoringUsecase) WithFetcher(f MailboxFetcher) *MonitoringUsecase {
	uc.fetcher = f
	return uc
}

// policy resolves the effective monitoring policy, applying built-in defaults to
// any value the settings provider leaves unset.
func (uc *MonitoringUsecase) policy(ctx context.Context) MonitoringPolicy {
	var p MonitoringPolicy
	if uc.settings != nil {
		p = uc.settings.MonitoringPolicyNow(ctx)
	}
	p.From = strings.ToLower(strings.TrimSpace(p.From))
	if p.ReconcileLookback <= 0 {
		p.ReconcileLookback = defaultReconcileLookback
	}
	if p.FetchTimeout <= 0 {
		p.FetchTimeout = defaultFetchTimeout
	}
	if p.FetchGiveUp <= 0 {
		p.FetchGiveUp = defaultFetchGiveUp
	}
	return p
}

// WithAnalyzer attaches the phase-3 LLM header analyzer. Without it, analysis
// falls back to the deterministic heuristic verdict.
func (uc *MonitoringUsecase) WithAnalyzer(a ProbeHeaderAnalyzer) *MonitoringUsecase {
	uc.analyzer = a
	return uc
}

// ListAccounts returns all monitoring accounts (without password material).
func (uc *MonitoringUsecase) ListAccounts(ctx context.Context) ([]*MonitoringAccount, error) {
	if _, err := RequirePermission(ctx, PermMonitoringRead); err != nil {
		return nil, err
	}
	return uc.repo.ListAccounts(ctx)
}

// GetAccount returns one account by id.
func (uc *MonitoringUsecase) GetAccount(ctx context.Context, id string) (*MonitoringAccount, error) {
	if _, err := RequirePermission(ctx, PermMonitoringRead); err != nil {
		return nil, err
	}
	return uc.repo.GetAccount(ctx, id)
}

// CreateAccount validates and inserts an account. The password (if any) is
// encrypted at the repo boundary.
func (uc *MonitoringUsecase) CreateAccount(ctx context.Context, a *MonitoringAccount) (*MonitoringAccount, error) {
	if _, err := RequirePermission(ctx, PermMonitoringWrite); err != nil {
		return nil, err
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateAccount(ctx, a)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "monitoring_account.create", out.ID, map[string]any{"email": out.Email})
	return out, nil
}

// UpdateAccount validates and updates mutable fields (password is set via
// SetAccountPassword). The password field on the input is ignored here.
func (uc *MonitoringUsecase) UpdateAccount(ctx context.Context, a *MonitoringAccount) (*MonitoringAccount, error) {
	if _, err := RequirePermission(ctx, PermMonitoringWrite); err != nil {
		return nil, err
	}
	if a.ID == "" {
		return nil, Invalid("MONITOR_ID_REQUIRED", "id is required")
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateAccount(ctx, a)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "monitoring_account.update", out.ID, map[string]any{"email": out.Email})
	return out, nil
}

// SetAccountPassword rotates the mailbox password.
func (uc *MonitoringUsecase) SetAccountPassword(ctx context.Context, id, password string) error {
	if _, err := RequirePermission(ctx, PermMonitoringWrite); err != nil {
		return err
	}
	if id == "" {
		return Invalid("MONITOR_ID_REQUIRED", "id is required")
	}
	if err := uc.repo.SetAccountPassword(ctx, id, password); err != nil {
		return err
	}
	uc.audit(ctx, "monitoring_account.set_password", id, nil)
	return nil
}

// DeleteAccount removes an account and its probes.
func (uc *MonitoringUsecase) DeleteAccount(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermMonitoringWrite); err != nil {
		return err
	}
	if err := uc.repo.DeleteAccount(ctx, id); err != nil {
		return err
	}
	uc.audit(ctx, "monitoring_account.delete", id, nil)
	return nil
}

// ListProbes returns probes for an account, newest first.
func (uc *MonitoringUsecase) ListProbes(ctx context.Context, accountID string, page Page) ([]*MonitoringProbe, error) {
	if _, err := RequirePermission(ctx, PermMonitoringRead); err != nil {
		return nil, err
	}
	if accountID == "" {
		return nil, Invalid("MONITOR_ACCOUNT_ID_REQUIRED", "account_id is required")
	}
	return uc.repo.ListProbes(ctx, accountID, page)
}

// SendProbe sends a probe message to the account's mailbox via KumoMTA and
// records it. This is both the manual "send now" action and the scheduler's
// per-account trigger (SendProbeForAccount).
func (uc *MonitoringUsecase) SendProbe(ctx context.Context, accountID string) (*MonitoringProbe, error) {
	if _, err := RequirePermission(ctx, PermMonitoringWrite); err != nil {
		return nil, err
	}
	acc, err := uc.repo.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return uc.sendProbe(ctx, acc, true)
}

// SendScheduledProbe sends a probe for an account already resolved by the
// scheduler worker (no per-call permission check — the worker runs as the
// system). Callers must have obtained acc from a trusted source.
func (uc *MonitoringUsecase) SendScheduledProbe(ctx context.Context, acc *MonitoringAccount) (*MonitoringProbe, error) {
	return uc.sendProbe(ctx, acc, false)
}

// sendProbe builds and injects the probe message, then persists it.
func (uc *MonitoringUsecase) sendProbe(ctx context.Context, acc *MonitoringAccount, audit bool) (*MonitoringProbe, error) {
	if !acc.Enabled {
		return nil, Invalid("MONITOR_ACCOUNT_DISABLED", "account %q is disabled", acc.ID)
	}
	base := acc.FromAddress
	if base == "" {
		base = uc.policy(ctx).From
	}
	if base == "" {
		return nil, Invalid("MONITOR_FROM_UNCONFIGURED",
			"account has no from_address and no default monitoring sender is configured (Global Settings → Inbox monitoring)")
	}
	uid, err := newProbeUID()
	if err != nil {
		return nil, Internal(err, "generate probe uid")
	}
	// Tag the From header with the uid so the send record can be correlated even
	// after the envelope sender is VERP-rewritten, and the mailbox fetch can match.
	fromAddr := plusTag(base, uid)
	subject := fmt.Sprintf("%s %s", probeSubjectPrefix, uid)

	req := KumoInjectRequest{
		EnvelopeSender: base,
		Recipients:     []KumoInjectRcpt{{Email: acc.Email}},
		Content: KumoInjectContent{
			Subject: subject,
			TextBody: fmt.Sprintf(
				"This is an automated iris inbox-placement probe.\n\nProbe ID: %s\nSent: %s\n",
				uid, uc.now().UTC().Format(time.RFC3339)),
			From: KumoInjectAddr{Email: fromAddr, Name: "iris monitor"},
			Headers: map[string]string{
				ProbeUIDHeader: uid,
			},
		},
	}
	if err := uc.injector.InjectV1(ctx, req); err != nil {
		// Record the failed attempt so the operator sees it.
		p := &MonitoringProbe{
			AccountID: acc.ID, ProbeUID: uid, Subject: subject, FromAddr: fromAddr,
			Recipient: acc.Email, SendStatus: ProbeSendError, Error: err.Error(),
		}
		if _, cerr := uc.repo.CreateProbe(ctx, p); cerr != nil {
			LoggerFrom(ctx).Error("record failed probe", "error", cerr.Error())
		}
		return nil, err
	}

	probe := &MonitoringProbe{
		AccountID:  acc.ID,
		ProbeUID:   uid,
		Subject:    subject,
		FromAddr:   fromAddr,
		Recipient:  acc.Email,
		SendStatus: ProbeSendQueued,
	}
	out, err := uc.repo.CreateProbe(ctx, probe)
	if err != nil {
		return nil, err
	}
	if err := uc.repo.TouchLastProbe(ctx, acc.ID, uc.now()); err != nil {
		LoggerFrom(ctx).Warn("touch last_probe_at failed", "account", acc.ID, "error", err.Error())
	}
	if audit {
		uc.audit(ctx, "monitoring_probe.send", out.ID, map[string]any{"account": acc.ID, "recipient": acc.Email})
	}
	return out, nil
}

// ReconcileSends correlates recently-sent probes still in the queued state
// against the mail log and advances their send status. It is called by the
// reconciler worker (system context, no RBAC). The lookback window comes from
// the monitoring policy. Returns the number of probes advanced.
func (uc *MonitoringUsecase) ReconcileSends(ctx context.Context) (int, error) {
	since := uc.now().Add(-uc.policy(ctx).ReconcileLookback)
	probes, err := uc.repo.ProbesAwaitingSend(ctx, since)
	if err != nil {
		return 0, err
	}
	advanced := 0
	for _, p := range probes {
		match, err := uc.repo.CorrelateSend(ctx, p.FromAddr, p.Recipient, p.SentAt)
		if err != nil {
			LoggerFrom(ctx).Warn("probe correlate failed", "probe", p.ID, "error", err.Error())
			continue
		}
		if !match.Found || match.Status == ProbeSendQueued {
			continue // no record yet, or still only received — leave queued
		}
		if err := uc.repo.UpdateProbeSend(ctx, p.ID, match.Status, match.MessageID); err != nil {
			LoggerFrom(ctx).Warn("probe send update failed", "probe", p.ID, "error", err.Error())
			continue
		}
		advanced++
	}
	return advanced, nil
}

// RunDueSchedules sends a probe for every enabled account whose recurring
// schedule is due. Called by the scheduler worker (system context, no RBAC).
// Returns the number of probes sent.
func (uc *MonitoringUsecase) RunDueSchedules(ctx context.Context) (int, error) {
	accounts, err := uc.repo.ScheduledAccounts(ctx, uc.now())
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, acc := range accounts {
		if _, err := uc.SendScheduledProbe(ctx, acc); err != nil {
			LoggerFrom(ctx).Warn("scheduled probe failed", "account", acc.ID, "error", err.Error())
			continue
		}
		sent++
	}
	return sent, nil
}

// RunDueFetches performs the phase-2 mailbox search for every probe whose fetch
// delay has elapsed. It sets found/not_found/timeout, an initial folder-based
// placement, latency, and the captured raw headers (for phase-3 analysis).
// Called by the fetch worker (system context, no RBAC). Returns probes advanced.
func (uc *MonitoringUsecase) RunDueFetches(ctx context.Context) (int, error) {
	if uc.fetcher == nil {
		return 0, nil // mailbox fetching disabled (no fetcher wired)
	}
	cands, err := uc.repo.ProbesAwaitingFetch(ctx, uc.now())
	if err != nil {
		return 0, err
	}
	pol := uc.policy(ctx)
	advanced := 0
	for _, c := range cands {
		if uc.fetchOne(ctx, c, pol) {
			advanced++
		}
	}
	return advanced, nil
}

// fetchOne runs one probe's mailbox search and persists the outcome. It returns
// true when the probe reached a terminal mailbox status (found/not_found/
// timeout); a transient failure before give-up leaves it pending for retry.
func (uc *MonitoringUsecase) fetchOne(ctx context.Context, c *ProbeFetchCandidate, pol MonitoringPolicy) bool {
	password, err := uc.repo.AccountSecret(ctx, c.Account.ID)
	if err != nil {
		LoggerFrom(ctx).Warn("probe fetch: secret unavailable", "account", c.Account.ID, "error", err.Error())
		return uc.giveUpOrRetry(ctx, c, pol, ProbeMailboxTimeout, err.Error())
	}
	// Bound the mailbox connection by the configured fetch timeout.
	fctx, cancel := context.WithTimeout(ctx, pol.FetchTimeout)
	defer cancel()
	res, err := uc.fetcher.Fetch(fctx, c.Account, password, c.Probe.ProbeUID)
	if err != nil {
		// Connection/search failure: retry until give-up, then mark timeout.
		return uc.giveUpOrRetry(ctx, c, pol, ProbeMailboxTimeout, err.Error())
	}
	if !res.Found {
		// Not there yet: retry until give-up, then mark not_found/missing.
		return uc.giveUpOrRetry(ctx, c, pol, ProbeMailboxNotFound, "")
	}
	now := uc.now()
	latency := now.Sub(c.Probe.SentAt).Milliseconds()
	update := ProbeMailboxUpdate{
		MailboxStatus: ProbeMailboxFound,
		Placement:     placementFromFolder(res.Folder),
		FoundAt:       &now,
		LatencyMs:     &latency,
		RawHeaders:    res.RawHeaders,
		Analysis:      uc.analyzeHeaders(ctx, res.RawHeaders),
	}
	if err := uc.repo.UpdateProbeMailbox(ctx, c.Probe.ID, update); err != nil {
		LoggerFrom(ctx).Warn("probe mailbox update failed", "probe", c.Probe.ID, "error", err.Error())
		return false
	}
	return true
}

// giveUpOrRetry marks the probe terminal (with the given status + a matching
// placement) once the give-up window has passed; otherwise it leaves the probe
// pending (records only the error) so the next tick retries. Returns true when
// the probe became terminal.
func (uc *MonitoringUsecase) giveUpOrRetry(ctx context.Context, c *ProbeFetchCandidate, pol MonitoringPolicy, terminalStatus, errMsg string) bool {
	elapsed := uc.now().Sub(c.Probe.SentAt)
	if elapsed < pol.FetchGiveUp {
		// Still within the retry window: only surface the last error, stay pending.
		if errMsg != "" {
			_ = uc.repo.UpdateProbeMailbox(ctx, c.Probe.ID, ProbeMailboxUpdate{
				MailboxStatus: ProbeMailboxPending,
				Placement:     c.Probe.Placement,
				Error:         errMsg,
			})
		}
		return false
	}
	placement := PlacementUnknown
	if terminalStatus == ProbeMailboxNotFound {
		placement = PlacementMissing
	}
	if err := uc.repo.UpdateProbeMailbox(ctx, c.Probe.ID, ProbeMailboxUpdate{
		MailboxStatus: terminalStatus,
		Placement:     placement,
		Error:         errMsg,
	}); err != nil {
		LoggerFrom(ctx).Warn("probe mailbox give-up update failed", "probe", c.Probe.ID, "error", err.Error())
		return false
	}
	return true
}

// analyzeHeaders runs the phase-3 deliverability analysis: always a deterministic
// SPF/DKIM/DMARC + spam-signal parse, refined by the LLM verdict when an analyzer
// is configured. Returns the analysis serialized to JSON ("" only on a marshal
// failure, which leaves the stored analysis untouched).
func (uc *MonitoringUsecase) analyzeHeaders(ctx context.Context, rawHeaders string) string {
	analysis := ParseHeaderSignals(rawHeaders)
	if uc.analyzer != nil && rawHeaders != "" {
		if v, err := uc.analyzer.AnalyzeHeaders(ctx, rawHeaders); err != nil {
			LoggerFrom(ctx).Warn("probe header analysis (llm) failed, using heuristic", "error", err.Error())
		} else {
			analysis.Verdict = v.Verdict
			analysis.Confidence = v.Confidence
			analysis.Source = AnalysisSourceLLM
			if strings.TrimSpace(v.Summary) != "" {
				analysis.Summary = strings.TrimSpace(v.Summary)
			}
			if len(v.Factors) > 0 {
				analysis.Factors = v.Factors
			}
		}
	}
	buf, err := json.Marshal(analysis)
	if err != nil {
		LoggerFrom(ctx).Warn("probe analysis marshal failed", "error", err.Error())
		return ""
	}
	return string(buf)
}

// placementFromFolder maps the IMAP folder a probe was found in to a placement.
// POP3 has no folders (empty folder) → treated as inbox (the primary mailbox).
func placementFromFolder(folder string) string {
	f := strings.ToLower(folder)
	switch {
	case strings.Contains(f, "spam"), strings.Contains(f, "junk"):
		return PlacementSpam
	case f == "" || strings.Contains(f, "inbox"):
		return PlacementInbox
	default:
		return PlacementUnknown
	}
}

func (uc *MonitoringUsecase) audit(ctx context.Context, op, id string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, "monitoring", id, AuditSuccess, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}

// newProbeUID returns a URL/subject-safe unique identifier for a probe.
func newProbeUID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ip" + hex.EncodeToString(b), nil
}

// plusTag inserts a plus-address tag into an email local part:
// "box@example.com" + "tag" -> "box+tag@example.com". If the address has no
// "@" it is returned unchanged.
func plusTag(addr, tag string) string {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return addr
	}
	return addr[:at] + "+" + tag + addr[at:]
}
