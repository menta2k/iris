package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/secret"
)

// MonitoringRepo persists inbox-placement monitoring accounts and probes. The
// mailbox password is stored reversibly encrypted (AES-GCM) because the fetch
// worker must present it to the IMAP/POP3 server; the cipher is keyed by
// IRIS_MONITORING_KEY. A nil cipher means encryption is unavailable — writes
// that carry a password are rejected and AccountSecret errors.
type MonitoringRepo struct {
	db     *DB
	cipher *secret.Cipher
}

// NewMonitoringRepo constructs the repository. cipher may be nil when
// IRIS_MONITORING_KEY is unset (password storage disabled).
func NewMonitoringRepo(db *DB, cipher *secret.Cipher) *MonitoringRepo {
	return &MonitoringRepo{db: db, cipher: cipher}
}

var _ biz.MonitoringRepo = (*MonitoringRepo)(nil)

// accountCols excludes password_enc — the plaintext/ciphertext password is never
// returned via the account model, only via AccountSecret.
// accountColList is the account column list; hasPasswordExpr is spliced in so the
// same ordering can be reused with a table alias in JOIN queries.
func accountColList(hasPasswordExpr string) string {
	return `id, label, provider, email, protocol, host, port, tls, username,
	check_folders, from_address, schedule_enabled, schedule_interval, fetch_delay,
	enabled, ` + hasPasswordExpr + ` AS has_password, last_probe_at, created_at, updated_at`
}

var accountCols = accountColList(`(password_enc <> '')`)

// accountScanArgs returns the scan destinations for an account row, in accountCols
// order. Shared by single-row scans and JOIN scans.
func accountScanArgs(a *biz.MonitoringAccount) []any {
	return []any{&a.ID, &a.Label, &a.Provider, &a.Email, &a.Protocol, &a.Host,
		&a.Port, &a.TLS, &a.Username, &a.CheckFolders, &a.FromAddress,
		&a.ScheduleEnabled, &a.ScheduleInterval, &a.FetchDelay, &a.Enabled,
		&a.HasPassword, &a.LastProbeAt, &a.CreatedAt, &a.UpdatedAt}
}

func scanAccount(row pgx.Row) (*biz.MonitoringAccount, error) {
	a := &biz.MonitoringAccount{}
	if err := row.Scan(accountScanArgs(a)...); err != nil {
		return nil, err
	}
	return a, nil
}

// ListAccounts returns all accounts, newest first.
func (r *MonitoringRepo) ListAccounts(ctx context.Context) ([]*biz.MonitoringAccount, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+accountCols+` FROM monitoring_accounts ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list monitoring accounts: %w", err)
	}
	defer rows.Close()
	var out []*biz.MonitoringAccount
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan monitoring account: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAccount returns one account by id, or a NotFound domain error.
func (r *MonitoringRepo) GetAccount(ctx context.Context, id string) (*biz.MonitoringAccount, error) {
	a, err := scanAccount(r.db.Pool.QueryRow(ctx,
		`SELECT `+accountCols+` FROM monitoring_accounts WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("MONITOR_ACCOUNT_NOT_FOUND", "monitoring account %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get monitoring account: %w", err)
	}
	return a, nil
}

