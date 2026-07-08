// Shared API types matching the Iris KumoMTA admin backend contract.

export interface PageInfo {
  // Responses are proto-JSON (camelCase). Kept the snake_case alias for any
  // older reader.
  nextPageToken?: string
  next_page_token?: string
}

export interface ListResponse<T> {
  items?: T[]
  page?: PageInfo
}

// ---- Outbound config ----

export type ListenerStatus = 'active' | 'disabled' | string

// Response type: proto-JSON returns camelCase field names. A listener owns the
// IP + port + EHLO hostname + TLS/relay config that VMTAs attach to.
export interface Listener {
  id: string
  name: string
  ipAddress: string
  port: number
  hostname: string
  tlsEnabled: boolean
  tlsCertPath: string
  tlsKeyPath: string
  requireAuth: boolean
  // int64 field: serialized as a JSON string via proto-JSON.
  maxMessageSize: string
  relayHosts: string[]
  status: ListenerStatus
  // "inbound" (MX) | "submission".
  role: ListenerRole
}

export type ListenerRole = 'inbound' | 'submission'

// Request body: the HTTP transcoder accepts proto field (snake_case) names.
export interface CreateListenerRequest {
  name: string
  ip_address: string
  port: number
  hostname: string
  tls_enabled: boolean
  tls_cert_path: string
  tls_key_path: string
  require_auth: boolean
  // int64 field: sent as a JSON string.
  max_message_size: string
  relay_hosts: string[]
  role: ListenerRole
}

// Update body adds the editable status field.
export interface UpdateListenerRequest extends CreateListenerRequest {
  status: string
}

export type VMTAStatus = 'STATUS_UNSPECIFIED' | 'ACTIVE' | 'DISABLED' | 'DRAINING' | string

// Response type: proto-JSON returns camelCase field names. ipAddress, ehloName
// and listenerName are READ-ONLY, resolved from the attached listener.
export interface VMTA {
  id: string
  name: string
  status: VMTAStatus
  notes?: string
  listenerId: string
  listenerName: string
  ipAddress: string
  ehloName: string
  maxConnections: number
}

// Request body: a VMTA owns its egress ip/ehlo (3.0.0); listener_id is optional.
export interface CreateVMTARequest {
  name: string
  ip_address: string
  ehlo_name: string
  listener_id?: string
  max_connections: number
}

// Update body: status and notes become editable on edit.
export interface UpdateVMTARequest {
  name: string
  ip_address: string
  ehlo_name: string
  listener_id?: string
  max_connections: number
  status: string
  notes: string
}

// ---- IP warmup ----
export type WarmupStatus = 'scheduled' | 'active' | 'paused' | 'completed'

export interface WarmupStage {
  dayFrom: number
  dayTo: number
  caps: Record<string, number> // MBP bucket -> messages/day
}

export interface WarmupSchedule {
  id: string
  vmtaId: string
  vmtaName: string
  startDate: string // YYYY-MM-DD
  curve: string
  stages: WarmupStage[]
  status: WarmupStatus
  pausedReason?: string
  heldDay?: number
  createdAt?: string
  updatedAt?: string
}

export interface WarmupCurve {
  name: string
  stages: WarmupStage[]
}

export interface WarmupListResponse {
  items?: WarmupSchedule[]
  curves?: WarmupCurve[]
  page?: { nextPageToken?: string }
}

// Request-shaped stage (snake_case) for a custom curve.
export interface WarmupStageInput {
  day_from: number
  day_to: number
  caps: Record<string, number>
}

export interface CreateWarmupScheduleRequest {
  vmta_id: string
  start_date: string
  curve: string
  stages?: WarmupStageInput[] // required when curve = 'custom'
}

export interface UpdateWarmupScheduleRequest {
  start_date: string
  curve: string
  stages?: WarmupStageInput[]
}

export interface PauseWarmupScheduleRequest {
  reason: string
}

// ---- Delivery blueprints (base shaping) ----
export type BlueprintStatus = 'active' | 'disabled'

export interface DeliveryBlueprint {
  id: string
  provider: string
  mxPattern: string
  connRate: string
  deliveriesPerConn: number
  connLimit: number
  dailyCap: number
  status: BlueprintStatus
  createdAt?: string
  updatedAt?: string
}

