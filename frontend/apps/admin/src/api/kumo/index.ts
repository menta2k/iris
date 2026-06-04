import { requestClient } from '#/utils/request';

interface ListResponse<T> {
  items: T[];
  total?: number;
}

interface ListParams {
  filter?: string;
  limit?: number;
  offset?: number;
}

// ─── Queues ─────────────────────────────────────────────────────────────────
export interface QueueItem {
  name: string;
  queue_size: number;
  delivered: number;
  failed: number;
  deferred?: number;
  suspended: boolean;
}

export interface ScheduledMessage {
  id: string;
  sender?: string;
  recipient?: string;
  due_at?: string;
  num_attempts: number;
  tenant?: string;
  campaign?: string;
  meta?: Record<string, unknown>;
}

export interface ScheduledMessagesResponse {
  queue_name: string;
  items: ScheduledMessage[];
}

export const queuesApi = {
  list: (params?: ListParams) =>
    requestClient.get<ListResponse<QueueItem>>('/v1/queues', { params }),
  inspect: (name: string, limit = 50) =>
    requestClient.get<ScheduledMessagesResponse>(
      `/v1/queues/${encodeURIComponent(name)}/messages`,
      { params: { limit } },
    ),
  suspend: (name: string) =>
    requestClient.post(`/v1/queues/${encodeURIComponent(name)}/suspend`),
  resume: (name: string) =>
    requestClient.post(`/v1/queues/${encodeURIComponent(name)}/resume`),
  bounce: (name: string) =>
    requestClient.post(`/v1/queues/${encodeURIComponent(name)}/bounce`),
};

// ─── Suppressions ────────────────────────────────────────────────────────────
export interface Suppression {
  id: string;
  address: string;
  scope: string;
  reason: string;
  created_at: string;
}

export interface SuppressionInput {
  address: string;
  scope: string;
  reason?: string;
}

export const suppressionsApi = {
  list: (params?: ListParams) =>
    requestClient.get<ListResponse<Suppression>>('/v1/suppressions', { params }),
  create: (input: SuppressionInput) =>
    requestClient.post<Suppression>('/v1/suppressions', input),
  remove: (id: string) =>
    requestClient.delete(`/v1/suppressions/${encodeURIComponent(id)}`),
};

// ─── Users ───────────────────────────────────────────────────────────────────
export interface User {
  id: number;
  username: string;
  email: string;
  active: boolean;
  roles: string[];
  last_login_at?: string;
}

export interface UserInput {
  username: string;
  email: string;
  password?: string;
  roles: string[];
}

export interface ChangePasswordInput {
  old_password?: string;
  new_password: string;
}

export const usersApi = {
  list: (params?: ListParams) =>
    requestClient.get<ListResponse<User>>('/v1/users', { params }),
  create: (input: UserInput) => requestClient.post<User>('/v1/users', input),
  remove: (id: number) => requestClient.delete(`/v1/users/${id}`),
  changePassword: (id: number, input: ChangePasswordInput) =>
    requestClient.post<void>(`/v1/users/${id}/password`, input),
  // Admin: clear another account's MFA (lost device recovery).
  resetMfa: (id: number) =>
    requestClient.post<void>(`/v1/users/${id}/mfa/reset`, {}),
};

// ─── MFA (self-service second factor) ────────────────────────────────────────
export interface MFAPasskey {
  id: number;
  label: string;
  created_at: string;
}

export interface MFAStatus {
  totp_enabled: boolean;
  webauthn_enabled: boolean;
  backup_remaining: number;
  passkeys: MFAPasskey[];
}

export interface TOTPEnrollStart {
  secret: string;
  otpauth_url: string;
  qr_code_data_uri: string;
  operation_id: string;
}

