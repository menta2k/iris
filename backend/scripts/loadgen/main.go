// loadgen — iris end-to-end test harness.
//
//	go run ./scripts/loadgen -scenario deploy/test/scenarios/mixed.yaml \
//	   -admin http://admin-service:8000 -smtp kumomta:2525
//
// Runs three phases:
//
//  1. Setup. Logs in, creates VMTAs / groups / classes / rules / suppressions
//     described in scenario.setup. Idempotent — re-running against an
//     already-seeded backend is fine.
//
//  2. Apply. Calls /v1/policy/apply so the admin-service writes the
//     freshly-rendered init.lua. The kumomta container needs a restart
//     to pick up listener changes; the harness assumes that's already
//     happened (see deploy/docker-compose.test.yaml).
//
//  3. Traffic + asserts. Runs each scenarios[] block (rate-limited or
//     burst) concurrently, waits for the Redis stream to drain, then
//     queries /v1/logs + /v1/feedback + /v1/suppressions to evaluate the
//     assertions. Exits 0 iff every assertion passed.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

func main() {
	scenarioPath := flag.String("scenario", "/scenarios/mixed.yaml", "path to YAML scenario")
	adminURL := flag.String("admin", "http://admin-service:8000", "admin-service base URL")
	smtpAddr := flag.String("smtp", "kumomta:2525", "kumomta SMTP host:port")
	username := flag.String("user", "admin", "admin user")
	password := flag.String("pass", "admin", "admin password")
	bootTimeout := flag.Duration("boot-timeout", 60*time.Second, "max wait for admin-service")
	drainGrace := flag.Duration("drain", 60*time.Second, "wait after traffic before asserting (kumomta delivery + log-stream + consumer latency stacks; 30-45s observed)")
	keepArtifacts := flag.Bool("keep", false, "skip teardown — leave the seeded scenario in the DB so the rendered policy keeps reflecting the test fixture (useful when debugging)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	scn, err := LoadScenario(*scenarioPath)
	if err != nil {
		log.Fatalf("loadgen: scenario: %v", err)
	}

	c := NewAdminClient(*adminURL)
	log.Printf("loadgen: waiting for admin-service at %s …", *adminURL)
	if err := c.WaitForAdmin(*bootTimeout); err != nil {
		log.Fatalf("loadgen: %v", err)
	}
	if err := c.Login(*username, *password); err != nil {
		log.Fatalf("loadgen: login: %v", err)
	}
	log.Printf("loadgen: logged in as %s", *username)

	if err := setup(c, scn.Setup); err != nil {
		log.Fatalf("loadgen: setup: %v", err)
	}
	if err := c.ApplyPolicy(); err != nil {
		log.Printf("loadgen: policy/apply non-fatal error: %v", err)
	}
	// kumomta reloads the policy file via an epoch task that polls every
	// 10s by default. Without this wait, our scenarios race the bootstrap
	// policy → routing rules and the mailclass meta haven't taken effect
	// yet, so messages get logged without the test-mode tagging.
	const policyReloadGrace = 12 * time.Second
	log.Printf("loadgen: waiting %s for kumomta to pick up the new policy", policyReloadGrace)
	time.Sleep(policyReloadGrace)

	since := time.Now()

	// Run scenarios concurrently — the harness exists to surface kumomta
	// behaviour under mixed traffic, not strictly-serial submission.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	results := make([]RunResult, len(scn.Scenarios))
	for i, s := range scn.Scenarios {
		wg.Add(1)
		go func(idx int, t TrafficBlock) {
			defer wg.Done()
			log.Printf("loadgen: scenario[%s] starting from=%s to=%s", t.Name, t.From, t.To)
			results[idx] = runScenario(ctx, *smtpAddr, t)
			r := results[idx]
			log.Printf("loadgen: scenario[%s] done sent=%d failed=%d duration=%s",
				t.Name, r.Sent, r.Failed, r.FinishedAt.Sub(r.StartedAt))
		}(i, s)
	}
	wg.Wait()

	totalSent := int64(0)
	for _, r := range results {
		totalSent += r.Sent
	}
	log.Printf("loadgen: traffic phase complete; total accepted by kumomta: %d. Draining for %s before asserting.",
		totalSent, *drainGrace)
	time.Sleep(*drainGrace)

	logCounts, _, err := c.CountByEventType(since)
	if err != nil {
		log.Fatalf("loadgen: query logs: %v", err)
	}
	fbTotal, err := c.FeedbackTotal()
	if err != nil {
		log.Fatalf("loadgen: query feedback: %v", err)
	}
	suppByReason, err := c.SuppressionsByReason()
	if err != nil {
		log.Fatalf("loadgen: query suppressions: %v", err)
	}

	log.Printf("loadgen: observed log_event by type: %v", logCounts)
	log.Printf("loadgen: feedback_reports total: %d", fbTotal)
	log.Printf("loadgen: suppressions by reason: %v", suppByReason)

	asserts := EvaluateAsserts(scn.Assert, logCounts, fbTotal, suppByReason)
	failed := 0
	for _, r := range asserts {
		fmt.Println(r)
		if !r.Passed {
			failed++
		}
	}
	// Teardown BEFORE Exit() — otherwise os.Exit short-circuits the cleanup.
	// We still teardown on assertion failure (the test artifacts shouldn't
	// linger just because the run was red), unless -keep is set so the
	// operator can inspect the broken state.
	if *keepArtifacts {
		log.Printf("loadgen: -keep set; leaving scenario artifacts in the DB (init.lua will reflect the test fixture)")
	} else {
		extraSuppr := make([]string, 0, len(scn.Scenarios))
		for _, t := range scn.Scenarios {
			extraSuppr = append(extraSuppr, t.To) // FBL auto-suppression target
		}
		if err := c.Teardown(scn.Setup, extraSuppr); err != nil {
			log.Printf("loadgen: teardown: %v (DB may have lingering test artifacts)", err)
		} else {
			log.Printf("loadgen: teardown complete — DB is back to a production-clean state")
			// Re-render so the on-disk init.lua is the production baseline.
			if err := c.ApplyPolicy(); err != nil {
				log.Printf("loadgen: post-teardown policy/apply non-fatal error: %v", err)
			}
		}
	}

	if failed > 0 {
		log.Printf("loadgen: %d assertion(s) failed", failed)
		os.Exit(1)
	}
	log.Printf("loadgen: all %d assertion(s) passed ✓", len(asserts))
}

// setup brings the SetupBlock to existence in admin-service. Each step is
// idempotent — re-runs against an already-seeded DB succeed silently.
func setup(c *AdminClient, s SetupBlock) error {
	vmtaIDs := map[string]int{}
	for _, v := range s.VMTAs {
		id, err := c.CreateVMTA(v)
		if err != nil {
			return fmt.Errorf("vmta %s: %w", v.Name, err)
		}
		vmtaIDs[v.Name] = id
		log.Printf("loadgen: vmta %s id=%d", v.Name, id)
	}
	for _, g := range s.Groups {
		if err := c.CreateGroup(g, vmtaIDs); err != nil {
			return fmt.Errorf("group %s: %w", g.Name, err)
		}
		log.Printf("loadgen: group %s (%d members)", g.Name, len(g.Members))
	}
	for _, mc := range s.Classes {
		if err := c.CreateMailClass(mc); err != nil {
			return fmt.Errorf("class %s: %w", mc.Name, err)
		}
		log.Printf("loadgen: class %s → %s/%s", mc.Name, mc.TargetKind, mc.TargetRef)
	}
	for _, r := range s.Rules {
		if err := c.CreateRule(r); err != nil {
			return fmt.Errorf("rule %s: %w", r.Name, err)
		}
		log.Printf("loadgen: rule %s (priority=%d) → %s/%s", r.Name, r.Priority, r.TargetK, r.TargetRef)
	}
	for _, sup := range s.Suppressions {
		if err := c.CreateSuppression(sup); err != nil {
			return fmt.Errorf("suppression %s: %w", sup.Address, err)
		}
		log.Printf("loadgen: suppression %s (%s)", sup.Address, sup.Scope)
	}
	return nil
}