export interface CreateDeliveryBlueprintRequest {
  provider: string
  mx_pattern: string
  conn_rate: string
  deliveries_per_conn: number
  conn_limit: number
  daily_cap: number
}

export interface UpdateDeliveryBlueprintRequest {
  provider: string
  mx_pattern: string
  conn_rate: string
  deliveries_per_conn: number
  conn_limit: number
  daily_cap: number
  status: string
}

export interface SeedDeliveryBlueprintsResponse {
  inserted?: number
}

// ---- TSA automation rules ----
export type AutomationAction = 'suspend' | 'suspend_tenant' | 'set_config'

export interface AutomationRule {
  id: string
  domain: string
  regex: string
  action: AutomationAction
  configName?: string
  configValue?: string
  trigger: string
  duration: string
  status: BlueprintStatus
  createdAt?: string
  updatedAt?: string
}

export interface CreateAutomationRuleRequest {
  domain: string
  regex: string
  action: string
  config_name: string
  config_value: string
  trigger: string
  duration: string
}

export interface UpdateAutomationRuleRequest extends CreateAutomationRuleRequest {
  status: string
}

// ---- Bounce-based actions ----

export type BounceAction = 'retry' | 'throttle' | 'suspend_domain' | 'suppress'
export type BounceClass = 'soft' | 'hard'

export interface BounceRule {
  id: string
  smtpCode: string
  enhancedCode: string
  provider: string
  pattern: string
  class: BounceClass
  category: string
  action: BounceAction
  actionConfig: string
  suggestedAction: string
  priority: number
  minAttempts: number
  suppressTtl: string
  source: 'default' | 'overlay'
  status: 'active' | 'disabled'
  createdAt?: string
  updatedAt?: string
}

export interface CreateBounceRuleRequest {
  smtp_code: string
  enhanced_code: string
  provider: string
  pattern: string
  class: string
  category: string
  action: string
  action_config: string
  suggested_action: string
  priority: number
  min_attempts: number
  suppress_ttl: string
}

export interface UpdateBounceRuleRequest extends CreateBounceRuleRequest {
  status: string
}

// ---- Event Processor ----

export type EventProcessorEventType =
  | 'bounce'
  | 'suppression_created'
  | 'feedback_report'
  | 'dmarc_received'

export interface EventProcessor {
  id: string
  name: string
  eventTypes: string[]
  mailclasses: string[]
  driver: string
  driverConfig: Record<string, string>
  mode: string
  batchMaxSize: number
  batchMaxWait: string
  status: 'active' | 'disabled'
  createdAt?: string
  updatedAt?: string
}

export interface CreateEventProcessorRequest {
  name: string
  event_types: string[]
  mailclasses: string[]
  driver: string
  driver_config: Record<string, string>
  mode: string
  batch_max_size: number
  batch_max_wait: string
}

export interface UpdateEventProcessorRequest extends CreateEventProcessorRequest {
  status: string
}

export interface TestEventProcessorResult {
  ok: boolean
  error?: string
}

// Estimated retry schedule for a deferred message (RFC3339 UTC timestamps).
export interface NextDeliveryAttempt {
  deferred: boolean
  attempts: number
  lastAttempt?: string
  nextAttempt?: string
  remainingAttempts: number
  finalAttempt?: string
  willExpire: boolean
  expiresAt?: string
  interval?: string
}

export interface TestBounceDiagnosticRequest {
  smtp_code: string
  domain: string
  diagnostic: string
  attempts: number
}

export interface TestBounceDiagnosticResult {
  smtpCode: string
  enhancedCode: string
  provider: string
  matched: boolean
  rule?: BounceRule
  effectiveAction: BounceAction
}

// Response member shape (camelCase).
export interface VMTAGroupMember {
  vmtaId: string
  weight: number
}

// Request member shape (snake_case) used in the create body.
export interface VMTAGroupMemberInput {
  vmta_id: string
  weight: number
}

export interface VMTAGroup {
  id: string
  name: string
  status: string
  members: VMTAGroupMember[]
}

export interface CreateVMTAGroupRequest {
  name: string
  members: VMTAGroupMemberInput[]
}

export interface UpdateVMTAGroupRequest {
  name: string
  status: string
  members: VMTAGroupMemberInput[]
}

// Backend enum values (lowercase).
export type MatchType = 'mailclass' | 'recipient_email' | 'recipient_domain' | 'sender_ip' | string
export type TargetType = 'vmta' | 'vmta_group' | '' | string