export const mfaApi = {
  status: () => requestClient.get<MFAStatus>('/v1/auth/mfa'),
  totpEnroll: () =>
    requestClient.post<TOTPEnrollStart>('/v1/auth/mfa/totp/enroll', {}),
  totpConfirm: (operationId: string, code: string) =>
    requestClient.post<{ backup_codes: string[] }>('/v1/auth/mfa/totp/confirm', {
      operation_id: operationId,
      code,
    }),
  regenerateBackupCodes: () =>
    requestClient.post<{ backup_codes: string[] }>(
      '/v1/auth/mfa/backup-codes/regenerate',
      {},
    ),
  disable: () => requestClient.post<void>('/v1/auth/mfa/disable', {}),
  // Passkey enrollment (Bearer-authed).
  passkeyEnrollStart: () =>
    requestClient.post<{ options: unknown; operation_id: string }>(
      '/v1/auth/mfa/passkey/enroll/start',
      {},
    ),
  passkeyEnrollFinish: (operationId: string, response: unknown, label: string) =>
    requestClient.post<void>('/v1/auth/mfa/passkey/enroll/finish', {
      operation_id: operationId,
      response,
      label,
    }),
  removePasskey: (id: number) =>
    requestClient.delete(`/v1/auth/mfa/passkey/${id}`),
  // Login step (uses the mfa_token from /v1/auth/login; no access token yet).
  verify: (mfaToken: string, body: { code?: string; backup_code?: string }) =>
    requestClient.post('/v1/auth/mfa/verify', { mfa_token: mfaToken, ...body }),
  passkeyLoginStart: (mfaToken: string) =>
    requestClient.post<{ options: unknown; operation_id: string }>(
      '/v1/auth/mfa/passkey/login/start',
      { mfa_token: mfaToken },
    ),
  passkeyLoginFinish: (operationId: string, response: unknown) =>
    requestClient.post('/v1/auth/mfa/passkey/login/finish', {
      operation_id: operationId,
      response,
    }),
};

// ─── Audit ───────────────────────────────────────────────────────────────────
export interface AuditEntry {
  id: string;
  at: string;
  operation: string;
  actor_username: string;
  resource_type: string;
  resource_id: string;
  client_ip: string;
  status_code: number;
  duration_ms: number;
}

export const auditApi = {
  list: (params?: ListParams) =>
    requestClient.get<ListResponse<AuditEntry>>('/v1/audit', { params }),
};

// ─── Logs ────────────────────────────────────────────────────────────────────
export interface LogEntry {
  id: string;
  at: string;
  event_type: string;
  recipient: string;
  sender: string;
  response_code: number;
  response_text: string;
  mail_class?: string;
  queue?: string;
  message_id?: string;
  source_ip?: string;
  vmta?: string;
}

export interface LogsListParams extends ListParams {
  event_type?: string;
  sender?: string;
  recipient?: string;
  mail_class?: string;
  // RFC3339 timestamps; backend tolerates malformed values silently.
  since?: string;
  until?: string;
}

export const logsApi = {
  list: (params?: LogsListParams) =>
    requestClient.get<ListResponse<LogEntry>>('/v1/logs', { params }),
};

// ─── DSNs (async bounces) ────────────────────────────────────────────────────
export interface DsnEntry {
  id: string;
  received_at: string;
  verp_token?: string;
  message_id_ref?: string;
  original_recipient?: string;
  final_recipient?: string;
  action?: string;
  status?: string;
  status_class?: string;
  diagnostic_code?: string;
  remote_mta?: string;
  category?: string;
  mail_class?: string;
  tenant?: string;
  campaign?: string;
  raw_size?: number;
  extra_json?: string;
}

export interface DsnsListParams extends ListParams {
  category?: string;
  status_class?: string;
  status?: string;
  recipient?: string;
  mail_class?: string;
  message_id?: string;
  since?: string;
  until?: string;
}

export const dsnsApi = {
  list: (params?: DsnsListParams) =>
    requestClient.get<ListResponse<DsnEntry>>('/v1/dsns', { params }),
};

// ─── Global Settings ─────────────────────────────────────────────────────────
export interface GlobalSettings {
  kumo_http_listen: string;
  esmtp_listen_addr: string;
  esmtp_relay_hosts: string[];
  http_trusted_hosts: string[];
  bounce_domain: string;
  bounce_sender_domains: string[];
  bounce_prefix: string;
  mail_class_header: string;
  egress_ehlo_domain: string;
  egress_retry_interval: string;
  egress_max_retry_interval: string;
  egress_max_age: string;
  https_listen: string;
  https_cert_pem_path: string;
  https_key_pem_path: string;
  updated_at?: string;
  updated_by?: string;
}

export const globalSettingsApi = {
  get: () => requestClient.get<GlobalSettings>('/v1/global-settings'),
  update: (input: GlobalSettings) =>
    requestClient.put<GlobalSettings>('/v1/global-settings', input),
};

