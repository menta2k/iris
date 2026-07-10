package biz

import (
	"context"
	"strings"
	"testing"
	"time"
)

func monitorCtx() context.Context {
	return WithIdentity(context.Background(), &Identity{
		UserID: "u", Permissions: NewPermissionSet([]string{string(PermAll)}), MFAVerified: true,
	})
}

type mailboxUpdateRecord struct {
	id     string
	update ProbeMailboxUpdate
}

// fakeMonitoringRepo is an in-memory MonitoringRepo for usecase tests.
type fakeMonitoringRepo struct {
	accounts       map[string]*MonitoringAccount
	probes         []*MonitoringProbe
	match          ProbeSendMatch
	touched        map[string]time.Time
	secrets        map[string]string
	fetchCands     []*ProbeFetchCandidate
	mailboxUpdates []mailboxUpdateRecord
}

func newFakeMonitoringRepo() *fakeMonitoringRepo {
	return &fakeMonitoringRepo{accounts: map[string]*MonitoringAccount{}, touched: map[string]time.Time{}}
}

func (f *fakeMonitoringRepo) ListAccounts(context.Context) ([]*MonitoringAccount, error) {
	out := make([]*MonitoringAccount, 0, len(f.accounts))
	for _, a := range f.accounts {
		out = append(out, a)
	}
	return out, nil
}
func (f *fakeMonitoringRepo) CreateAccount(_ context.Context, a *MonitoringAccount) (*MonitoringAccount, error) {
	cp := *a
	cp.ID = "acc-" + a.Email
	f.accounts[cp.ID] = &cp
	return &cp, nil
}
func (f *fakeMonitoringRepo) UpdateAccount(_ context.Context, a *MonitoringAccount) (*MonitoringAccount, error) {
	f.accounts[a.ID] = a
	return a, nil
}
func (f *fakeMonitoringRepo) SetAccountPassword(context.Context, string, string) error { return nil }
func (f *fakeMonitoringRepo) DeleteAccount(_ context.Context, id string) error {
	delete(f.accounts, id)
	return nil
}
func (f *fakeMonitoringRepo) GetAccount(_ context.Context, id string) (*MonitoringAccount, error) {
	a, ok := f.accounts[id]
	if !ok {
		return nil, NotFound("MONITOR_ACCOUNT_NOT_FOUND", "not found")
	}
	return a, nil
}
func (f *fakeMonitoringRepo) AccountSecret(_ context.Context, id string) (string, error) {
	return f.secrets[id], nil
}
func (f *fakeMonitoringRepo) ScheduledAccounts(context.Context, time.Time) ([]*MonitoringAccount, error) {
	return nil, nil
}
func (f *fakeMonitoringRepo) TouchLastProbe(_ context.Context, id string, at time.Time) error {
	f.touched[id] = at
	return nil
}
func (f *fakeMonitoringRepo) ListProbes(context.Context, string, Page) ([]*MonitoringProbe, error) {
	return f.probes, nil
}
func (f *fakeMonitoringRepo) CreateProbe(_ context.Context, p *MonitoringProbe) (*MonitoringProbe, error) {
	cp := *p
	cp.ID = "probe-1"
	cp.SentAt = time.Unix(1_700_000_000, 0)
	f.probes = append(f.probes, &cp)
	return &cp, nil
}
func (f *fakeMonitoringRepo) GetProbe(context.Context, string) (*MonitoringProbe, error) {
	return nil, nil
}
func (f *fakeMonitoringRepo) UpdateProbeSend(_ context.Context, id, status, _ string) error {
	for _, p := range f.probes {
		if p.ID == id {
			p.SendStatus = status
		}
	}
	return nil
}
func (f *fakeMonitoringRepo) ProbesAwaitingSend(context.Context, time.Time) ([]*MonitoringProbe, error) {
	return f.probes, nil
}
func (f *fakeMonitoringRepo) CorrelateSend(context.Context, string, string, time.Time) (ProbeSendMatch, error) {
	return f.match, nil
}
func (f *fakeMonitoringRepo) ProbesAwaitingFetch(context.Context, time.Time) ([]*ProbeFetchCandidate, error) {
	return f.fetchCands, nil
}
func (f *fakeMonitoringRepo) UpdateProbeMailbox(_ context.Context, id string, u ProbeMailboxUpdate) error {
	f.mailboxUpdates = append(f.mailboxUpdates, mailboxUpdateRecord{id: id, update: u})
	return nil
}
func (f *fakeMonitoringRepo) ProbeRaw(context.Context, string) (*ProbeRawMessage, error) {
	return &ProbeRawMessage{}, nil
}

// recordingInjector captures the last injected request.
type recordingInjector struct {
	last KumoInjectRequest
	err  error
}

