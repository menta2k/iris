// asserts.go — turn the YAML AssertionBlock into pass/fail per assertion.
//
// Counts use a tiny "op N" mini-language: `>= 5`, `== 0`, `< 100`. Anything
// else is a parse error reported up-front so a typo doesn't silently pass.
package main

import (
	"fmt"
	"strconv"
	"strings"
)

// AssertResult is one yes/no with the human-readable comparison line.
type AssertResult struct {
	Label    string
	Expected string
	Actual   int
	Passed   bool
}

func (r AssertResult) String() string {
	mark := "FAIL"
	if r.Passed {
		mark = "ok  "
	}
	return fmt.Sprintf("[%s] %-40s actual=%-6d expected=%s", mark, r.Label, r.Actual, r.Expected)
}

// EvaluateAsserts compares observed counts against the scenario's expectations.
func EvaluateAsserts(a AssertionBlock, logCounts map[string]int, fbTotal int, suppByReason map[string]int) []AssertResult {
	out := []AssertResult{}
	for evt, expr := range a.LogEvent {
		actual := logCounts[evt]
		ok, err := compare(actual, expr)
		out = append(out, AssertResult{
			Label:    "log_event[" + evt + "]",
			Expected: expr,
			Actual:   actual,
			Passed:   ok && err == nil,
		})
	}
	if a.FeedbackReports != "" {
		ok, _ := compare(fbTotal, a.FeedbackReports)
		out = append(out, AssertResult{
			Label:    "feedback_reports",
			Expected: a.FeedbackReports,
			Actual:   fbTotal,
			Passed:   ok,
		})
	}
	for reason, expr := range a.SuppressionEntries {
		actual := suppByReason[reason]
		ok, _ := compare(actual, expr)
		out = append(out, AssertResult{
			Label:    "suppression_entries[" + reason + "]",
			Expected: expr,
			Actual:   actual,
			Passed:   ok,
		})
	}
	return out
}

// compare parses "op N" and evaluates against actual. Defaults to ">=" if
// the expression is just a bare integer (most-common operator for the
// "we should see at least N events" pattern).
func compare(actual int, expr string) (bool, error) {
	expr = strings.TrimSpace(expr)
	for _, op := range []string{">=", "<=", "==", ">", "<"} {
		if strings.HasPrefix(expr, op) {
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(expr, op)))
			if err != nil {
				return false, fmt.Errorf("parse %q: %w", expr, err)
			}
			switch op {
			case ">=":
				return actual >= n, nil
			case "<=":
				return actual <= n, nil
			case "==":
				return actual == n, nil
			case ">":
				return actual > n, nil
			case "<":
				return actual < n, nil
			}
		}
	}
	if n, err := strconv.Atoi(expr); err == nil {
		return actual >= n, nil
	}
	return false, fmt.Errorf("unrecognised comparison %q", expr)
}