// Response type: camelCase. matchHeader is the header NAME for a mailclass match.
export interface RoutingRule {
  id: string
  name: string
  matchType: MatchType
  matchHeader?: string
  matchValue: string
  priority: number
  targetType: TargetType
  targetId: string
  // assignMailclass is the class applied by a sender_ip rule (matchValue is
  // then an IP or CIDR). Empty for other match types.
  assignMailclass?: string
  status: string
}

export interface CreateRoutingRuleRequest {
  name: string
  match_type: MatchType
  match_header?: string
  match_value: string
  priority: number
  target_type: TargetType
  target_id: string
  assign_mailclass?: string
}

export interface UpdateRoutingRuleRequest {
  name: string
  match_type: MatchType
  match_header?: string
  match_value: string
  priority: number
  target_type: TargetType
  target_id: string
  assign_mailclass?: string
  status: string
}

// ---- Mail operations ----

export interface MailRecord {
  id: string
  messageId: string
  eventTime: string
  mailclass: string
  sender: string
  /** Original From header (the envelope sender is VERP-rewritten at reception). */
  fromHeader?: string
  recipient: string
  recipientDomain: string
  vmtaId: string
  /** Sending VMTA name for this event (delivery/bounce); empty on reception. */
  egressSource?: string
  status: string
  /** Raw KumoMTA log record type (Reception/Delivery/Bounce/TransientFailure/
   * AdminBounce/Expiration); distinguishes the three types that share status="bounced". */
  recordType?: string
  /** SMTP response for this event (code + text); present on delivery/deferral/bounce. */
  smtpStatus?: string
  diagnostic?: string
  /** Optional subject-derived label (≤2 words); empty when the feature is off. */
  classification?: string
}

export interface MailRecordFilters {
  mailclass?: string
  sender?: string
  /** Case-insensitive substring match on the original From header. */
  from?: string
  recipient?: string
  vmta_id?: string
  status?: string
  /** Filter by raw KumoMTA log record type (e.g. "AdminBounce"). */
  record_type?: string
  /** RFC3339 lower bound on event time. */
  from_time?: string
  /** RFC3339 upper bound on event time. */
  to_time?: string
  [key: string]: string | undefined
}

/** One persisted worker error-log entry (a Warn/Error a background worker emitted). */
export interface WorkerErrorLog {
  id: string
  eventTime: string
  /** "warn" | "error" */
  level: string
  worker: string
  message: string
  /** Structured slog attributes as a JSON object string. */
  detail: string
}

export interface WorkerErrorLogFilters {
  level?: string
  worker?: string
  /** RFC3339 lower/upper bounds on event_time. */
  from?: string
  to?: string
  [key: string]: string | undefined
}

export interface Bounce {
  id: string
  eventTime: string
  recipient: string
  mailclass: string
  smtpStatus: string
  bounceType: string
  diagnostic: string
  processingState: string
  classification?: string
}

/** Query filters for the bounce list (mirrors ListBouncesRequest). */
export interface BounceFilters {
  /** Case-insensitive substring match on the recipient address. */
  recipient?: string
  mailclass?: string
  /** hard | soft | dsn; empty matches all. */
  bounce_type?: string
  /** Case-insensitive substring match on the classifier category. */
  classification?: string
  /** new | processing | processed | suppressed | retried; empty matches all. */
  processing_state?: string
  /** RFC3339 lower bound on event time. */
  from_time?: string
  /** RFC3339 upper bound on event time. */
  to_time?: string
  [key: string]: string | undefined
}

export interface FeedbackReport {
  id: string
  receivedAt: string
  source: string
  reportType: string
  recipient: string
  processingState: string
  /** True when the complaint was proven to be about mail we sent. */
  verified: boolean
  /** How it was proven: supplemental-trace | send-log | dkim | "" (unverified). */
  verification: string
}

// Queue is a live kumod scheduled-queue summary for a destination domain.
export interface Queue {
  domain: string
  // int64 arrives as a JSON string via proto-JSON.
  depth: string
  suspended?: boolean
  suspendReason?: string
}

// Backend enum values (lowercase).
export type QueueAction = 'suspend' | 'resume' | 'bounce'

export interface QueueActionRequest {
  action: QueueAction
  domain: string
  reason?: string
  confirmation_id?: string
}

