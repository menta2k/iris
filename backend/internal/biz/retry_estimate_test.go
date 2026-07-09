package biz

import (
	"testing"
	"time"
)

func rec(rt, status string, t time.Time) *MailRecord {
	return &MailRecord{RecordType: rt, Status: status, EventTime: t}
}

func TestEstimateNextAttempt(t *testing.T) {
	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	pol := RetrySchedule{Interval: 20 * time.Minute, MaxAge: 7 * 24 * time.Hour}

	// Reception + 3 deferrals → next interval is 20m*2^2 = 80m after the last one.
	events := []*MailRecord{
		rec("Reception", "received", base),
		rec("TransientFailure", "deferred", base.Add(1*time.Minute)),
		rec("TransientFailure", "deferred", base.Add(21*time.Minute)),
		rec("TransientFailure", "deferred", base.Add(61*time.Minute)), // last deferral
	}
	est := EstimateNextAttempt(events, pol)
	if !est.Deferred || est.Attempts != 3 {
		t.Fatalf("expected deferred with 3 attempts, got %+v", est)
	}
	if est.Interval != 80*time.Minute {
		t.Fatalf("expected 80m next interval, got %s", est.Interval)
	}
	want := base.Add(61 * time.Minute).Add(80 * time.Minute)
	if !est.NextAttempt.Equal(want) {
		t.Fatalf("next attempt = %s, want %s", est.NextAttempt, want)
	}
	if est.RemainingAttempts < 1 {
		t.Fatalf("expected remaining attempts >= 1, got %d", est.RemainingAttempts)
	}
	if est.ExpiresAt != base.Add(7*24*time.Hour) {
		t.Fatalf("expires at %s, want creation+7d", est.ExpiresAt)
	}

	// A delivered message is not deferred.
	done := EstimateNextAttempt([]*MailRecord{
		rec("Reception", "received", base),
		rec("TransientFailure", "deferred", base.Add(time.Minute)),
		rec("Delivery", "sent", base.Add(30*time.Minute)),
	}, pol)
	if done.Deferred {
		t.Fatalf("delivered message must not be deferred: %+v", done)
	}

	// No deferral → not deferred.
	if EstimateNextAttempt([]*MailRecord{rec("Reception", "received", base)}, pol).Deferred {
		t.Fatal("reception-only message must not be deferred")
	}
}

func TestEstimateRemainingAndExpiry(t *testing.T) {
	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	// Uncapped 20m base over a 7d max age: 20,40,80,...; the doublings from
	// creation sum past 7d within ~10 steps, so the remaining count is finite.
	pol := RetrySchedule{Interval: 20 * time.Minute, MaxAge: 7 * 24 * time.Hour}
	est := EstimateNextAttempt([]*MailRecord{
		rec("Reception", "received", base),
		rec("TransientFailure", "deferred", base.Add(time.Minute)),
	}, pol)
	if !est.Deferred || est.RemainingAttempts < 2 || est.RemainingAttempts > 20 {
		t.Fatalf("expected a small finite remaining count, got %d (%+v)", est.RemainingAttempts, est)
	}
	if est.FinalAttempt.After(est.ExpiresAt) {
		t.Fatalf("final attempt %s must not exceed expiry %s", est.FinalAttempt, est.ExpiresAt)
	}

	// A message whose next attempt already exceeds max age will expire, with no
	// remaining attempts.
	old := base.Add(-8 * 24 * time.Hour)
	expiring := EstimateNextAttempt([]*MailRecord{
		rec("Reception", "received", old),
		rec("TransientFailure", "deferred", base),
	}, pol)
	if !expiring.WillExpire || expiring.RemainingAttempts != 0 {
		t.Fatalf("expected will-expire with 0 remaining, got %+v", expiring)
	}
}

func TestRetryScheduleCap(t *testing.T) {
	pol := RetrySchedule{Interval: 10 * time.Minute, MaxInterval: 30 * time.Minute, MaxAge: time.Hour}
	// 10 → 20 → 30 (capped) → 30 …
	if got := pol.intervalForAttempt(1); got != 10*time.Minute {
		t.Fatalf("attempt 1 = %s", got)
	}
	if got := pol.intervalForAttempt(3); got != 30*time.Minute {
		t.Fatalf("attempt 3 should cap at 30m, got %s", got)
	}
	if got := pol.intervalForAttempt(9); got != 30*time.Minute {
		t.Fatalf("attempt 9 should stay capped at 30m, got %s", got)
	}
}
