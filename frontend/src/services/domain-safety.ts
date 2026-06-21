import { http } from './http'
import type {
  CreateDkimDomainRequest,
  CreateSuppressionRequest,
  DkimDomain,
  GenerateDkimKeyReply,
  GenerateDkimKeyRequest,
  ListResponse,
  Suppression,
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
  listSuppressions() {
    return http.get<ListResponse<Suppression>>('/suppressions')
  },
  createSuppression(body: CreateSuppressionRequest) {
    return http.post<Suppression>('/suppressions', body)
  },
  updateSuppression(id: string, body: UpdateSuppressionRequest) {
    return http.put<Suppression>(`/suppressions/${id}`, body)
  },
}
