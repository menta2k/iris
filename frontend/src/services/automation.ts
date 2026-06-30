import { http } from './http'
import type {
  AutomationRule,
  CreateAutomationRuleRequest,
  ListResponse,
  UpdateAutomationRuleRequest,
} from '@/types'

export const automationService = {
  list() {
    return http.get<ListResponse<AutomationRule>>('/automation-rules')
  },
  create(body: CreateAutomationRuleRequest) {
    return http.post<AutomationRule>('/automation-rules', body)
  },
  update(id: string, body: UpdateAutomationRuleRequest) {
    return http.put<AutomationRule>(`/automation-rules/${id}`, body)
  },
  setStatus(id: string, status: 'active' | 'disabled') {
    return http.post<AutomationRule>(`/automation-rules/${id}:status`, { status })
  },
}
