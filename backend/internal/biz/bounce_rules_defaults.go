package biz

// DefaultBounceRules returns the curated starter ruleset ("Reset to defaults").
// Priorities: specific hard/policy rules (100) outrank generic transient rules
// (50) so, e.g., a 5.1.1 "user unknown" suppresses even though a broad 5xx rule
// might also match. All are Source=default; operators layer overlay rules on top.
func DefaultBounceRules() []*BounceActionRule {
	r := func(code, enh, provider, pattern, class, category, action, cfg, suggested string, prio int) *BounceActionRule {
		return &BounceActionRule{
			SMTPCode: code, EnhancedCode: enh, Provider: provider, Pattern: pattern,
			Class: class, Category: category, Action: action, ActionConfig: cfg,
			SuggestedAction: suggested, Priority: prio, Source: BounceRuleSourceDefault, Status: "active",
		}
	}
	return []*BounceActionRule{
		// --- Transient / connection (retry) ---
		r("421", "", "", "", "soft", "Connection Issue", BounceActionRetry, "",
			"Retry normally; monitor if frequent.", 50),
		r("451", "4.3.0", "", "", "soft", "Connection Issue", BounceActionRetry, "",
			"Transient local error; retry.", 50),
		r("", "4.4.2", "", "", "soft", "Connection Issue", BounceActionRetry, "",
			"Connection dropped; retry.", 50),

		// --- Rate limiting (throttle) ---
		r("421", "4.7.0", "gmail", "rate", "soft", "Rate Limited (Too Many Requests)", BounceActionThrottle, "receiving/60m",
			"Reduce connection rate and sending speed to Gmail.", 100),
		r("421", "", "yahoo", "rate", "soft", "Rate Limited (Too Many Requests)", BounceActionThrottle, "receiving/60m",
			"Back off; Yahoo is rate-limiting this IP.", 100),
		r("", "4.7.0", "microsoft", "throttl", "soft", "Rate Limited (Too Many Requests)", BounceActionThrottle, "receiving/60m",
			"Reduce rate; Outlook/Microsoft throttling.", 100),
		r("", "", "", "too many", "soft", "Rate Limited (Too Many Requests)", BounceActionThrottle, "receiving/30m",
			"Slow down sending to this destination.", 90),

		// --- Policy / spam block (suspend the domain, do NOT suppress the user) ---
		r("", "5.7.1", "", "", "hard", "Policy / Blocked", BounceActionSuspendDomain, "2h",
			"Blocked by policy; pause and review content/reputation. Do not suppress the recipient.", 100),
		r("550", "", "", "spam", "hard", "Policy / Blocked", BounceActionSuspendDomain, "2h",
			"Flagged as spam; pause delivery to this destination.", 100),
		r("554", "", "", "blocked", "hard", "Policy / Blocked", BounceActionSuspendDomain, "2h",
			"Connection/content blocked; pause and review.", 100),

		// --- Authentication ---
		r("", "5.7.26", "", "", "hard", "Authentication Failed", BounceActionSuspendDomain, "1h",
			"SPF/DKIM/DMARC failed; fix authentication for this domain.", 100),
		r("", "", "", "unauthenticated", "hard", "Authentication Failed", BounceActionSuspendDomain, "1h",
			"Fix SPF/DKIM authentication for this domain.", 90),

		// --- Mailbox full (transient; retry) ---
		r("", "4.2.2", "", "", "soft", "Mailbox Full", BounceActionRetry, "",
			"Mailbox over quota; retry — it often clears.", 80),
		r("452", "", "", "storage", "soft", "Mailbox Full", BounceActionRetry, "",
			"Recipient inbox out of storage; retry.", 80),

		// --- Invalid recipient (hard; suppress) ---
		r("550", "5.1.1", "", "", "hard", "Invalid Recipient", BounceActionSuppress, "",
			"Recipient does not exist; suppress the address.", 100),
		r("", "5.1.1", "", "user unknown", "hard", "Invalid Recipient", BounceActionSuppress, "",
			"User unknown; suppress the address.", 100),
		r("", "5.1.10", "", "", "hard", "Invalid Recipient", BounceActionSuppress, "",
			"Address does not exist (NULL MX); suppress.", 100),
	}
}
