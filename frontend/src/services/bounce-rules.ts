import { http } from './http'
import type {
  BounceRule,
  CreateBounceRuleRequest,
  ListResponse,
  TestBounceDiagnosticRequest,
  TestBounceDiagnosticResult,
  UpdateBounceRuleRequest,
} from '@/types'

export const bounceRulesService = {
  list() {
    return http.get<ListResponse<BounceRule>>('/bounce-rules')
  },
  create(body: CreateBounceRuleRequest) {
    return http.post<BounceRule>('/bounce-rules', body)
  },
  update(id: string, body: UpdateBounceRuleRequest) {
    return http.put<BounceRule>(`/bounce-rules/${id}`, body)
  },
  remove(id: string) {
    return http.delete<Record<string, never>>(`/bounce-rules/${id}`)
  },
  reset() {
    return http.post<ListResponse<BounceRule>>('/bounce-rules:reset', {})
  },
  test(body: TestBounceDiagnosticRequest) {
    return http.post<TestBounceDiagnosticResult>('/bounce-rules:test', body)
  },
}