func (r *recordingInjector) InjectV1(_ context.Context, req KumoInjectRequest) error {
	r.last = req
	return r.err
}

// fakeSettings supplies a canned monitoring policy.
type fakeSettings struct{ policy MonitoringPolicy }

func (f fakeSettings) MonitoringPolicyNow(context.Context) MonitoringPolicy { return f.policy }

func TestSendProbeTagsFromAndRecords(t *testing.T) {
	repo := newFakeMonitoringRepo()
	repo.accounts["a1"] = &MonitoringAccount{
		ID: "a1", Email: "seed@gmail.com", FromAddress: "probe@monitor.example.com", Enabled: true,
	}
	inj := &recordingInjector{}
	uc := NewMonitoringUsecase(repo, inj, nil)

	probe, err := uc.SendProbe(monitorCtx(), "a1")
	if err != nil {
		t.Fatalf("SendProbe: %v", err)
	}
	if probe.SendStatus != ProbeSendQueued {
		t.Errorf("send status = %q, want queued", probe.SendStatus)
	}
	// Must be seeded pending so the phase-2 fetch selector (mailbox_status =
	// 'pending') can pick it up once it is sent.
	if probe.MailboxStatus != ProbeMailboxPending {
		t.Errorf("mailbox status = %q, want pending", probe.MailboxStatus)
	}
	// From header must be plus-tagged with the probe uid for later correlation.
	wantPrefix := "probe+" + probe.ProbeUID + "@"
	if !strings.HasPrefix(inj.last.Content.From.Email, wantPrefix) {
		t.Errorf("from = %q, want prefix %q", inj.last.Content.From.Email, wantPrefix)
	}
	// Envelope sender stays the base address (VERP-rewritten downstream).
	if inj.last.EnvelopeSender != "probe@monitor.example.com" {
		t.Errorf("envelope sender = %q", inj.last.EnvelopeSender)
	}
	// The probe uid must ride in the correlation header.
	if inj.last.Content.Headers[ProbeUIDHeader] != probe.ProbeUID {
		t.Errorf("missing %s header", ProbeUIDHeader)
	}
	if _, ok := repo.touched["a1"]; !ok {
		t.Error("last_probe_at was not touched")
	}
}

func TestSendProbeUsesDefaultFrom(t *testing.T) {
	repo := newFakeMonitoringRepo()
	repo.accounts["a1"] = &MonitoringAccount{ID: "a1", Email: "seed@gmail.com", Enabled: true}
	inj := &recordingInjector{}
	uc := NewMonitoringUsecase(repo, inj, nil).
		WithSettings(fakeSettings{policy: MonitoringPolicy{From: "fallback@monitor.example.com"}})

	if _, err := uc.SendProbe(monitorCtx(), "a1"); err != nil {
		t.Fatalf("SendProbe: %v", err)
	}
	if inj.last.EnvelopeSender != "fallback@monitor.example.com" {
		t.Errorf("envelope sender = %q, want fallback", inj.last.EnvelopeSender)
	}
}

func TestSendProbeNoFromFails(t *testing.T) {
	repo := newFakeMonitoringRepo()
	repo.accounts["a1"] = &MonitoringAccount{ID: "a1", Email: "seed@gmail.com", Enabled: true}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil)
	if _, err := uc.SendProbe(monitorCtx(), "a1"); err == nil {
		t.Error("expected error when no from address is configured")
	}
}

func TestReconcileSendsAdvancesStatus(t *testing.T) {
	repo := newFakeMonitoringRepo()
	repo.probes = []*MonitoringProbe{{
		ID: "probe-1", FromAddr: "probe+x@m.example.com", Recipient: "seed@gmail.com",
		SendStatus: ProbeSendQueued, SentAt: time.Unix(1_700_000_000, 0),
	}}
	repo.match = ProbeSendMatch{Found: true, Status: ProbeSendSent, MessageID: "msg1"}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil)

	n, err := uc.ReconcileSends(context.Background())
	if err != nil {
		t.Fatalf("ReconcileSends: %v", err)
	}
	if n != 1 {
		t.Fatalf("advanced = %d, want 1", n)
	}
	if repo.probes[0].SendStatus != ProbeSendSent {
		t.Errorf("status = %q, want sent", repo.probes[0].SendStatus)
	}
}

func TestReconcileSkipsWhenNoMatch(t *testing.T) {
	repo := newFakeMonitoringRepo()
	repo.probes = []*MonitoringProbe{{ID: "probe-1", SendStatus: ProbeSendQueued, SentAt: time.Unix(1_700_000_000, 0)}}
	repo.match = ProbeSendMatch{Found: false}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil)
	n, err := uc.ReconcileSends(context.Background())
	if err != nil {
		t.Fatalf("ReconcileSends: %v", err)
	}
	if n != 0 {
		t.Errorf("advanced = %d, want 0", n)
	}
}