// CreateAccount inserts an account, encrypting the password if one is supplied.
func (r *MonitoringRepo) CreateAccount(ctx context.Context, a *biz.MonitoringAccount) (*biz.MonitoringAccount, error) {
	enc, err := r.encrypt(a.Password)
	if err != nil {
		return nil, err
	}
	out, err := scanAccount(r.db.Pool.QueryRow(ctx, `
		INSERT INTO monitoring_accounts
			(label, provider, email, protocol, host, port, tls, username, password_enc,
			 check_folders, from_address, schedule_enabled, schedule_interval, fetch_delay, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+accountCols,
		a.Label, a.Provider, a.Email, a.Protocol, a.Host, a.Port, a.TLS, a.Username, enc,
		a.CheckFolders, a.FromAddress, a.ScheduleEnabled, a.ScheduleInterval, a.FetchDelay, a.Enabled))
	if err != nil {
		return nil, fmt.Errorf("create monitoring account: %w", err)
	}
	return out, nil
}

// UpdateAccount edits mutable fields (not the password — see SetAccountPassword).
func (r *MonitoringRepo) UpdateAccount(ctx context.Context, a *biz.MonitoringAccount) (*biz.MonitoringAccount, error) {
	out, err := scanAccount(r.db.Pool.QueryRow(ctx, `
		UPDATE monitoring_accounts SET
			label=$2, provider=$3, email=$4, protocol=$5, host=$6, port=$7, tls=$8,
			username=$9, check_folders=$10, from_address=$11, schedule_enabled=$12,
			schedule_interval=$13, fetch_delay=$14, enabled=$15, updated_at=now()
		WHERE id=$1
		RETURNING `+accountCols,
		a.ID, a.Label, a.Provider, a.Email, a.Protocol, a.Host, a.Port, a.TLS,
		a.Username, a.CheckFolders, a.FromAddress, a.ScheduleEnabled,
		a.ScheduleInterval, a.FetchDelay, a.Enabled))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("MONITOR_ACCOUNT_NOT_FOUND", "monitoring account %q not found", a.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("update monitoring account: %w", err)
	}
	return out, nil
}

// SetAccountPassword stores a new encrypted password.
func (r *MonitoringRepo) SetAccountPassword(ctx context.Context, id, password string) error {
	enc, err := r.encrypt(password)
	if err != nil {
		return err
	}
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE monitoring_accounts SET password_enc=$2, updated_at=now() WHERE id=$1`, id, enc)
	if err != nil {
		return fmt.Errorf("set monitoring password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("MONITOR_ACCOUNT_NOT_FOUND", "monitoring account %q not found", id)
	}
	return nil
}

// DeleteAccount removes an account (probes cascade).
func (r *MonitoringRepo) DeleteAccount(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM monitoring_accounts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete monitoring account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("MONITOR_ACCOUNT_NOT_FOUND", "monitoring account %q not found", id)
	}
	return nil
}

// AccountSecret returns the decrypted mailbox password for an account.
func (r *MonitoringRepo) AccountSecret(ctx context.Context, id string) (string, error) {
	if r.cipher == nil {
		return "", biz.Unavailable("MONITOR_CIPHER_UNSET", "IRIS_MONITORING_KEY is not configured")
	}
	var enc string
	err := r.db.Pool.QueryRow(ctx,
		`SELECT password_enc FROM monitoring_accounts WHERE id = $1`, id).Scan(&enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", biz.NotFound("MONITOR_ACCOUNT_NOT_FOUND", "monitoring account %q not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("get monitoring secret: %w", err)
	}
	pw, err := r.cipher.Decrypt(enc)
	if err != nil {
		return "", fmt.Errorf("decrypt monitoring secret: %w", err)
	}
	return pw, nil
}

// ScheduledAccounts returns enabled accounts whose recurring schedule is due at
// now: schedule_enabled and (last_probe_at is null or last_probe_at + interval
// has elapsed). The interval comparison is done in Go (interval is a duration
// string, not an SQL interval), so this fetches candidates and filters.
func (r *MonitoringRepo) ScheduledAccounts(ctx context.Context, now time.Time) ([]*biz.MonitoringAccount, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+accountCols+` FROM monitoring_accounts
		 WHERE enabled = true AND schedule_enabled = true`)
	if err != nil {
		return nil, fmt.Errorf("scheduled monitoring accounts: %w", err)
	}
	defer rows.Close()
	var out []*biz.MonitoringAccount
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan scheduled account: %w", err)
		}
		if dueForProbe(a, now) {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}

// dueForProbe reports whether the account's recurring probe is due.
func dueForProbe(a *biz.MonitoringAccount, now time.Time) bool {
	d, ok := biz.ParseFlexDuration(a.ScheduleInterval)
	if !ok || d <= 0 {
		return false
	}
	if a.LastProbeAt == nil {
		return true
	}
	return !now.Before(a.LastProbeAt.Add(d))
}

