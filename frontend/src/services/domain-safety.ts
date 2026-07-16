import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type {
  CreateDkimDomainRequest,
  CreateSuppressionRequest,
  CreateTLSPolicyRequest,
  DkimDomain,
  DsnMessage,
  GenerateDkimKeyReply,
  GenerateDkimKeyRequest,
  ListResponse,
  Suppression,
  SuppressionFilters,
  TLSPolicy,
  UpdateDkimDomainRequest,
  UpdateSuppressionRequest,
} from '@/types'

export const domainSafetyService = {
  listDkimDomains() {
    return http.get<ListResponse<DkimDomain>>('/dkim-domains')
  },
  createDkimDomain(body: CreateDkimDomainRequest) {
    return http.post<DkimDomain>('/dkim-domains', body)
  },
  updateDkimDomain(id: string, body: UpdateDkimDomainRequest) {
    return http.put<DkimDomain>(`/dkim-domains/${id}`, body)
  },
  generateDkimKey(body: GenerateDkimKeyRequest) {
    return http.post<GenerateDkimKeyReply>('/dkim-domains:generate-key', body)
  },
  listSuppressions(page?: PageParams, filters?: SuppressionFilters) {
    // Only send non-empty filters so the query stays clean.
    const clean = Object.fromEntries(
      Object.entries(filters ?? {}).filter(([, v]) => (v ?? '').toString().trim() !== ''),
    )
    return http.get<ListResponse<Suppression>>('/suppressions', {
      query: pageQuery(page, Object.keys(clean).length ? clean : undefined),
    })
  },
  createSuppression(body: CreateSuppressionRequest) {
    return http.post<Suppression>('/suppressions', body)
  },
  updateSuppression(id: string, body: UpdateSuppressionRequest) {
    return http.put<Suppression>(`/suppressions/${id}`, body)
  },
  // Bulk-remove every permanent (no-expiry) suppression from DB + Redis.
  deletePermanentSuppressions() {
    return http.post<{ deleted: number }>('/suppressions:delete-permanent', {})
  },
  listSuppressionDsnMessages(id: string) {
    return http.get<ListResponse<DsnMessage>>(`/suppressions/${id}/dsn-messages`)
  },
  listTLSPolicies(page?: PageParams, search?: string) {
    const s = (search ?? '').trim()
    return http.get<ListResponse<TLSPolicy>>('/tls-policies', {
      query: pageQuery(page, s ? { search: s } : undefined),
    })
  },
  createTLSPolicy(body: CreateTLSPolicyRequest) {
    return http.post<TLSPolicy>('/tls-policies', body)
  },
  deleteTLSPolicy(id: string) {
    return http.delete<Record<string, never>>(`/tls-policies/${id}`)
  },
}
