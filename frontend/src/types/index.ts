export * from './api'

export type Permission =
  | 'outbound:read'
  | 'outbound:write'
  | 'operations:read'
  | 'operations:write'
  | 'security:read'
  | 'security:write'
  | 'domain-safety:read'
  | 'domain-safety:write'
  | 'inbound:read'
  | 'inbound:write'
  | 'dashboard:read'
  | 'service:control'

export type Role = 'admin' | 'operator' | 'viewer'
