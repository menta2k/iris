import { http } from './http'
import type {
  DashboardSummary,
  MailClassStats,
  RecipientDomainStats,
  WarmupDeliveryStats,
} from '@/types'

// Lookback windows for the warmup delivery/bounce-rate panel.
export type WarmupStatsRange = '24h' | '7d'

export const dashboardService = {
  getSummary() {
    return http.get<DashboardSummary>('/dashboard/summary')
  },
  // node empty = all cluster nodes; non-empty narrows to one node.
  getWarmupStats(range: WarmupStatsRange = '24h', node = '') {
    return http.get<WarmupDeliveryStats>('/dashboard/warmup-stats', {
      query: node ? { range, node } : { range },
    })
  },
  getMailClassStats(range: WarmupStatsRange = '24h', node = '') {
    return http.get<MailClassStats>('/dashboard/mailclass-stats', {
      query: node ? { range, node } : { range },
    })
  },
  getRecipientDomainStats(range: WarmupStatsRange = '24h', node = '') {
    return http.get<RecipientDomainStats>('/dashboard/recipient-domain-stats', {
      query: node ? { range, node } : { range },
    })
  },
}
