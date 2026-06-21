// Shared API types matching the Iris KumoMTA admin backend contract.

export interface PageInfo {
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
}

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

// Request body: a VMTA attaches to a listener; there is no ip/ehlo anymore.
export interface CreateVMTARequest {
  name: string
  listener_id: string
  max_connections: number
}

// Update body: status and notes become editable on edit.
export interface UpdateVMTARequest {
  name: string
  listener_id: string
  max_connections: number
  status: string
  notes: string
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
export type MatchType = 'mailclass' | 'recipient_email' | 'recipient_domain' | string
export type TargetType = 'vmta' | 'vmta_group' | string

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
}

export interface UpdateRoutingRuleRequest {
  name: string
  match_type: MatchType
  match_header?: string
  match_value: string
  priority: number
  target_type: TargetType
  target_id: string
  status: string
}

// ---- Mail operations ----

export interface MailRecord {
  id: string
  messageId: string
  eventTime: string
  mailclass: string
  sender: string
  recipient: string
  recipientDomain: string
  vmtaId: string
  status: string
}

export interface MailRecordFilters {
  mailclass?: string
  sender?: string
  recipient?: string
  vmta_id?: string
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
}

export interface FeedbackReport {
  id: string
  receivedAt: string
  source: string
  reportType: string
  recipient: string
  processingState: string
}

export interface Queue {
  mailclass: string
  state: string
  // int64 fields arrive as JSON strings via proto-JSON.
  depth: string
  oldestMessageAgeSeconds: string
}

// Backend enum values (lowercase).
export type QueueAction = 'pause' | 'resume' | 'drain' | 'flush'

export interface QueueActionRequest {
  action: QueueAction
  confirmation_id: string
}

export interface QueueActionResponse {
  request_id: string
  status: string
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

export interface CreateUserRequest {
  email: string
  display_name: string
  mfa_required: boolean
  roles: string[]
}

// Email is immutable on edit, so it is not part of the update body.
export interface UpdateUserRequest {
  display_name: string
  status: string
  mfa_required: boolean
  roles: string[]
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
}

export interface CreateSuppressionRequest {
  type: 'email' | 'domain'
  value: string
  reason: string
}

// Only reason and status are editable; type/value are immutable.
export interface UpdateSuppressionRequest {
  reason: string
  status: string
}

// ---- Inbound automation ----

export interface WebhookRule {
  id: string
  name: string
  matchType: string
  matchValue: string
  destinationUrl: string
  status: string
  timeoutSeconds: number
}

export interface CreateWebhookRuleRequest {
  name: string
  match_type: 'recipient_email' | 'recipient_domain'
  match_value: string
  destination_url: string
  secret_ref: string
  timeout_seconds: number
}

// secret_ref is optional on edit: blank preserves the existing secret.
export interface UpdateWebhookRuleRequest {
  name: string
  match_type: 'recipient_email' | 'recipient_domain'
  match_value: string
  destination_url: string
  secret_ref: string
  status: string
  timeout_seconds: number
}

export interface WebhookDeliveryEvent {
  id: string
  eventTime: string
  webhookRuleId: string
  webhookName: string
  mailRecordId: string
  recipient: string
  attempt: number
  status: string
  responseCode: number
  errorSummary: string
}

export interface RspamdResult {
  id: string
  eventTime: string
  mailRecordId: string
  action: string
  score: number
  symbols: string
  reason: string
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
  bounceDomain: string
  autoSuppressHardBounces: boolean
  softBounceThreshold: number
  fblDomain: string
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
  bounce_domain: string
  auto_suppress_hard_bounces: boolean
  soft_bounce_threshold: number
  fbl_domain: string
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
}