// ─── ACME ────────────────────────────────────────────────────────────────────
export interface AcmeAccount {
  email: string;
  server_url: string;
  has_registration: boolean;
  updated_at?: string;
}

export interface AcmeProviderInfo {
  name: string;
  description: string;
  required_fields: string[];
  optional_fields: string[];
}

// Response shape — the secret `config` values are NEVER returned by the API.
// `configured_keys` lists which credential fields currently hold a value so
// the UI can show "saved" without exposing the secret.
export interface AcmeDnsProviderConfig {
  provider: string;
  configured_keys?: string[];
  updated_at?: string;
  updated_by?: string;
}

// Write-only request body. Only non-empty fields are sent; the backend
// merges them over the stored config (blank = keep existing).
export interface AcmeDnsProviderConfigInput {
  provider: string;
  config: Record<string, string>;
}

export interface AcmeCertificate {
  id: number;
  domain: string;
  alt_names: string[];
  challenge_type: 'dns-01' | 'http-01';
  dns_provider?: string;
  cert_pem_path?: string;
  key_pem_path?: string;
  expires_at?: string;
  last_renewed_at?: string;
  status: 'failed' | 'issued' | 'pending' | 'renewing';
  last_error?: string;
  created_at?: string;
  updated_at?: string;
}

export interface AcmeIssueRequest {
  domain: string;
  alt_names?: string[];
  challenge_type: 'dns-01' | 'http-01';
  dns_provider?: string;
}

export const acmeApi = {
  // Account is a singleton — GET always returns one row (possibly with
  // empty fields when never configured).
  getAccount: () => requestClient.get<AcmeAccount>('/v1/acme/account'),
  saveAccount: (in_: { email: string; server_url: string }) =>
    requestClient.put<AcmeAccount>('/v1/acme/account', in_),

  // Registry: read-only metadata for every supported DNS provider; the
  // DNS Providers page uses this to render a dynamic credentials form
  // keyed by required_fields + optional_fields.
  listRegistry: () =>
    requestClient.get<{ items: AcmeProviderInfo[] }>(
      '/v1/acme/dns-providers/registry',
    ),

  listDnsProviderConfigs: () =>
    requestClient.get<{ items: AcmeDnsProviderConfig[] }>(
      '/v1/acme/dns-providers',
    ),
  saveDnsProviderConfig: (in_: AcmeDnsProviderConfigInput) =>
    requestClient.put<AcmeDnsProviderConfig>('/v1/acme/dns-providers', in_),
  removeDnsProviderConfig: (provider: string) =>
    requestClient.delete(
      `/v1/acme/dns-providers/${encodeURIComponent(provider)}`,
    ),

  listCertificates: () =>
    requestClient.get<{ items: AcmeCertificate[] }>('/v1/acme/certificates'),
  getCertificate: (id: number) =>
    requestClient.get<AcmeCertificate>(`/v1/acme/certificates/${id}`),
  issueCertificate: (in_: AcmeIssueRequest) =>
    requestClient.post<AcmeCertificate>('/v1/acme/certificates', in_),
  renewCertificate: (id: number) =>
    requestClient.post<AcmeCertificate>(`/v1/acme/certificates/${id}/renew`),
  removeCertificate: (id: number) =>
    requestClient.delete(`/v1/acme/certificates/${id}`),
};

// ─── Feedback ────────────────────────────────────────────────────────────────
export interface FeedbackReport {
  id: string;
  received_at: string;
  feedback_type: string;
  original_recipient: string;
  reporting_mta: string;
  raw?: string;
}

export const feedbackApi = {
  list: (params?: ListParams) =>
    requestClient.get<ListResponse<FeedbackReport>>('/v1/feedback', { params }),
};

// ─── DKIM ────────────────────────────────────────────────────────────────────
export interface DkimIdentity {
  id: string;
  domain: string;
  selector: string;
  algorithm: 'rsa' | 'ed25519';
  active: boolean;
  public_key_pem?: string;
}

export interface DkimInput {
  domain: string;
  selector: string;
  algorithm: 'ed25519' | 'rsa-1024' | 'rsa-2048' | 'rsa-4096';
  // Optional. When set, the backend imports the supplied PEM instead of
  // generating a fresh keypair. The public key is derived server-side.
  private_key_pem?: string;
}

