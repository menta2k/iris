package biz

import (
	"net/url"
	"strings"
	"time"
)

// Webhook rule status values.
const (
	WebhookActive   = "active"
	WebhookDisabled = "disabled"
)

// Webhook delivery states.
const (
	WebhookPending   = "pending"
	WebhookDelivered = "delivered"
	WebhookRetrying  = "retrying"
	WebhookFailed    = "failed"
	WebhookCancelled = "cancelled"
)

// RetryPolicy controls webhook redelivery behavior.
type RetryPolicy struct {
	MaxAttempts    int `json:"max_attempts"`
	BackoffSeconds int `json:"backoff_seconds"`
}

// WebhookRule routes matching inbound mail to an HTTP destination.
type WebhookRule struct {
	ID             string
	Name           string
	MatchType      string
	MatchValue     string
	DestinationURL string
	SecretRef      string
	Status         string
	TimeoutSeconds int
	RetryPolicy    RetryPolicy
}

// Validate checks webhook-rule invariants. allowInsecure permits plain HTTP for
// local development; otherwise HTTPS is required.
func (w *WebhookRule) Validate(allowInsecure bool) error {
	w.Name = strings.TrimSpace(w.Name)
	w.MatchValue = strings.ToLower(strings.TrimSpace(w.MatchValue))
	w.DestinationURL = strings.TrimSpace(w.DestinationURL)
	if w.Status == "" {
		w.Status = WebhookActive
	}
	if w.TimeoutSeconds == 0 {
		w.TimeoutSeconds = 10
	}
	if w.RetryPolicy.MaxAttempts == 0 {
		w.RetryPolicy = RetryPolicy{MaxAttempts: 5, BackoffSeconds: 30}
	}

	if w.Name == "" {
		return Invalid("WEBHOOK_NAME_REQUIRED", "webhook name is required")
	}
	if w.MatchType != MatchRecipientEmail && w.MatchType != MatchRecipientDomain {
		return Invalid("WEBHOOK_MATCH_TYPE_INVALID", "match_type %q is not valid", w.MatchType)
	}
	if w.MatchValue == "" {
		return Invalid("WEBHOOK_MATCH_VALUE_REQUIRED", "match_value is required")
	}
	u, err := url.Parse(w.DestinationURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Invalid("WEBHOOK_URL_INVALID", "destination_url %q is not a valid URL", w.DestinationURL)
	}
	if u.Scheme != "https" && !allowInsecure {
		return Invalid("WEBHOOK_URL_INSECURE", "destination_url must use https")
	}
	if w.TimeoutSeconds < 1 || w.TimeoutSeconds > 120 {
		return Invalid("WEBHOOK_TIMEOUT_RANGE", "timeout_seconds must be between 1 and 120")
	}
	if w.RetryPolicy.MaxAttempts < 1 || w.RetryPolicy.MaxAttempts > 20 {
		return Invalid("WEBHOOK_RETRY_RANGE", "max_attempts must be between 1 and 20")
	}
	if strings.Contains(strings.ToUpper(w.SecretRef), "BEGIN") {
		return Invalid("WEBHOOK_SECRET_INLINE", "secret_ref must be a reference, not inline secret material")
	}
	return nil
}

// WebhookDeliveryEvent records a single webhook delivery attempt.
type WebhookDeliveryEvent struct {
	ID            string
	EventTime     time.Time
	WebhookRuleID string
	MailRecordID  string
	Attempt       int
	Status        string
	ResponseCode  int
	ErrorSummary  string
	NextRetryAt   *time.Time
	// WebhookName and Recipient are read-only, populated only on list (joined
	// from webhook_rules / mail_records); ignored on write.
	WebhookName string
	Recipient   string
}