export interface QueueActionResponse {
  status: string
  summary: string
}

// Backend enum values (lowercase).
export type ServiceOperation = 'restart' | 'reload' | 'start' | 'stop'

export interface ServiceControlRequest {
  operation: ServiceOperation
  confirmation_id: string
}

export interface ServiceControlResponse {
  id: string
  operation: string
  status: string
}

// ---- Identity & audit ----

export interface User {
  id: string
  email: string
  displayName: string
  status: string
  mfaRequired: boolean
  roles: string[]
}

export interface EnrollMfaReply {
  secret: string
  otpauthUri: string
}

export interface ConfirmMfaReply {
  enrolled: boolean
  // A fresh, fully-authenticated token issued when a first-login enrollment
  // is confirmed.
  token?: string
}

// Login status values returned by the auth endpoints.
export type LoginStatus = 'authenticated' | 'mfa_required' | 'mfa_enrollment_required' | string

// Response: proto-JSON camelCase.
export interface LoginReply {
  token: string
  status: LoginStatus
  user: User
  permissions: string[]
}

export interface CurrentUserReply {
  user: User
  permissions: string[]
}

export interface CreateUserRequest {
  email: string
  display_name: string
  mfa_required: boolean
  roles: string[]
  // Optional initial password; empty leaves login disabled for the account.
  password?: string
}

// Email is immutable on edit, so it is not part of the update body.
export interface UpdateUserRequest {
  display_name: string
  status: string
  mfa_required: boolean
  roles: string[]
}

// Admin reset of another user's password. Strength-validated server-side.
export interface ResetPasswordRequest {
  password: string
}

export interface AuditEntry {
  id: string
  occurredAt: string
  actorUserId: string
  operation: string
  targetType: string
  targetId: string
  outcome: string
  ipAddress: string
}

// ---- Domain safety ----

export interface DkimDomain {
  id: string
  domain: string
  selector: string
  publicKeyFingerprint: string
  status: string
}

export interface CreateDkimDomainRequest {
  domain: string
  selector: string
  public_key_fingerprint: string
  private_key_ref: string
}

// Domain is immutable on edit. private_key_ref (PEM) is optional: leave blank to
// keep the existing key (the server preserves it when blank).
export interface UpdateDkimDomainRequest {
  selector: string
  public_key_fingerprint: string
  private_key_ref: string
  status: string
}

export interface GenerateDkimKeyRequest {
  domain: string
  selector: string
}

export interface GenerateDkimKeyReply {
  privateKeyPem: string
  recordName: string
  recordValue: string
  publicKeyFingerprint: string
}

export interface Suppression {
  id: string
  type: string
  value: string
  reason: string
  source: string
  status: string
  mailclass?: string
  createdAt?: string
  expiresAt?: string
}

/** Query filters for the suppression list (mirrors ListSuppressionsRequest). */
export interface SuppressionFilters {
  /** Case-insensitive substring match on the suppressed value. */
  search?: string
  /** email | domain; empty matches all. */
  type?: string
  /** active | disabled | expired; empty matches all. */
  status?: string
  /** manual | bounce | feedback | dsn; empty matches all. */
  source?: string
  /** Exact match on the triggering event's mailclass. */
  mailclass?: string
  [key: string]: string | undefined
}

export interface CreateSuppressionRequest {
  type: 'email' | 'domain'
  value: string
  reason: string
}

// ---- Require-TLS policies (outbound delivery) ----

export type TLSPolicyMode = 'required' | 'required_insecure'

export interface TLSPolicy {
  id: string
  domain: string
  mode: TLSPolicyMode | string
  status: string
}

export interface CreateTLSPolicyRequest {
  domain: string
  mode: TLSPolicyMode
}

// Only reason and status are editable; type/value are immutable.
export interface UpdateSuppressionRequest {
  reason: string
  status: string
}

// Raw asynchronous bounce (DSN) archived behind a dsn-sourced suppression.
export interface DsnMessage {
  id: string
  messageId: string
  rawMessage: string
  receivedAt: string
}

// ---- Inbound automation ----

// ---- Inbound routes (maildir / forward / webhook) ----

export type InboundRouteAction = 'maildir' | 'forward' | 'webhook'
export type InboundRouteMatchType = 'recipient_email' | 'recipient_domain'
export type ForwardTLS = 'none' | 'opportunistic' | 'required'
export type SpamScanMode = 'default' | 'off' | 'tag' | 'enforce'

