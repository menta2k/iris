import { http } from './http'
import type {
  AuditEntry,
  CreateUserRequest,
  EnrollMfaReply,
  ListResponse,
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
  listAuditEntries() {
    return http.get<ListResponse<AuditEntry>>('/audit-entries')
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