// TouchLastProbe records that a probe was just sent for an account.
func (r *MonitoringRepo) TouchLastProbe(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE monitoring_accounts SET last_probe_at = $2, updated_at = now() WHERE id = $1`, id, at)
	if err != nil {
		return fmt.Errorf("touch last_probe_at: %w", err)
	}
	return nil
}

// --- probes ---

const probeColNames = `id, account_id, probe_uid, message_id, subject, from_addr, recipient,
	sent_at, send_status, mailbox_status, placement, found_at, latency_ms, analysis,
	raw_headers, error, created_at, updated_at`

const probeCols = probeColNames

// probeScanArgs returns the scan destinations for a probe row, in probeCols
// order, optionally followed by extra destinations (for JOIN scans).
func probeScanArgs(p *biz.MonitoringProbe, extra ...any) []any {
	args := []any{&p.ID, &p.AccountID, &p.ProbeUID, &p.MessageID, &p.Subject,
		&p.FromAddr, &p.Recipient, &p.SentAt, &p.SendStatus, &p.MailboxStatus,
		&p.Placement, &p.FoundAt, &p.LatencyMs, &p.Analysis, &p.RawHeaders,
		&p.Error, &p.CreatedAt, &p.UpdatedAt}
	return append(args, extra...)
}

func scanProbe(row pgx.Row) (*biz.MonitoringProbe, error) {
	p := &biz.MonitoringProbe{}
	if err := row.Scan(probeScanArgs(p)...); err != nil {
		return nil, err
	}
	return p, nil
}

// Alias-qualified column lists for the ProbesAwaitingFetch JOIN. The order must
// match probeScanArgs / accountScanArgs exactly.
const probeColsP = `p.id, p.account_id, p.probe_uid, p.message_id, p.subject, p.from_addr,
	p.recipient, p.sent_at, p.send_status, p.mailbox_status, p.placement, p.found_at,
	p.latency_ms, p.analysis, p.raw_headers, p.error, p.created_at, p.updated_at`

const accountColsP = `a.id, a.label, a.provider, a.email, a.protocol, a.host, a.port, a.tls,
	a.username, a.check_folders, a.from_address, a.schedule_enabled, a.schedule_interval,
	a.fetch_delay, a.enabled, (a.password_enc <> '') AS has_password, a.last_probe_at,
	a.created_at, a.updated_at`

// ListProbes returns probes for an account, newest first, bounded by page.
func (r *MonitoringRepo) ListProbes(ctx context.Context, accountID string, page biz.Page) ([]*biz.MonitoringProbe, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+probeCols+` FROM monitoring_probes
		 WHERE account_id = $1 ORDER BY sent_at DESC LIMIT $2 OFFSET $3`,
		accountID, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("list monitoring probes: %w", err)
	}
	defer rows.Close()
	var out []*biz.MonitoringProbe
	for rows.Next() {
		p, err := scanProbe(rows)
		if err != nil {
			return nil, fmt.Errorf("scan monitoring probe: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateProbe inserts a probe row.
func (r *MonitoringRepo) CreateProbe(ctx context.Context, p *biz.MonitoringProbe) (*biz.MonitoringProbe, error) {
	analysis := p.Analysis
	if analysis == "" {
		analysis = "{}"
	}
	// Coalesce the status enums to their initial values so an unset field can't
	// write an empty string that the reconciler/fetch selectors (which filter on
	// = 'queued' / = 'pending') would never match.
	sendStatus := p.SendStatus
	if sendStatus == "" {
		sendStatus = biz.ProbeSendQueued
	}
	mailboxStatus := p.MailboxStatus
	if mailboxStatus == "" {
		mailboxStatus = biz.ProbeMailboxPending
	}
	out, err := scanProbe(r.db.Pool.QueryRow(ctx, `
		INSERT INTO monitoring_probes
			(account_id, probe_uid, message_id, subject, from_addr, recipient,
			 send_status, mailbox_status, placement, analysis, raw_headers, error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING `+probeCols,
		p.AccountID, p.ProbeUID, p.MessageID, p.Subject, p.FromAddr, p.Recipient,
		sendStatus, mailboxStatus, p.Placement, analysis, p.RawHeaders, p.Error))
	if err != nil {
		return nil, fmt.Errorf("create monitoring probe: %w", err)
	}
	return out, nil
}

// GetProbe returns one probe by id.
func (r *MonitoringRepo) GetProbe(ctx context.Context, id string) (*biz.MonitoringProbe, error) {
	p, err := scanProbe(r.db.Pool.QueryRow(ctx,
		`SELECT `+probeCols+` FROM monitoring_probes WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("MONITOR_PROBE_NOT_FOUND", "monitoring probe %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get monitoring probe: %w", err)
	}
	return p, nil
}

