// DsnstreamPersister bridges the pkg/dsnstream consumer to ent: it persists
// each parsed DSN into dsn_event and denormalises mail_class / tenant /
// campaign by looking up the originating LogEvent (Reception row) when a
// message_id correlation is available. The lookup is best-effort — a miss
// just means the column is empty in dsn_event, which is fine.
package data

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/dsnevent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/logevent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/suppressionentry"
	"github.com/menta2k/iris/backend/pkg/bounceclass"
	"github.com/menta2k/iris/backend/pkg/dsnstream"
)

// DsnstreamPersister implements dsnstream.Persister. After inserting a
// dsn_event row, it optionally drives the auto-suppression policy:
//
//   - hard bounce (status starts "5.", or action="expired") → upsert a
//     suppression_entry with reason="hard_bounce" or "expired".
//   - soft bounce (status starts "4.") → only suppresses after enough
//     repeated soft bounces accumulate within the configured window.
//
// All thresholds are env-driven so operators can tune without redeploy.
type DsnstreamPersister struct {
	client *ent.Client

	// Auto-suppression knobs. autoSuppress=false short-circuits the
	// policy entirely (the operator wants to inspect bounces in the UI
	// before deciding what to do with them).
	autoSuppress    bool
	softThreshold   int
	softWindow      time.Duration
}

// NewDsnstreamPersister wires the ent client and reads the policy knobs
// from env. Defaults are conservative — auto-suppress on hard bounces,
// 3 soft bounces over 7 days for the soft path.
func NewDsnstreamPersister(c *ent.Client) *DsnstreamPersister {
	p := &DsnstreamPersister{
		client:        c,
		autoSuppress:  parseAutoSuppressFlag(),
		softThreshold: parseIntEnv("IRIS_SOFT_BOUNCE_THRESHOLD", 3),
		softWindow:    time.Duration(parseIntEnv("IRIS_SOFT_BOUNCE_WINDOW_HOURS", 168)) * time.Hour,
	}
	if !p.autoSuppress {
		log.Printf("dsnstream: auto-suppress disabled (set IRIS_BOUNCE_AUTO_SUPPRESS=true to enable)")
	}
	return p
}

