import { http } from './http'
import type {
  MetricsTimeseries,
  MonitorSettings,
  SystemMonitor,
  TestMonitorNotificationResult,
} from '@/types'

export type SystemMetricsRange = '1h' | '6h' | '24h' | '7d'

export const systemMonitorService = {
  get() {
    return http.get<SystemMonitor>('/system-monitor')
  },
  getMetrics(range: SystemMetricsRange = '6h') {
    return http.get<MetricsTimeseries>('/system-monitor/metrics', { query: { range } })
  },
  updateSettings(settings: MonitorSettings) {
    return http.put<MonitorSettings>('/system-monitor/settings', { settings })
  },
  test(settings: MonitorSettings) {
    return http.post<TestMonitorNotificationResult>('/system-monitor:test', { settings })
  },
}
