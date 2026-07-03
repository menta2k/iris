import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type { DmarcStats, DmarcReport, ListResponse } from '@/types'

export const dmarcService = {
  stats(domain?: string, reporter?: string) {
    const query: Record<string, string> = {}
    if (domain) query.domain = domain
    if (reporter) query.reporter = reporter
    return http.get<DmarcStats>('/dmarc/stats', {
      query: Object.keys(query).length ? query : undefined,
    })
  },
  listReports(domain?: string, page?: PageParams) {
    const query = { ...pageQuery(page), ...(domain ? { domain } : {}) }
    return http.get<ListResponse<DmarcReport>>('/dmarc/reports', { query })
  },
  domains() {
    return http.get<{ domains?: string[] }>('/dmarc/domains')
  },
}
