package biz

import (
	"context"
	"crypto/subtle"
	"strings"
)

// GreenArrow-compatible mail injection.
//
// This mirrors the JSON contract of GreenArrow's mail-injection API so an
// existing GreenArrow client can be repointed at iris unchanged: it POSTs the
// same body ({username, password, message:{…}}) and reads back {success:1} or
// {success:0, error:"…"}. iris authenticates the body credentials and forwards
// the message to KumoMTA's HTTP injection API (/api/inject/v1).

// GAInjectRequest is the GreenArrow injection request body.
type GAInjectRequest struct {
	Username string    `json:"username"`
	Password string    `json:"password"`
	Message  GAMessage `json:"message"`
}

// GAMessage is the message envelope+content in a GreenArrow request.
type GAMessage struct {
	HTML      string        `json:"html"`
	Text      string        `json:"text"`
	Subject   string        `json:"subject"`
	To        []GARecipient `json:"to"`
	FromEmail string        `json:"from_email"`
	FromName  string        `json:"from_name"`
	Mailclass string        `json:"mailclass"`
	// Headers is GreenArrow's shape: a list of single-entry objects, e.g.
	// [{"X-Feedback-ID": "…"}]. Each entry contributes one header.
	Headers []map[string]string `json:"headers"`
}

// GARecipient is one {email, name} recipient.
type GARecipient struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// GAResponse is the GreenArrow reply. Success is 1 on success, 0 on failure;
// Error carries a human-readable message on failure. Clients check Success == 1.
type GAResponse struct {
	Success int    `json:"success"`
	Error   string `json:"error,omitempty"`
}

// --- KumoMTA /api/inject/v1 payload (builder form) ---

// KumoInjectRequest is the KumoMTA HTTP injection request. Using the builder
// form of `content` lets kumod assemble the MIME; DKIM signing and mailclass
// classification happen in the iris-generated policy's http_message_generated
// hook.
type KumoInjectRequest struct {
	EnvelopeSender string            `json:"envelope_sender"`
	Content        KumoInjectContent `json:"content"`
	Recipients     []KumoInjectRcpt  `json:"recipients"`
}