export interface InboundRoute {
  id: string
  name: string
  matchType: string
  matchValue: string
  action: string
  priority: number
  status: string
  spamScan: string
  forwardHost: string
  forwardPort: number
  forwardTls: string
  maildirPath: string
  destinationUrl: string
  timeoutSeconds: number
}

// secret_ref is write-only; blank preserves the existing secret on edit.
export interface InboundRouteRequest {
  name: string
  match_type: InboundRouteMatchType
  match_value: string
  action: InboundRouteAction
  priority: number
  status: string
  spam_scan: SpamScanMode
  forward_host: string
  forward_port: number
  forward_tls: ForwardTLS
  maildir_path: string
  destination_url: string
  timeout_seconds: number
  secret_ref: string
}

// ---- Feedback loops ----

export type FeedbackLoopStatus = 'awaiting_approval' | 'approved'

export interface FeedbackLoop {
  id: string
  domain: string
  feedbackAddress: string
  forwardAddress: string
  status: string
}

export interface CreateFeedbackLoopRequest {
  domain: string
  feedback_address: string
  forward_address: string
  status: FeedbackLoopStatus
}

export interface UpdateFeedbackLoopRequest {
  domain: string
  feedback_address: string
  forward_address: string
  status: FeedbackLoopStatus
}

export interface RspamdResult {
  id: string
  eventTime: string
  mailRecordId: string
  messageId: string
  recipient: string
  action: string
  score: number
  /** Rspamd symbol names that fired for this message (proto: repeated string). */
  symbols: string[]
  reason: string
}

// ---- Retention ----

export interface RetentionPolicy {
  tableName: string
  retentionDays: number
  compressAfterDays: number
  enabled: boolean
  updatedAt?: string
  updatedBy?: string
}

export interface RetentionRun {
  id: string
  tableName: string
  startedAt: string
  finishedAt?: string
  chunksCompressed: number
  chunksDropped: number
  bytesBefore: number
  bytesAfter: number
  error?: string
}

export interface RetentionView {
  policy: RetentionPolicy
  label: string
  hypertable: boolean
  chunkCount: number
  compressedChunks: number
  totalBytes: number
  compressedBytes: number
  uncompressedBytes: number
  oldestData?: string
  newestData?: string
  lastRun?: RetentionRun
}

export interface UpdateRetentionPolicyRequest {
  table_name: string
  retention_days: number
  compress_after_days: number
  enabled: boolean
}

// ---- KumoMTA config ----

export interface KumoConfigPreview {
  content: string
  vmtaCount: number
  poolCount: number
  routeCount: number
  dkimCount: number
  suppressionCount: number
  checksum: string
  // valid is true when the rendered policy passed the Lua syntax lint.
  valid?: boolean
  lintIssues?: string[]
}

export interface KumoConfigApplyRequest {
  confirmation_id: string
}

// The policy currently running on KumoMTA (the last one Iris applied), used to
// diff against a freshly generated policy.
export interface AppliedKumoConfig {
  content: string
  checksum: string
  appliedAt: string
  neverApplied: boolean
}

// ---- Global settings (deployment-level policy knobs) ----

export interface GlobalSettings {
  rspamdMode: string
  rspamdUrl: string
  egressEhloDomain: string
  logStreamRedisUrl: string
  esmtpListen: string
  httpListen: string
  egressRetryInterval: string
  egressMaxRetryInterval: string
  egressMaxAge: string
  pinEgressPerMessage: boolean
  bounceDomain: string
  bounceDomainTemplate: string
  autoSuppressHardBounces: boolean
  softBounceThreshold: number
  suppressionTtl: string
  dmarcReportEmail: string
  adminHttpAddr: string
  adminTlsEnabled: boolean
  adminTlsCertDomain: string
  acmeRenewInterval: string
  acmeRenewBefore: string
  prometheusUrl: string
  fblRequireVerification: boolean
  inboundMaildirBasePath: string
  classifySubjects: boolean
  classifyModel: string
  classifyThreshold: number
  classifyApiBase: string
  updatedAt?: string
  updatedBy?: string
}

