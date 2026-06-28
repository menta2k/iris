import { http } from './http'
import type {
  InboundRoute,
  InboundRouteRequest,
  ListResponse,
  RspamdResult,
} from '@/types'

export const inboundAutomationService = {
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
  listRspamdResults() {
    return http.get<ListResponse<RspamdResult>>('/rspamd-results')
  },
}