// KumoInjectContent is the builder-form content block.
type KumoInjectContent struct {
	Subject  string            `json:"subject,omitempty"`
	TextBody string            `json:"text_body,omitempty"`
	HTMLBody string            `json:"html_body,omitempty"`
	From     KumoInjectAddr    `json:"from"`
	To       []KumoInjectAddr  `json:"to,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// KumoInjectAddr is an {email, name} address for the builder.
type KumoInjectAddr struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// KumoInjectRcpt is one envelope recipient.
type KumoInjectRcpt struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// KumoInjector forwards a built message to KumoMTA's injection API.
type KumoInjector interface {
	InjectV1(ctx context.Context, req KumoInjectRequest) error
}

// InjectionCredentialVerifier looks up and records use of the DB-managed
// injection credentials. Satisfied by InjectionCredentialRepo.
type InjectionCredentialVerifier interface {
	ByUsername(ctx context.Context, username string) (*InjectionCredential, error)
	TouchLastUsed(ctx context.Context, id string) error
}

// GreenArrowInjectUsecase authenticates injection requests and forwards them to
// KumoMTA. It performs no RBAC/JWT check — the caller is authenticated purely by
// the body credentials, on a dedicated listener isolated from the admin API.
type GreenArrowInjectUsecase struct {
	injector        KumoInjector
	creds           InjectionCredentialVerifier // DB-managed keys; nil disables them
	username        string
	password        string
	mailClassHeader string
}

// NewGreenArrowInjectUsecase constructs the use case with the configured static
// API credential (the fallback). mailClassHeader is the header kumod classifies
// mailclass from (DefaultMailClassHeader when empty). Attach DB-managed
// credentials with WithCredentialStore.
func NewGreenArrowInjectUsecase(injector KumoInjector, username, password, mailClassHeader string) *GreenArrowInjectUsecase {
	if mailClassHeader == "" {
		mailClassHeader = DefaultMailClassHeader
	}
	return &GreenArrowInjectUsecase{
		injector:        injector,
		username:        username,
		password:        password,
		mailClassHeader: mailClassHeader,
	}
}

// WithCredentialStore attaches the DB-managed credential store. These are
// checked before the static config credential.
func (uc *GreenArrowInjectUsecase) WithCredentialStore(v InjectionCredentialVerifier) *GreenArrowInjectUsecase {
	uc.creds = v
	return uc
}

// Authenticate reports whether the supplied credentials match the STATIC config
// credential, in constant time. Empty configured credentials never authenticate
// (fail closed). DB-managed credentials are checked separately in Inject.
func (uc *GreenArrowInjectUsecase) Authenticate(username, password string) bool {
	if uc.username == "" || uc.password == "" {
		return false
	}
	u := subtle.ConstantTimeCompare([]byte(username), []byte(uc.username))
	p := subtle.ConstantTimeCompare([]byte(password), []byte(uc.password))
	return u == 1 && p == 1
}

// Inject authenticates the request and forwards the message to KumoMTA. It
// returns a domain error (Unauthorized/Forbidden/Invalid/Unavailable) that the
// caller maps to a GreenArrow {success:0,error} reply.
func (uc *GreenArrowInjectUsecase) Inject(ctx context.Context, req *GAInjectRequest) error {
	cred, err := uc.resolve(ctx, req.Username, req.Password)
	if err != nil {
		return err
	}
	// A DB credential may be restricted to specific mailclasses (the static
	// config credential is unrestricted).
	if cred != nil && !cred.AllowsMailclass(req.Message.Mailclass) {
		return Forbidden("INJECT_MAILCLASS_DENIED", "credential %q may not inject mailclass %q",
			cred.Username, strings.TrimSpace(req.Message.Mailclass))
	}
	kreq, err := uc.build(req.Message)
	if err != nil {
		return err
	}
	if err := uc.injector.InjectV1(ctx, kreq); err != nil {
		return err
	}
	if cred != nil {
		if err := uc.creds.TouchLastUsed(ctx, cred.ID); err != nil {
			// Non-fatal: the message was already accepted.
			LoggerFrom(ctx).Warn("inject: touch last_used failed", "id", cred.ID, "error", err.Error())
		}
	}
	return nil
}

// resolve authenticates against the DB-managed credentials first, then the
// static config credential. It returns the matched DB credential (nil when the
// config credential matched) or an Unauthorized error when neither matches.
func (uc *GreenArrowInjectUsecase) resolve(ctx context.Context, username, password string) (*InjectionCredential, error) {
	if uc.creds != nil {
		cred, err := uc.creds.ByUsername(ctx, normalizeUsername(username))
		if err != nil {
			return nil, err
		}
		if cred != nil && cred.Enabled && CheckPassword(cred.PasswordHash, password) {
			return cred, nil
		}
	}
	if uc.Authenticate(username, password) {
		return nil, nil
	}
	return nil, Unauthorized("INJECT_UNAUTHORIZED", "invalid API credentials")
}

// build validates a GreenArrow message and maps it to a KumoMTA inject request.
func (uc *GreenArrowInjectUsecase) build(m GAMessage) (KumoInjectRequest, error) {
	from := strings.TrimSpace(m.FromEmail)
	if from == "" {
		return KumoInjectRequest{}, Invalid("INJECT_FROM_REQUIRED", "from_email is required")
	}
	if strings.TrimSpace(m.Subject) == "" {
		return KumoInjectRequest{}, Invalid("INJECT_SUBJECT_REQUIRED", "subject is required")
	}
	if strings.TrimSpace(m.HTML) == "" && strings.TrimSpace(m.Text) == "" {
		return KumoInjectRequest{}, Invalid("INJECT_BODY_REQUIRED", "html or text body is required")
	}

	recipients := make([]KumoInjectRcpt, 0, len(m.To))
	toHeader := make([]KumoInjectAddr, 0, len(m.To))
	for _, r := range m.To {
		email := strings.TrimSpace(r.Email)
		if email == "" {
			continue
		}
		recipients = append(recipients, KumoInjectRcpt{Email: email, Name: strings.TrimSpace(r.Name)})
		toHeader = append(toHeader, KumoInjectAddr{Email: email, Name: strings.TrimSpace(r.Name)})
	}
	if len(recipients) == 0 {
		return KumoInjectRequest{}, Invalid("INJECT_RECIPIENT_REQUIRED", "at least one recipient is required")
	}

	headers := map[string]string{}
	// mailclass drives kumod's routing/shaping via the classification header.
	if mc := strings.TrimSpace(m.Mailclass); mc != "" {
		headers[uc.mailClassHeader] = mc
	}
	// Flatten GreenArrow's [{name:value}] header list. Later entries win on a
	// duplicate name; the mailclass header is not overridden by a custom header
	// of the same name (operator intent stays authoritative).
	for _, h := range m.Headers {
		for name, value := range h {
			name = strings.TrimSpace(name)
			if name == "" || strings.EqualFold(name, uc.mailClassHeader) {
				continue
			}
			headers[name] = value
		}
	}
	if len(headers) == 0 {
		headers = nil
	}

	return KumoInjectRequest{
		EnvelopeSender: from,
		Recipients:     recipients,
		Content: KumoInjectContent{
			Subject:  m.Subject,
			TextBody: m.Text,
			HTMLBody: m.HTML,
			From:     KumoInjectAddr{Email: from, Name: strings.TrimSpace(m.FromName)},
			To:       toHeader,
			Headers:  headers,
		},
	}, nil
}
