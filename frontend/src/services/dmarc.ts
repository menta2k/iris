import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type { DmarcStats, DmarcReport, ListResponse } from '@/types'

export const dmarcService = {
  stats(domain?: string) {
    return http.get<DmarcStats>('/dmarc/stats', { query: domain ? { domain } : undefined })
  },
  listReports(domain?: string, page?: PageParams) {
    const query = { ...pageQuery(page), ...(domain ? { domain } : {}) }
    return http.get<ListResponse<DmarcReport>>('/dmarc/reports', { query })
  },
  domains() {
    return http.get<{ domains?: string[] }>('/dmarc/domains')
  },
}