func TestMonitoringAccountValidate(t *testing.T) {
	cases := []struct {
		name string
		acc  MonitoringAccount
		ok   bool
	}{
		{"valid", MonitoringAccount{Label: "l", Email: "a@b.com", Host: "imap.b.com", Port: 993}, true},
		{"missing label", MonitoringAccount{Email: "a@b.com", Host: "h", Port: 993}, false},
		{"bad email", MonitoringAccount{Label: "l", Email: "nope", Host: "h", Port: 993}, false},
		{"bad protocol", MonitoringAccount{Label: "l", Email: "a@b.com", Protocol: "smtp", Host: "h", Port: 993}, false},
		{"bad port", MonitoringAccount{Label: "l", Email: "a@b.com", Host: "h", Port: 0}, false},
		{"bad schedule", MonitoringAccount{Label: "l", Email: "a@b.com", Host: "h", Port: 1, ScheduleEnabled: true, ScheduleInterval: "soon"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.acc.Validate()
			if c.ok && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
			if !c.ok && err == nil {
				t.Error("Validate() = nil, want error")
			}
		})
	}
}

func TestMonitoringValidateDefaults(t *testing.T) {
	a := MonitoringAccount{Label: "l", Email: "A@B.com", Host: "h", Port: 993}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if a.Email != "a@b.com" {
		t.Errorf("email not lowercased: %q", a.Email)
	}
	if a.Provider != "custom" {
		t.Errorf("provider default = %q, want custom", a.Provider)
	}
	if a.Protocol != MonitorProtocolIMAP {
		t.Errorf("protocol default = %q, want imap", a.Protocol)
	}
	if a.Username != "a@b.com" {
		t.Errorf("username default = %q, want email", a.Username)
	}
	if len(a.CheckFolders) != 1 || a.CheckFolders[0] != "INBOX" {
		t.Errorf("check folders default = %v, want [INBOX]", a.CheckFolders)
	}
}

// fakeFetcher returns a canned result/error for the mailbox search.
type fakeFetcher struct {
	res  MailboxProbeResult
	err  error
	uids []string
}

func (f *fakeFetcher) Fetch(_ context.Context, _ *MonitoringAccount, _, probeUID string) (MailboxProbeResult, error) {
	f.uids = append(f.uids, probeUID)
	return f.res, f.err
}
func (f *fakeFetcher) Verify(_ context.Context, _ *MonitoringAccount, _ string) error { return f.err }

func fetchCtx() context.Context { return context.Background() }

func candidate(sentAgo time.Duration, now time.Time) *ProbeFetchCandidate {
	return &ProbeFetchCandidate{
		Probe:   &MonitoringProbe{ID: "p1", ProbeUID: "ipabc", SentAt: now.Add(-sentAgo), Placement: ""},
		Account: &MonitoringAccount{ID: "a1", Protocol: MonitorProtocolIMAP, CheckFolders: []string{"INBOX"}},
	}
}

func TestRunDueFetchesFoundInInbox(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	repo := newFakeMonitoringRepo()
	repo.secrets = map[string]string{"a1": "pw"}
	repo.fetchCands = []*ProbeFetchCandidate{candidate(30*time.Minute, now)}
	f := &fakeFetcher{res: MailboxProbeResult{Found: true, Folder: "INBOX", RawHeaders: "X-Iris-Probe-Id: ipabc"}}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil).WithClock(func() time.Time { return now }).WithFetcher(f)

	n, err := uc.RunDueFetches(fetchCtx())
	if err != nil || n != 1 {
		t.Fatalf("RunDueFetches = %d, %v", n, err)
	}
	if len(repo.mailboxUpdates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.mailboxUpdates))
	}
	u := repo.mailboxUpdates[0].update
	if u.MailboxStatus != ProbeMailboxFound || u.Placement != PlacementInbox {
		t.Errorf("status=%q placement=%q, want found/inbox", u.MailboxStatus, u.Placement)
	}
	if u.RawHeaders == "" || u.LatencyMs == nil || u.FoundAt == nil {
		t.Error("expected headers, latency, found_at to be set")
	}
	// Phase-3 analysis is produced inline; with no analyzer it is the heuristic.
	if !strings.Contains(u.Analysis, `"verdict"`) || !strings.Contains(u.Analysis, `"heuristic"`) {
		t.Errorf("analysis missing heuristic verdict: %q", u.Analysis)
	}
}

// stubAnalyzer returns a canned LLM verdict.
type stubAnalyzer struct{ v LLMHeaderVerdict }

func (s stubAnalyzer) AnalyzeHeaders(context.Context, string) (LLMHeaderVerdict, error) {
	return s.v, nil
}