export interface UpdateGlobalSettingsRequest {
  rspamd_mode: string
  rspamd_url: string
  egress_ehlo_domain: string
  log_stream_redis_url: string
  esmtp_listen: string
  http_listen: string
  egress_retry_interval: string
  egress_max_retry_interval: string
  egress_max_age: string
  pin_egress_per_message: boolean
  bounce_domain: string
  auto_suppress_hard_bounces: boolean
  soft_bounce_threshold: number
  suppression_ttl: string
  dmarc_report_email: string
  admin_http_addr: string
  admin_tls_enabled: boolean
  admin_tls_cert_domain: string
  acme_renew_interval: string
  acme_renew_before: string
  prometheus_url: string
  fbl_require_verification: boolean
  inbound_maildir_base_path: string
  bounce_domain_template: string
  classify_subjects: boolean
  classify_model: string
  classify_threshold: number
  classify_api_base: string
}

// ---- Subject classifications ----

export interface SubjectClassification {
  id: string
  subject: string
  subjectNormalized: string
  label: string
  source: string // "manual" | "ai"
  hitCount: string // int64 as JSON string
  createdAt?: string
  updatedAt?: string
}

export interface CreateSubjectClassificationRequest {
  subject: string
  label: string
}

export interface UpdateSubjectClassificationRequest {
  id: string
  subject: string
  label: string
}

// ---- Dashboard metrics (Prometheus-backed time-series) ----

export interface MetricPoint {
  timestamp: number // unix seconds
  value: number // events per minute
}

export interface MetricsSeries {
  key: string
  label: string
  points?: MetricPoint[]
}

export interface MetricsTimeseries {
  series?: MetricsSeries[]
  range: string
  stepSeconds: number
  prometheusAvailable: boolean
}

// Delivery queue-time distribution (from the iris_mail_queue_time_seconds
// histogram). Counts are int64 → JSON strings.
export interface QueueTimeBucket {
  le: string
  upperBound: number
  count: string
}

export interface QueueTimeHistogram {
  buckets?: QueueTimeBucket[]
  mailclasses?: string[]
  totalCount: string
  range: string
  prometheusAvailable: boolean
}

export interface KumoConfigApplyResponse {
  requestId: string
  status: string
  checksum: string
  appliedPath: string
  resultSummary: string
}

export interface AcmeAccount {
  email: string
  serverUrl: string
  configured: boolean
  registered: boolean
  updatedAt: string
}

export interface SaveAcmeAccountRequest {
  email: string
  server_url: string
}

export interface AcmeCertificate {
  id: string
  domain: string
  altNames: string[]
  challengeType: string
  certPath: string
  keyPath: string
  expiresAt: string
  lastRenewedAt: string
  status: string
  lastError: string
}

export interface RequestAcmeCertificateRequest {
  domain: string
  alt_names: string[]
}

// DNS-01 provider registry metadata (drives the dynamic credentials form).
export interface AcmeDnsProviderInfo {
  name: string
  description: string
  requiredFields?: string[]
  optionalFields?: string[]
}

// Configured DNS-01 provider. On read, config values are redacted to
// "[stored]"; on write, send real credential values.
export interface AcmeDnsProvider {
  provider: string
  config: Record<string, string>
  updatedAt: string
}

export interface SetAcmeDnsProviderRequest {
  provider: string
  config: Record<string, string>
}

export interface KumoConfigStatus {
  drift: boolean
  neverApplied: boolean
  currentChecksum: string
  appliedChecksum: string
  appliedAt: string
  restartRequired: boolean
}

// ---- Dashboard ----

// The dashboard summary returns scalar counts. Note: protobuf int64 fields are
// serialized as JSON strings, so the count fields are strings.
export interface DashboardSummary {
  serviceState: string
  queuedMessages: string
  recentMailEvents: string
  recentAuditEvents: string
  // Messages deferred and still in the queue (retrying, not yet bounced).
  deferredInQueue: string
}

// Per-VMTA, per-recipient-domain delivery/bounce breakdown for IP-warmup health.
// int64 count fields serialize as JSON strings; rates are doubles (0..1).
export interface WarmupDeliveryStat {
  vmtaId: string
  vmtaName: string
  recipientDomain: string
  sent: string
  bounced: string
  deferred: string
  attempted: string
  deliveryRate: number
  bounceRate: number
}

// Distinct messages that deferred to one recipient domain, deduped across VMTAs
// (so it's summable, unlike the per-VMTA rows). int64 → JSON string.
export interface DomainDeferredStat {
  recipientDomain: string
  messages: string
}

