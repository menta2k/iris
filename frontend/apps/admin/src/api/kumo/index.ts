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

export const usersApi = {
  list: (params?: ListParams) =>
    requestClient.get<ListResponse<User>>('/v1/users', { params }),
  create: (input: UserInput) => requestClient.post<User>('/v1/users', input),
  remove: (id: number) => requestClient.delete(`/v1/users/${id}`),
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
  algorithm: 'ed25519' | 'rsa-2048' | 'rsa-4096';
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

// ─── Listener (single-record) ────────────────────────────────────────────────
export interface ListenerConfig {
  trusted_hosts: string[];
  relay_hosts: string[];
}

export const listenerApi = {
  get: () => requestClient.get<ListenerConfig>('/v1/listener'),
  update: (cfg: ListenerConfig) =>
    requestClient.put<ListenerConfig>('/v1/listener', cfg),
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
