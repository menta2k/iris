import { http } from './http'
import type {
  MonitorSettings,
  SystemMonitor,
  TestMonitorNotificationResult,
} from '@/types'

export const systemMonitorService = {
  get() {
    return http.get<SystemMonitor>('/system-monitor')
  },
  updateSettings(settings: MonitorSettings) {
    return http.put<MonitorSettings>('/system-monitor/settings', { settings })
  },
  test(settings: MonitorSettings) {
    return http.post<TestMonitorNotificationResult>('/system-monitor:test', { settings })
  },
}
