import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type {
  AuditEntry,
  CreateUserRequest,
  EnrollMfaReply,
  ListResponse,
  ResetPasswordRequest,
  UpdateUserRequest,
  User,
} from '@/types'

export const identityAuditService = {
  listUsers() {
    return http.get<ListResponse<User>>('/users')
  },
  createUser(body: CreateUserRequest) {
    return http.post<User>('/users', body)
  },
  updateUser(id: string, body: UpdateUserRequest) {
    return http.put<User>(`/users/${id}`, body)
  },
  resetPassword(id: string, body: ResetPasswordRequest) {
    return http.post<Record<string, never>>(`/users/${id}:reset-password`, body)
  },
  listAuditEntries(page?: PageParams) {
    return http.get<ListResponse<AuditEntry>>('/audit-entries', { query: pageQuery(page) })
  },
  enrollMfa() {
    return http.post<EnrollMfaReply>('/mfa:enroll', {})
  },
  confirmMfa(code: string) {
    return http.post<{ enrolled: boolean }>('/mfa:confirm', { code })
  },
  disableMfa() {
    return http.post<Record<string, never>>('/mfa:disable', {})
  },
}