export const dkimApi = {
  list: () => requestClient.get<ListResponse<DkimIdentity>>('/v1/dkim'),
  create: (input: DkimInput) =>
    requestClient.post<DkimIdentity>('/v1/dkim', input),
  rotate: (id: string) =>
    requestClient.post<DkimIdentity>(`/v1/dkim/${encodeURIComponent(id)}/rotate`),
  remove: (id: string) =>
    requestClient.delete(`/v1/dkim/${encodeURIComponent(id)}`),
};

// ─── Virtual MTAs ────────────────────────────────────────────────────────────
export interface Vmta {
  id: string;
  name: string;
  source_ips: string[];
  helo_name: string;
  max_connections: number;
  provider_profile?: string;
}

export interface VmtaInput {
  name: string;
  source_ips: string[];
  helo_name: string;
  max_connections: number;
  provider_profile?: string;
}

export const vmtasApi = {
  list: () => requestClient.get<ListResponse<Vmta>>('/v1/vmtas'),
  create: (input: VmtaInput) => requestClient.post<Vmta>('/v1/vmtas', input),
  remove: (id: string) =>
    requestClient.delete(`/v1/vmtas/${encodeURIComponent(id)}`),
};

// ─── VMTA Groups ─────────────────────────────────────────────────────────────
export interface VmtaGroupMember {
  vmta_id: number;
  vmta_name?: string;
  weight: number;
  priority: number;
  enabled: boolean;
}

export interface VmtaGroup {
  id: number;
  name: string;
  description?: string;
  enabled: boolean;
  members?: VmtaGroupMember[];
}

export interface VmtaGroupInput {
  name: string;
  description?: string;
  enabled: boolean;
}

export const vmtaGroupsApi = {
  list: () => requestClient.get<ListResponse<VmtaGroup>>('/v1/vmta-groups'),
  get: (id: number) => requestClient.get<VmtaGroup>(`/v1/vmta-groups/${id}`),
  create: (input: VmtaGroupInput) =>
    requestClient.post<VmtaGroup>('/v1/vmta-groups', input),
  update: (id: number, input: VmtaGroupInput) =>
    requestClient.request<VmtaGroup>(`/v1/vmta-groups/${id}`, {
      method: 'PUT',
      data: input,
    }),
  remove: (id: number) =>
    requestClient.delete(`/v1/vmta-groups/${id}`),
  setMembers: (id: number, members: VmtaGroupMember[]) =>
    requestClient.request<VmtaGroup>(`/v1/vmta-groups/${id}/members`, {
      method: 'PUT',
      data: { members },
    }),
};

// ─── Routing ─────────────────────────────────────────────────────────────────
export interface RuleCondition {
  field: string;
  op: string;
  value: string;
}

export interface RuleTarget {
  kind: 'discard' | 'queue' | 'reject' | 'vmta' | 'vmta_group';
  ref?: string;
  reject_code?: number;
  reject_text?: string;
}

export interface RoutingRule {
  id: string;
  name: string;
  priority: number;
  enabled: boolean;
  conditions: RuleCondition[];
  target: RuleTarget;
}

export interface RoutingRuleInput {
  name: string;
  priority: number;
  enabled: boolean;
  conditions: RuleCondition[];
  target: RuleTarget;
}

export const routingApi = {
  list: () => requestClient.get<ListResponse<RoutingRule>>('/v1/routing'),
  create: (input: RoutingRuleInput) =>
    requestClient.post<RoutingRule>('/v1/routing', input),
  update: (id: string, patch: Partial<RoutingRule>) =>
    requestClient.request<RoutingRule>(
      `/v1/routing/${encodeURIComponent(id)}`,
      { method: 'PATCH', data: patch },
    ),
  remove: (id: string) =>
    requestClient.delete(`/v1/routing/${encodeURIComponent(id)}`),
};

// ─── Policy ──────────────────────────────────────────────────────────────────
// The backend returns `{lua, sha256}`. `active` reads init.lua off disk —
// what kumomta is actually running — while `render` produces a preview
// from the current DB snapshot (the two diverge after edits, before apply).
export interface PolicyRender {
  lua: string;
  sha256: string;
}

export interface PolicyValidation {
  valid: boolean;
  issues?: string[];
}