// UpdateProbeSend updates the KumoMTA send outcome and backfills the KumoMTA
// message id (kept if the new value is empty).
func (r *MonitoringRepo) UpdateProbeSend(ctx context.Context, id, status, messageID string) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE monitoring_probes
		 SET send_status = $2,
		     message_id = CASE WHEN $3 <> '' THEN $3 ELSE message_id END,
		     updated_at = now()
		 WHERE id = $1`, id, status, messageID)
	if err != nil {
		return fmt.Errorf("update probe send: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("MONITOR_PROBE_NOT_FOUND", "monitoring probe %q not found", id)
	}
	return nil
}

// CorrelateSend finds the most relevant mail record for a probe's uid-tagged
// From header + recipient since sentAt, mapping its KumoMTA record type/status to
// a probe send status. Terminal outcomes (delivery/bounce) win over transient
// ones so a single query resolves the final state. A 1-minute skew before sentAt
// tolerates clock drift between iris and kumod.
//
// The match on from_header is a substring (not equality): mail_records stores the
// full RFC 5322 header (e.g. `"iris monitor" <monitoring+<uid>@dom>`) while the
// probe carries only the bare plus-tagged address. The uid makes that address
// globally unique, so a contains match is unambiguous. strpos (not LIKE) is used
// so `+`/`_`/`%` in the address carry no wildcard meaning.
func (r *MonitoringRepo) CorrelateSend(ctx context.Context, fromAddr, recipient string, sentAt time.Time) (biz.ProbeSendMatch, error) {
	var status, messageID string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT status, message_id FROM mail_records
		WHERE strpos(lower(from_header), lower($1)) > 0
		  AND lower(recipient) = lower($2)
		  AND event_time >= $3
		ORDER BY
		  CASE status
		    WHEN 'bounced' THEN 0
		    WHEN 'sent' THEN 1
		    WHEN 'suppressed' THEN 2
		    WHEN 'failed' THEN 3
		    WHEN 'deferred' THEN 4
		    ELSE 5
		  END,
		  event_time DESC
		LIMIT 1`,
		fromAddr, recipient, sentAt.Add(-time.Minute)).Scan(&status, &messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.ProbeSendMatch{Found: false}, nil
	}
	if err != nil {
		return biz.ProbeSendMatch{}, fmt.Errorf("correlate probe send: %w", err)
	}
	return biz.ProbeSendMatch{Found: true, Status: mapMailStatusToProbe(status), MessageID: messageID}, nil
}

// mapMailStatusToProbe maps a mail_records status to a probe send status.
func mapMailStatusToProbe(status string) string {
	switch status {
	case biz.MailSent:
		return biz.ProbeSendSent
	case biz.MailBounced, biz.MailFailed, biz.MailSuppressed:
		return biz.ProbeSendBounced
	case biz.MailDeferred:
		return biz.ProbeSendDeferred
	default:
		return biz.ProbeSendQueued
	}
}

