import { http } from './http'
import type { DashboardSummary } from '@/types'

export const dashboardService = {
  getSummary() {
    return http.get<DashboardSummary>('/dashboard/summary')
  },
}
