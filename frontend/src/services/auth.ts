import { http } from './http'
import type {
  ConfirmMfaReply,
  CurrentUserReply,
  EnrollMfaReply,
  LoginReply,
} from '@/types'

// Auth endpoints. Request bodies use the proto field (snake_case) names the
// HTTP transcoder accepts; responses are proto-JSON camelCase.
export const authService = {
  login(email: string, password: string) {
    return http.post<LoginReply>('/auth:login', { email, password })
  },
  verifyMfa(code: string) {
    return http.post<LoginReply>('/auth:verify-mfa', { code })
  },
  currentUser() {
    return http.get<CurrentUserReply>('/auth:me')
  },
  enrollMfa() {
    return http.post<EnrollMfaReply>('/mfa:enroll', {})
  },
  confirmMfa(code: string) {
    return http.post<ConfirmMfaReply>('/mfa:confirm', { code })
  },
  changePassword(currentPassword: string, newPassword: string) {
    return http.post<Record<string, never>>('/auth:change-password', {
      current_password: currentPassword,
      new_password: newPassword,
    })
  },
  logout() {
    return http.post<Record<string, never>>('/auth:logout', {})
  },
}
