import { http } from './http'
import type {
  CreateWebhookRuleRequest,
  ListResponse,
  RspamdResult,
  UpdateWebhookRuleRequest,
  WebhookDeliveryEvent,
  WebhookRule,
} from '@/types'

export const inboundAutomationService = {
  listWebhookRules() {
    return http.get<ListResponse<WebhookRule>>('/webhook-rules')
  },
  createWebhookRule(body: CreateWebhookRuleRequest) {
    return http.post<WebhookRule>('/webhook-rules', body)
  },
  updateWebhookRule(id: string, body: UpdateWebhookRuleRequest) {
    return http.put<WebhookRule>(`/webhook-rules/${id}`, body)
  },
  listDeliveryEvents() {
    return http.get<ListResponse<WebhookDeliveryEvent>>('/webhook-deliveries')
  },
  listRspamdResults() {
    return http.get<ListResponse<RspamdResult>>('/rspamd-results')
  },
}
