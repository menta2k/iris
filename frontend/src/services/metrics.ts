import { http } from './http'
import type { MetricsTimeseries } from '@/types'

export type MetricsRange = '1h' | '6h' | '24h' | '7d'

export const metricsService = {
  getTimeseries(range: MetricsRange = '6h') {
    return http.get<MetricsTimeseries>(`/dashboard/metrics?range=${encodeURIComponent(range)}`)
  },
}