// ProbesAwaitingSend returns probes sent since the cutoff whose send status is
// still non-terminal (queued), for the reconciler to correlate.
func (r *MonitoringRepo) ProbesAwaitingSend(ctx context.Context, since time.Time) ([]*biz.MonitoringProbe, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+probeCols+` FROM monitoring_probes
		 WHERE sent_at >= $1 AND send_status = $2 ORDER BY sent_at ASC`,
		since, biz.ProbeSendQueued)
	if err != nil {
		return nil, fmt.Errorf("probes awaiting send: %w", err)
	}
	defer rows.Close()
	var out []*biz.MonitoringProbe
	for rows.Next() {
		p, err := scanProbe(rows)
		if err != nil {
			return nil, fmt.Errorf("scan awaiting probe: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ProbesAwaitingFetch returns probes whose mailbox has not been checked yet
// (mailbox_status pending) whose send was confirmed and whose per-account fetch
// delay has elapsed, paired with their account. The delay is a Go duration
// string, so due-filtering is done in Go after the JOIN.
func (r *MonitoringRepo) ProbesAwaitingFetch(ctx context.Context, now time.Time) ([]*biz.ProbeFetchCandidate, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+probeColsP+`, `+accountColsP+`
		FROM monitoring_probes p
		JOIN monitoring_accounts a ON a.id = p.account_id
		WHERE p.mailbox_status = $1
		  AND p.send_status = $2
		  AND a.enabled = true
		ORDER BY p.sent_at ASC`,
		biz.ProbeMailboxPending, biz.ProbeSendSent)
	if err != nil {
		return nil, fmt.Errorf("probes awaiting fetch: %w", err)
	}
	defer rows.Close()
	var out []*biz.ProbeFetchCandidate
	for rows.Next() {
		p := &biz.MonitoringProbe{}
		a := &biz.MonitoringAccount{}
		if err := rows.Scan(probeScanArgs(p, accountScanArgs(a)...)...); err != nil {
			return nil, fmt.Errorf("scan fetch candidate: %w", err)
		}
		if dueForFetch(p, a, now) {
			out = append(out, &biz.ProbeFetchCandidate{Probe: p, Account: a})
		}
	}
	return out, rows.Err()
}

// dueForFetch reports whether now is past the probe's sent_at + account fetch
// delay (default 10m when unset/invalid).
func dueForFetch(p *biz.MonitoringProbe, a *biz.MonitoringAccount, now time.Time) bool {
	delay, ok := biz.ParseFlexDuration(a.FetchDelay)
	if !ok || delay <= 0 {
		delay = 10 * time.Minute
	}
	return !now.Before(p.SentAt.Add(delay))
}

// UpdateProbeMailbox records the phase-2 mailbox fetch outcome.
func (r *MonitoringRepo) UpdateProbeMailbox(ctx context.Context, id string, u biz.ProbeMailboxUpdate) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE monitoring_probes SET
			mailbox_status = $2,
			placement = $3,
			found_at = $4,
			latency_ms = $5,
			raw_headers = CASE WHEN $6 <> '' THEN $6 ELSE raw_headers END,
			analysis = CASE WHEN $7 <> '' THEN $7::jsonb ELSE analysis END,
			error = $8,
			updated_at = now()
		WHERE id = $1`,
		id, u.MailboxStatus, u.Placement, u.FoundAt, u.LatencyMs, u.RawHeaders, u.Analysis, u.Error)
	if err != nil {
		return fmt.Errorf("update probe mailbox: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("MONITOR_PROBE_NOT_FOUND", "monitoring probe %q not found", id)
	}
	return nil
}

// encrypt returns the ciphertext for a password, or an error if a non-empty
// password is supplied without a configured cipher.
func (r *MonitoringRepo) encrypt(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	if r.cipher == nil {
		return "", biz.Unavailable("MONITOR_CIPHER_UNSET", "IRIS_MONITORING_KEY is not configured; cannot store mailbox password")
	}
	enc, err := r.cipher.Encrypt(password)
	if err != nil {
		return "", fmt.Errorf("encrypt monitoring password: %w", err)
	}
	return enc, nil
}