export interface PolicyApplyResp {
  sha256: string;
  applied_at: string;
}

export const policyApi = {
  active: () => requestClient.get<PolicyRender>('/v1/policy/active'),
  render: () => requestClient.get<PolicyRender>('/v1/policy/render'),
  validate: () => requestClient.get<PolicyValidation>('/v1/policy/validate'),
  apply: (note?: string) =>
    requestClient.post<PolicyApplyResp>('/v1/policy/apply', { note: note ?? '' }),
};

// ─── ESMTP listeners (multi) ─────────────────────────────────────────────────
export interface Listener {
  id?: number;
  name: string;
  listen_addr: string;
  hostname: string;
  tls_enabled: boolean;
  tls_cert_pem_path?: string;
  tls_key_pem_path?: string;
  require_auth: boolean;
  max_message_size?: number;
  created_at?: string;
  updated_at?: string;
}

export const listenersApi = {
  list: () => requestClient.get<ListResponse<Listener>>('/v1/listeners'),
  create: (input: Listener) =>
    requestClient.post<Listener>('/v1/listeners', input),
  update: (id: number, input: Listener) =>
    requestClient.put<Listener>(`/v1/listeners/${id}`, input),
  remove: (id: number) => requestClient.delete(`/v1/listeners/${id}`),
};

// ─── Mail webhooks (inbound mail → HTTP endpoint) ────────────────────────────
export interface MailWebhook {
  id?: number;
  name: string;
  address: string; // exact recipient (a@host) or bare domain (host) catch-all
  url: string;
  secret?: string; // write-only; responses report secret_set instead
  secret_set?: boolean;
  enabled?: boolean;
  created_at?: string;
  updated_at?: string;
}

export const mailWebhooksApi = {
  list: () => requestClient.get<ListResponse<MailWebhook>>('/v1/mail-webhooks'),
  create: (input: MailWebhook) =>
    requestClient.post<MailWebhook>('/v1/mail-webhooks', input),
  update: (id: number, input: MailWebhook) =>
    requestClient.put<MailWebhook>(`/v1/mail-webhooks/${id}`, input),
  remove: (id: number) => requestClient.delete(`/v1/mail-webhooks/${id}`),
};

// ─── Listener Domains ───────────────────────────────────────────────────────
export interface ListenerDomain {
  id: string;
  domain: string;
  relay_to?: string;
  enabled: boolean;
}

export const listenerDomainsApi = {
  list: () =>
    requestClient.get<ListResponse<ListenerDomain>>('/v1/listener/domains'),
  create: (input: Omit<ListenerDomain, 'id'>) =>
    requestClient.post<ListenerDomain>('/v1/listener/domains', input),
  remove: (id: string) =>
    requestClient.delete(`/v1/listener/domains/${encodeURIComponent(id)}`),
};

// ─── Mail Classes ────────────────────────────────────────────────────────────
// MailClass is a header-driven router: when an inbound message has the
// configured global header (X-Kumo-Mail-Class by default) with a value
// matching a class name, the message is routed to that class's target.
export interface MailClass {
  id: number;
  name: string;
  description?: string;
  enabled: boolean;
  target_kind: 'vmta' | 'vmta_group';
  target_ref: string;
}

export interface MailClassInput {
  name: string;
  description?: string;
  enabled: boolean;
  target_kind: 'vmta' | 'vmta_group';
  target_ref: string;
}

export const mailClassesApi = {
  list: () => requestClient.get<ListResponse<MailClass>>('/v1/mail-classes'),
  create: (input: MailClassInput) =>
    requestClient.post<MailClass>('/v1/mail-classes', input),
  update: (id: number, input: MailClassInput) =>
    requestClient.request<MailClass>(`/v1/mail-classes/${id}`, {
      method: 'PUT',
      data: input,
    }),
  remove: (id: number) =>
    requestClient.delete(`/v1/mail-classes/${id}`),
};

// ─── Bounces ─────────────────────────────────────────────────────────────────
export interface Bounce {
  id: string;
  domain?: string;
  tenant?: string;
  campaign?: string;
  duration_seconds: number;
  expires_at: string;
}

export const bouncesApi = {
  list: () => requestClient.get<ListResponse<Bounce>>('/v1/bounces'),
  create: (input: Omit<Bounce, 'id' | 'expires_at'>) =>
    requestClient.post<Bounce>('/v1/bounces', input),
  remove: (id: string) =>
    requestClient.delete(`/v1/bounces/${encodeURIComponent(id)}`),
};