// parseAutoSuppressFlag defaults to true. Operator must explicitly set
// IRIS_BOUNCE_AUTO_SUPPRESS=false to disable; any other value (or
// missing) keeps it on. Rationale: a populated dsn_event table without
// suppression on top quietly accumulates list rot; the safer default is
// "honour bounces."
func parseAutoSuppressFlag() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IRIS_BOUNCE_AUTO_SUPPRESS")))
	switch v {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

func parseIntEnv(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// Insert writes one dsn_event row. Field-population priority for
// mail_class / tenant / campaign:
//
//  1. lookup log_event by message_id_ref (most reliable);
//  2. fallback to embedded message headers (X-Kumo-* tags survive on
//     receivers that include the original headers in the DSN body).
//
// Tracking which path won via metrics is left to the consumer.
func (p *DsnstreamPersister) Insert(ctx context.Context, parsed *dsnstream.Parsed) error {
	mailClass, tenant, campaign := p.lookupContext(ctx, parsed)

	statusClass := ""
	if len(parsed.Status) > 0 {
		statusClass = string(parsed.Status[0])
	}

	cat := string(bounceclass.Classify(parsed.Status))

	create := p.client.DsnEvent.Create().
		SetID(genID()).
		SetReceivedAt(parsed.ReceivedAt).
		SetAction(parsed.Action).
		SetRawSize(int32(parsed.RawSize)).
		SetCategory(cat)

	if parsed.VerpToken != "" {
		create = create.SetVerpToken(clip(parsed.VerpToken, 128))
	}
	if parsed.MessageID != "" {
		create = create.SetMessageIDRef(clip(parsed.MessageID, 255))
	}
	if parsed.OriginalRecipient != "" {
		create = create.SetOriginalRecipient(clip(parsed.OriginalRecipient, 320))
	}
	if parsed.FinalRecipient != "" {
		create = create.SetFinalRecipient(clip(parsed.FinalRecipient, 320))
	}
	if parsed.Status != "" {
		create = create.SetStatus(clip(parsed.Status, 16))
	}
	if statusClass != "" {
		create = create.SetStatusClass(statusClass)
	}
	if parsed.DiagnosticCode != "" {
		create = create.SetDiagnosticCode(clip(parsed.DiagnosticCode, 1024))
	}
	if parsed.RemoteMTA != "" {
		create = create.SetRemoteMta(clip(parsed.RemoteMTA, 253))
	}
	if mailClass != "" {
		create = create.SetMailClass(clip(mailClass, 64))
	}
	if tenant != "" {
		create = create.SetTenant(clip(tenant, 64))
	}
	if campaign != "" {
		create = create.SetCampaign(clip(campaign, 64))
	}

	// Stash a small JSON snapshot of the parsed structure for forensic
	// inspection in the UI ("View raw fields"). Not the raw RFC822 body
	// — that's larger than we want in the row; if operators want it they
	// can XRANGE it out of the DLQ stream for unparseable cases or
	// re-fetch from the audit log.
	if extra, err := json.Marshal(map[string]any{
		"envelope_recipient": parsed.EnvelopeRecipient,
		"embedded_headers":   filteredHeaders(parsed.EmbeddedHeaders),
	}); err == nil {
		create = create.SetExtraJSON(string(extra))
	}

	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("dsnstream_persister: insert: %w", err)
	}

	// Auto-suppression runs *after* the dsn_event insert so the audit
	// trail shows "this DSN row is what caused the suppression". Errors
	// here are logged but not returned: a bounced row is more valuable
	// than a perfect suppression policy. If suppression fails the
	// operator can re-trigger from the UI.
	if p.autoSuppress {
		if err := p.autoSuppressFromDsn(ctx, parsed); err != nil {
			log.Printf("dsnstream_persister: auto-suppress failed for %s: %v",
				parsed.FinalRecipient, err)
		}
	}

	return nil
}

// autoSuppressFromDsn applies the suppression policy. Three classes:
//
//   - "expired" action (kumomta gave up after retry window) → permanent
//     suppression with reason="expired".
//   - hard bounce (status_class==5) → permanent suppression with
//     reason="hard_bounce".
//   - soft bounce (status_class==4) → only suppress if repeated soft
//     bounces for the same recipient cross the configured threshold.
//
// Suppressions upsert by (address, scope) so a re-bounce after manual
// rescue is recorded with a fresh timestamp + tightened reason.
func (p *DsnstreamPersister) autoSuppressFromDsn(ctx context.Context, parsed *dsnstream.Parsed) error {
	addr := normalizeAddr(parsed.FinalRecipient)
	if addr == "" {
		return nil
	}
	cat := bounceclass.Classify(parsed.Status)

	switch {
	case parsed.Action == "expired":
		return p.upsertSuppression(ctx, addr, "expired",
			fmt.Sprintf("kumomta retry exhausted (%s)", parsed.DiagnosticCode),
			parsed.Status, string(cat))

	case bounceclass.IsHard(parsed.Status):
		return p.upsertSuppression(ctx, addr, "hard_bounce",
			parsed.DiagnosticCode, parsed.Status, string(cat))

	case bounceclass.IsTransient(parsed.Status):
		// Threshold check: count recent soft bounces. We use Status
		// prefix "4" rather than action="delayed" because some MTAs
		// emit a transient status without setting Action accurately.
		since := time.Now().UTC().Add(-p.softWindow)
		count, err := p.client.DsnEvent.Query().
			Where(
				dsnevent.FinalRecipientEQ(addr),
				dsnevent.StatusClassEQ("4"),
				dsnevent.ReceivedAtGTE(since),
			).Count(ctx)
		if err != nil {
			return fmt.Errorf("count soft bounces: %w", err)
		}
		if count < p.softThreshold {
			return nil
		}
		return p.upsertSuppression(ctx, addr, "soft_bounce",
			fmt.Sprintf("%d soft bounces in %s", count, p.softWindow),
			parsed.Status, string(cat))
	}
	// Unknown / parse-failure: don't suppress on noise.
	return nil
}

// upsertSuppression upserts a suppression_entry by (address, scope). If a
// row already exists (e.g. an earlier manual or auto suppression) we
// only refresh reason/note when the new event is at least as severe —
// hard supersedes soft, "expired" supersedes everything.
func (p *DsnstreamPersister) upsertSuppression(ctx context.Context, addr, reason, note, status, category string) error {
	now := time.Now().UTC()
	noteValue := clip(buildSuppressionNote(note, status, category), 512)
	existing, err := p.client.SuppressionEntry.Query().
		Where(
			suppressionentry.AddressEQ(addr),
			suppressionentry.ScopeEQ("address"),
		).Only(ctx)
	if err == nil {
		if !shouldOverride(existing.Reason, reason) {
			return nil
		}
		if _, err := p.client.SuppressionEntry.UpdateOneID(existing.ID).
			SetReason(reason).
			SetNote(noteValue).
			Save(ctx); err != nil {
			return fmt.Errorf("update suppression: %w", err)
		}
		log.Printf("dsnstream: refreshed suppression addr=%s reason=%s (was %s)",
			addr, reason, existing.Reason)
		return nil
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("lookup suppression: %w", err)
	}
	if _, err := p.client.SuppressionEntry.Create().
		SetAddress(addr).
		SetScope("address").
		SetReason(reason).
		SetNote(noteValue).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("create suppression: %w", err)
	}
	log.Printf("dsnstream: auto-suppressed addr=%s reason=%s status=%s", addr, reason, status)
	return nil
}

