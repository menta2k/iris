import { http } from './http'
import type { MetricsTimeseries, QueueTimeHistogram } from '@/types'

export type MetricsRange = '1h' | '6h' | '24h' | '7d'

export const metricsService = {
  getTimeseries(range: MetricsRange = '6h') {
    return http.get<MetricsTimeseries>(`/dashboard/metrics?range=${encodeURIComponent(range)}`)
  },
  // Delivery queue-time distribution; mailclass empty = global (all classes).
  getQueueTimeHistogram(range: MetricsRange = '6h', mailclass = '') {
    const query: Record<string, string> = { range }
    if (mailclass) query.mailclass = mailclass
    return http.get<QueueTimeHistogram>('/dashboard/queue-time-histogram', { query })
  },
}
