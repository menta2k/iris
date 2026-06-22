import { http } from './http'
import type { DomainBounceCheck } from '@/types'

export const domainCheckService = {
  check(domain: string) {
    return http.get<DomainBounceCheck>(`/domain-check/${encodeURIComponent(domain)}`)
  },
}