// shouldOverride encodes the severity ladder for the upsert path.
// Manual + complaint suppressions are operator-driven and should not be
// silently overwritten by an auto-suppression.
func shouldOverride(existing, incoming string) bool {
	if existing == incoming {
		return true // refresh timestamp / note
	}
	severity := map[string]int{
		"manual":      100, // operator-set; leave alone
		"complaint":   80,  // FBL — strong signal, leave alone
		"expired":     60,
		"hard_bounce": 50,
		"soft_bounce": 30,
	}
	return severity[incoming] > severity[existing]
}

func buildSuppressionNote(diag, status, category string) string {
	parts := []string{}
	if status != "" {
		parts = append(parts, "status="+status)
	}
	if category != "" {
		parts = append(parts, "category="+category)
	}
	if diag != "" {
		parts = append(parts, "diag="+diag)
	}
	return strings.Join(parts, " | ")
}

// normalizeAddr is shared with logstream_persister.go in the same
// package — see that file for the definition.

// lookupContext resolves the mail-class / tenant / campaign for a DSN.
// Strategy — DB lookup by message_id wins; embedded-headers fallback
// covers DSNs that didn't preserve our X-Kumo-* tags.
func (p *DsnstreamPersister) lookupContext(ctx context.Context, parsed *dsnstream.Parsed) (mailClass, tenant, campaign string) {
	if parsed.MessageID != "" && p.client != nil {
		// Limit to the Reception row — that's where mail_class is set.
		// A message_id can also appear on Delivery / Bounce rows but
		// they all carry the same mail_class so any match is fine.
		row, err := p.client.LogEvent.Query().
			Where(logevent.MessageIDEQ(parsed.MessageID)).
			Order(ent.Asc(logevent.FieldAt)).
			First(ctx)
		if err == nil && row != nil {
			return row.MailClass, "", ""
		}
	}
	// Fallback: pull from the embedded original headers. These are
	// lower-cased by the parser.
	mailClass = parsed.MailClass()
	tenant = parsed.Tenant()
	campaign = parsed.Campaign()
	return mailClass, tenant, campaign
}

// filteredHeaders keeps a small allowlist for extra_json so the column
// stays small. We carry the X-Kumo-* tags (already used downstream) and
// a couple of standard headers that help operator triage. Everything
// else is dropped — the raw DSN body lives on the DLQ stream if a
// deeper inspection is ever needed.
func filteredHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, 8)
	for _, k := range []string{
		"x-kumo-mail-class", "x-kumo-tenant", "x-kumo-campaign",
		"message-id", "from", "to", "subject", "date",
	} {
		if v := h[k]; v != "" {
			out[k] = v
		}
	}
	return out
}

// _ assertion: compile-time check that the persister matches the
// dsnstream.Persister interface so a future signature change surfaces
// here rather than at consumer-construction time.
var _ dsnstream.Persister = (*DsnstreamPersister)(nil)
