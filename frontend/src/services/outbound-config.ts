import { http } from './http'
import type {
  CreateListenerRequest,
  CreateRoutingRuleRequest,
  CreateVMTAGroupRequest,
  CreateVMTARequest,
  Listener,
  ListResponse,
  RoutingRule,
  UpdateListenerRequest,
  UpdateRoutingRuleRequest,
  UpdateVMTAGroupRequest,
  UpdateVMTARequest,
  VMTA,
  VMTAGroup,
} from '@/types'

export const outboundConfigService = {
  listListeners(status?: string) {
    return http.get<ListResponse<Listener>>('/listeners', { query: { status } })
  },
  createListener(body: CreateListenerRequest) {
    return http.post<Listener>('/listeners', body)
  },
  updateListener(id: string, body: UpdateListenerRequest) {
    return http.put<Listener>(`/listeners/${id}`, body)
  },
  listVmtas(status?: string) {
    return http.get<ListResponse<VMTA>>('/vmtas', { query: { status } })
  },
  createVmta(body: CreateVMTARequest) {
    return http.post<VMTA>('/vmtas', body)
  },
  updateVmta(id: string, body: UpdateVMTARequest) {
    return http.put<VMTA>(`/vmtas/${id}`, body)
  },
  listVmtaGroups() {
    return http.get<ListResponse<VMTAGroup>>('/vmta-groups')
  },
  createVmtaGroup(body: CreateVMTAGroupRequest) {
    return http.post<VMTAGroup>('/vmta-groups', body)
  },
  updateVmtaGroup(id: string, body: UpdateVMTAGroupRequest) {
    return http.put<VMTAGroup>(`/vmta-groups/${id}`, body)
  },
  listRoutingRules(filters?: { match_type?: string; match_value?: string }) {
    return http.get<ListResponse<RoutingRule>>('/routing-rules', { query: filters })
  },
  createRoutingRule(body: CreateRoutingRuleRequest) {
    return http.post<RoutingRule>('/routing-rules', body)
  },
  updateRoutingRule(id: string, body: UpdateRoutingRuleRequest) {
    return http.put<RoutingRule>(`/routing-rules/${id}`, body)
  },
}
