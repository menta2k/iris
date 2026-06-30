import { http } from './http'
import type { DashboardSummary, WarmupDeliveryStats } from '@/types'

// Lookback windows for the warmup delivery/bounce-rate panel.
export type WarmupStatsRange = '24h' | '7d'

export const dashboardService = {
  getSummary() {
    return http.get<DashboardSummary>('/dashboard/summary')
  },
  getWarmupStats(range: WarmupStatsRange = '24h') {
    return http.get<WarmupDeliveryStats>(
      `/dashboard/warmup-stats?range=${encodeURIComponent(range)}`,
    )
  },
}
