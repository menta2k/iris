package biz

import (
	"strings"
	"time"
)

// KumoMTA's built-in retry defaults, used when Global Settings leave them blank.
const (
	DefaultRetryInterval = 20 * time.Minute
	DefaultMaxAge        = 7 * 24 * time.Hour
)

// RetrySchedule is the effective exponential-backoff schedule for deferred mail:
// the delay starts at Interval and doubles on each attempt, capped at MaxInterval
// (0 = uncapped), until the message reaches MaxAge and is expired.
type RetrySchedule struct {
	Interval    time.Duration
	MaxInterval time.Duration
	MaxAge      time.Duration
}

func (p RetrySchedule) withDefaults() RetrySchedule {
	if p.Interval <= 0 {
		p.Interval = DefaultRetryInterval
	}
	if p.MaxAge <= 0 {
		p.MaxAge = DefaultMaxAge
	}
	return p
}

// grow doubles an interval, capping at MaxInterval (when set) and at MaxAge (a
// safety bound that also prevents overflow on long-lived messages).
func (p RetrySchedule) grow(interval time.Duration) time.Duration {
	interval *= 2
	if p.MaxInterval > 0 && interval > p.MaxInterval {
		return p.MaxInterval
	}
	if interval > p.MaxAge {
		return p.MaxAge
	}
	return interval
}

// intervalForAttempt returns the delay applied after the nth transient failure
// (n = 1 is the first failure): Interval * 2^(n-1), capped.
func (p RetrySchedule) intervalForAttempt(n int) time.Duration {
	interval := p.Interval
	for i := 1; i < n; i++ {
		interval = p.grow(interval)
	}
	return interval
}

// NextAttemptEstimate is the projected retry schedule for a message, derived from
// its recorded events and the effective RetrySchedule. All times are absolute (UTC).
type NextAttemptEstimate struct {
	Deferred          bool          // currently awaiting retry (no terminal event)
	Attempts          int           // transient failures recorded so far
	LastAttempt       time.Time     // time of the most recent deferral
	NextAttempt       time.Time     // estimated next delivery attempt
	RemainingAttempts int           // future attempts before expiry (incl. the next)
	FinalAttempt      time.Time     // last attempt that fits before expiry
	WillExpire        bool          // the next attempt would exceed MaxAge → expires
	ExpiresAt         time.Time     // creation time + MaxAge
	Interval          time.Duration // delay until the next attempt
}

// EstimateNextAttempt projects the retry schedule for a message from its events
// (any order). It returns Deferred=false when the message already reached a
// terminal outcome (delivered / bounced / expired) or has no recorded deferral.
// The result is an estimate: KumoMTA jitters each interval and traffic shaping
// can override the schedule per destination.
func EstimateNextAttempt(events []*MailRecord, pol RetrySchedule) NextAttemptEstimate {
	pol = pol.withDefaults()

	var created, lastDeferral time.Time
	attempts := 0
	terminal := false
	for _, e := range events {
		if e == nil {
			continue
		}
		t := e.EventTime
		rt := strings.ToLower(strings.TrimSpace(e.RecordType))
		st := strings.ToLower(strings.TrimSpace(e.Status))
		switch {
		case rt == "reception" || st == "received":
			if created.IsZero() || t.Before(created) {
				created = t
			}
		case rt == "transientfailure" || st == "deferred":
			attempts++
			if t.After(lastDeferral) {
				lastDeferral = t
			}
		case rt == "delivery" || st == "sent" || st == "delivered" ||
			rt == "bounce" || st == "bounced" || rt == "expiration":
			terminal = true
		}
	}

	est := NextAttemptEstimate{Attempts: attempts, LastAttempt: lastDeferral}
	if terminal || attempts == 0 || lastDeferral.IsZero() {
		return est // reached a final outcome, or never deferred
	}
	est.Deferred = true

	est.Interval = pol.intervalForAttempt(attempts)
	est.NextAttempt = lastDeferral.Add(est.Interval)

	if !created.IsZero() {
		est.ExpiresAt = created.Add(pol.MaxAge)
		est.WillExpire = est.NextAttempt.After(est.ExpiresAt)
		// Count the remaining attempts that still fit before expiry.
		at := lastDeferral
		interval := est.Interval
		for est.RemainingAttempts < 100000 {
			at = at.Add(interval)
			if at.After(est.ExpiresAt) {
				break
			}
			est.RemainingAttempts++
			est.FinalAttempt = at
			interval = pol.grow(interval)
		}
	}
	return est
}
