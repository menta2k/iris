import { http } from './http'
import type {
  CreateWebhookRuleRequest,
  InboundRoute,
  InboundRouteRequest,
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
  listInboundRoutes() {
    return http.get<ListResponse<InboundRoute>>('/inbound-routes')
  },
  createInboundRoute(body: InboundRouteRequest) {
    return http.post<InboundRoute>('/inbound-routes', body)
  },
  updateInboundRoute(id: string, body: InboundRouteRequest) {
    return http.put<InboundRoute>(`/inbound-routes/${id}`, body)
  },
  deleteInboundRoute(id: string) {
    return http.delete<{ ok: boolean }>(`/inbound-routes/${id}`)
  },
  listDeliveryEvents() {
    return http.get<ListResponse<WebhookDeliveryEvent>>('/webhook-deliveries')
  },
  listRspamdResults() {
    return http.get<ListResponse<RspamdResult>>('/rspamd-results')
  },
}
