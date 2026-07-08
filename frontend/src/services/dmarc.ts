import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type { DmarcStats, DmarcReport, ListResponse } from '@/types'

export interface DmarcStatsOptions {
  domain?: string
  reporter?: string
  /** RFC3339 lower bound on the report window. */
  from?: string
}

export const dmarcService = {
  stats(opts: DmarcStatsOptions = {}) {
    const query: Record<string, string> = {}
    if (opts.domain) query.domain = opts.domain
    if (opts.reporter) query.reporter = opts.reporter
    if (opts.from) query.from = opts.from
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