func TestRunDueFetchesUsesLLMAnalysis(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	repo := newFakeMonitoringRepo()
	repo.fetchCands = []*ProbeFetchCandidate{candidate(30*time.Minute, now)}
	f := &fakeFetcher{res: MailboxProbeResult{Found: true, Folder: "INBOX", RawHeaders: "Subject: x"}}
	analyzer := stubAnalyzer{v: LLMHeaderVerdict{Verdict: VerdictSpam, Confidence: 0.9, Summary: "DKIM failed", Factors: []string{"dkim fail"}}}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil).
		WithClock(func() time.Time { return now }).
		WithFetcher(f).
		WithAnalyzer(analyzer)

	if _, err := uc.RunDueFetches(fetchCtx()); err != nil {
		t.Fatal(err)
	}
	a := repo.mailboxUpdates[0].update.Analysis
	if !strings.Contains(a, `"verdict":"spam"`) || !strings.Contains(a, `"source":"llm"`) {
		t.Errorf("expected llm spam verdict, got %q", a)
	}
	// Placement stays folder-truth (inbox) even when the LLM says spam-risk.
	if repo.mailboxUpdates[0].update.Placement != PlacementInbox {
		t.Errorf("placement = %q, want inbox (folder truth)", repo.mailboxUpdates[0].update.Placement)
	}
}

func TestRunDueFetchesSpamFolder(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	repo := newFakeMonitoringRepo()
	repo.fetchCands = []*ProbeFetchCandidate{candidate(30*time.Minute, now)}
	f := &fakeFetcher{res: MailboxProbeResult{Found: true, Folder: "[Gmail]/Spam"}}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil).WithClock(func() time.Time { return now }).WithFetcher(f)
	if _, err := uc.RunDueFetches(fetchCtx()); err != nil {
		t.Fatal(err)
	}
	if repo.mailboxUpdates[0].update.Placement != PlacementSpam {
		t.Errorf("placement = %q, want spam", repo.mailboxUpdates[0].update.Placement)
	}
}

func TestRunDueFetchesNotFoundRetriesThenGivesUp(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	f := &fakeFetcher{res: MailboxProbeResult{Found: false}}

	// Within the give-up window: stays pending, no terminal advance.
	repo := newFakeMonitoringRepo()
	repo.fetchCands = []*ProbeFetchCandidate{candidate(10*time.Minute, now)}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil).WithClock(func() time.Time { return now }).WithFetcher(f)
	n, _ := uc.RunDueFetches(fetchCtx())
	if n != 0 {
		t.Errorf("within window advanced = %d, want 0", n)
	}

	// Past give-up: marked not_found/missing.
	repo2 := newFakeMonitoringRepo()
	repo2.fetchCands = []*ProbeFetchCandidate{candidate(2*time.Hour, now)}
	uc2 := NewMonitoringUsecase(repo2, &recordingInjector{}, nil).WithClock(func() time.Time { return now }).WithFetcher(f)
	n2, _ := uc2.RunDueFetches(fetchCtx())
	if n2 != 1 {
		t.Fatalf("past give-up advanced = %d, want 1", n2)
	}
	u := repo2.mailboxUpdates[len(repo2.mailboxUpdates)-1].update
	if u.MailboxStatus != ProbeMailboxNotFound || u.Placement != PlacementMissing {
		t.Errorf("status=%q placement=%q, want not_found/missing", u.MailboxStatus, u.Placement)
	}
}

func TestRunDueFetchesNoFetcherIsNoop(t *testing.T) {
	repo := newFakeMonitoringRepo()
	repo.fetchCands = []*ProbeFetchCandidate{candidate(time.Hour, time.Unix(1_700_000_000, 0))}
	uc := NewMonitoringUsecase(repo, &recordingInjector{}, nil)
	n, err := uc.RunDueFetches(fetchCtx())
	if err != nil || n != 0 {
		t.Errorf("no fetcher: got %d, %v; want 0, nil", n, err)
	}
}

func TestPlacementFromFolder(t *testing.T) {
	cases := map[string]string{
		"":             PlacementInbox,
		"INBOX":        PlacementInbox,
		"[Gmail]/Spam": PlacementSpam,
		"Junk":         PlacementSpam,
		"Archive":      PlacementUnknown,
	}
	for folder, want := range cases {
		if got := placementFromFolder(folder); got != want {
			t.Errorf("placementFromFolder(%q) = %q, want %q", folder, got, want)
		}
	}
}

func TestPlusTag(t *testing.T) {
	if got := plusTag("box@example.com", "uid"); got != "box+uid@example.com" {
		t.Errorf("plusTag = %q", got)
	}
	if got := plusTag("no-at", "uid"); got != "no-at" {
		t.Errorf("plusTag no-at = %q, want unchanged", got)
	}
}