// ─── Dashboard ───────────────────────────────────────────────────────────────
// Backed by the Prometheus instance the admin-service queries on the
// operator's behalf — so the SPA doesn't need direct Prometheus access
// or PromQL knowledge. Returns 503 with code=METRICS_NOT_CONFIGURED
// when IRIS_PROMETHEUS_URL is unset; the page handles that and shows a
// "metrics not configured" placeholder.

export interface DashboardSummary {
  events_24h: Record<string, number>;
  delivery_rate_24h: number;
  bounce_rate_24h: number;
  stream_pending: number;
  suppression_entries: Record<string, number>; // scope → count
  policy_applies_24h: Record<string, number>; // result → count
  generated_at: string;
}

export interface DashboardEventRatesPoint {
  at: string;
  value: number; // events per second over a 5-minute trailing window
}

export interface DashboardEventRatesSeries {
  event_type: string;
  points: DashboardEventRatesPoint[];
}

export interface DashboardEventRates {
  range_seconds: number;
  step_seconds: number;
  series: DashboardEventRatesSeries[];
}

export interface DashboardClassRow {
  mail_class: string; // empty = unclassified
  events_24h: number;
  delivery_rate: number;
}

export interface DashboardByClass {
  classes: DashboardClassRow[];
}

export const dashboardApi = {
  summary: () =>
    requestClient.get<DashboardSummary>('/v1/dashboard/summary'),
  /**
   * @param range  Go duration string (e.g. "1h", "24h"); clamped server-side.
   * @param step   Go duration string (e.g. "30s", "5m"); clamped server-side.
   */
  eventRates: (range = '1h', step = '1m') =>
    requestClient.get<DashboardEventRates>(
      `/v1/dashboard/event-rates?range=${encodeURIComponent(range)}&step=${encodeURIComponent(step)}`,
    ),
  byClass: () => requestClient.get<DashboardByClass>('/v1/dashboard/by-class'),
};

// ─── Login Firewall (login policies) ─────────────────────────────────────────
export type LoginPolicyType = 'BLACKLIST' | 'WHITELIST';
// IP / REGION / TIME are enforced; MAC / DEVICE exist in the backend enum for
// forward-compat but are rejected on create, so the UI never offers them.
export type LoginPolicyMethod = 'IP' | 'REGION' | 'TIME';

// Weekdays follow Go's time.Weekday: 0 = Sunday .. 6 = Saturday.
export interface LoginPolicyTimeWindow {
  days: number[];
  start: string; // "HH:MM"
  end: string; // "HH:MM"
  timezone: string; // IANA, empty = UTC
}

export interface LoginPolicy {
  id: number;
  targetId?: number; // 0 / absent = global
  type: LoginPolicyType;
  method: LoginPolicyMethod | string;
  value?: string; // CIDR/IP (IP) or ISO country code (REGION)
  timeWindow?: LoginPolicyTimeWindow;
  reason?: string;
  enabled?: boolean;
  createdBy?: number;
  updatedBy?: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface LoginPolicyInput {
  targetId?: number;
  type: LoginPolicyType;
  method: LoginPolicyMethod;
  value?: string;
  timeWindow?: LoginPolicyTimeWindow;
  reason?: string;
  enabled?: boolean;
}

export const loginPoliciesApi = {
  list: (params?: ListParams) =>
    requestClient.get<ListResponse<LoginPolicy>>('/v1/login-policies', {
      params,
    }),
  // acknowledge=true bypasses the backend self-lockout guard (409
  // WOULD_LOCK_OUT_SELF) after the operator confirms.
  create: (input: LoginPolicyInput, acknowledge = false) =>
    requestClient.post<LoginPolicy>(
      `/v1/login-policies${acknowledge ? '?acknowledge=true' : ''}`,
      input,
    ),
  update: (id: number, input: LoginPolicyInput, acknowledge = false) =>
    requestClient.request<LoginPolicy>(
      `/v1/login-policies/${id}${acknowledge ? '?acknowledge=true' : ''}`,
      { method: 'PUT', data: input },
    ),
  remove: (id: number) => requestClient.delete(`/v1/login-policies/${id}`),
};