export interface WarmupDeliveryStats {
  rows?: WarmupDeliveryStat[]
  deferredByDomain?: DomainDeferredStat[]
  range: string
  since: string
}

// Mail-record volume grouped by mailclass over a lookback window. int64 counts
// serialize as JSON strings via proto-JSON.
export interface MailClassStat {
  mailclass: string
  count: string
  delivered: string
  bounced: string
  deferred: string
}

export interface MailClassStats {
  rows?: MailClassStat[]
  range: string
  since: string
}

// Mail-record volume grouped by recipient domain (busiest first) over a window.
export interface RecipientDomainStat {
  recipientDomain: string
  count: string
  delivered: string
  bounced: string
  deferred: string
}

export interface RecipientDomainStats {
  rows?: RecipientDomainStat[]
  range: string
  since: string
}

// ---- Domain bounce-readiness check ----
export interface DomainCheckItem {
  name: string
  status: 'pass' | 'warn' | 'fail' | string
  detail: string
  records?: string[]
}
export interface DomainBounceCheck {
  domain: string
  items?: DomainCheckItem[]
}

// ---- DMARC aggregate reports ----

export interface DmarcCount {
  label: string
  count: number
}
export interface DmarcSource {
  ip: string
  total: number
  pass: number
  fail: number
}
export interface DmarcDomainStat {
  domain: string
  messages: number
  pass: number
}
// Per-reporter (org_name) rollup — who sent the aggregate reports.
export interface DmarcReporterStat {
  reporter: string
  messages: number
  pass: number
}
export interface DmarcDay {
  date: string
  messages: number
  pass: number
}
export interface DmarcStats {
  totalMessages: number
  dmarcPass: number
  spfPass: number
  dkimPass: number
  dispositions?: DmarcCount[]
  topSources?: DmarcSource[]
  domains?: DmarcDomainStat[]
  series?: DmarcDay[]
  reporters?: DmarcReporterStat[]
}
export interface DmarcReport {
  orgName: string
  reportId: string
  domain: string
  dateBegin: string
  dateEnd: string
  policyP: string
  policyPct: number
  receivedAt: string
}

// ---- Tools: Diagnose + RBL ----

export interface DiagnoseRequest {
  from_email: string
  recipient?: string
  mailclass?: string
}
export interface RoutingOutcome {
  matchedRule?: string
  egressPool?: string
  vmtas?: string[]
  egressIps?: string[]
  listeners?: string[]
  note?: string
}
export interface DiagnoseResult {
  fromEmail: string
  domain: string
  items?: DomainCheckItem[]
  routing?: RoutingOutcome
}
export interface RblListing {
  zone: string
  listed: boolean
  reason?: string
}
export interface RblIpResult {
  ip: string
  source: string
  listed: boolean
  listings?: RblListing[]
}
export interface RblCheckReply {
  results?: RblIpResult[]
  zones?: string[]
  checkedAt?: string
  skipped?: string[]
}

// ---- System self-monitoring ----

export interface DiskUsage {
  path: string
  usedPercent: number
  usedBytes: string // uint64 → JSON string
  totalBytes: string
}

export interface SystemSnapshot {
  collectedAt: string
  cpuPercent: number
  memPercent: number
  memUsedBytes: string
  memTotalBytes: string
  disks?: DiskUsage[]
  available: boolean
}

export interface MonitorSettings {
  enabled: boolean
  cpuThreshold: number
  memThreshold: number
  diskThreshold: number
  diskPaths?: string[]
  notifyEmails?: string[]
  fromEmail: string
  smtpHost: string
  cooldownMinutes: number
  sampleSeconds: number
}

export interface MonitorAlert {
  id: string
  resource: string
  detail: string
  level: string
  value: number
  threshold: number
  message: string
  notified: boolean
  createdAt: string
}

export interface Mount {
  path: string
  device: string
  fstype: string
  usedPercent: number
  usedBytes: string
  totalBytes: string
}

export interface SystemMonitor {
  snapshot?: SystemSnapshot
  settings?: MonitorSettings
  recentAlerts?: MonitorAlert[]
  mounts?: Mount[]
  spoolPath?: string
}

export interface TestMonitorNotificationResult {
  ok: boolean
  error?: string
}
