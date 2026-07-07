// System self-monitoring: live snapshot, settings (mutable), test, alert history.

import type { MonitorSettings } from '../../types'
import { ok, type Route } from '../router'

const GB = 1024 ** 3

let settings: MonitorSettings = {
  enabled: false,
  cpuThreshold: 90,
  memThreshold: 90,
  diskThreshold: 85,
  diskPaths: ['/', '/var/spool/kumomta'],
  notifyEmails: [],
  fromEmail: '',
  smtpHost: 'localhost:25',
  cooldownMinutes: 30,
  sampleSeconds: 30,
}

function snapshot() {
  const cpu = 18 + Math.random() * 28
  const mem = 54 + Math.random() * 12
  return {
    collectedAt: new Date().toISOString(),
    cpuPercent: cpu,
    memPercent: mem,
    memUsedBytes: String(Math.round((mem / 100) * 16 * GB)),
    memTotalBytes: String(16 * GB),
    disks: [
      { path: '/', usedPercent: 62.3, usedBytes: String(Math.round(62.3 * GB)), totalBytes: String(100 * GB) },
      { path: '/var/spool/kumomta', usedPercent: 41.0, usedBytes: String(205 * GB), totalBytes: String(500 * GB) },
    ],
    available: true,
  }
}

const recentAlerts = [
  {
    id: 'ma_1', resource: 'disk', detail: '/', level: 'breached', value: 87.4, threshold: 85,
    message: 'Disk / at 87.4% (threshold 85%)', notified: true,
    createdAt: new Date(Date.now() - 3600_000).toISOString(),
  },
  {
    id: 'ma_2', resource: 'disk', detail: '/', level: 'recovered', value: 78.1, threshold: 85,
    message: 'Disk / recovered to 78.1% (threshold 85%)', notified: true,
    createdAt: new Date(Date.now() - 1800_000).toISOString(),
  },
]

export const systemMonitorRoutes: Route[] = [
  { method: 'GET', pattern: '/system-monitor', handler: () => ok({ snapshot: snapshot(), settings, recentAlerts }) },
  {
    method: 'PUT',
    pattern: '/system-monitor/settings',
    handler: (ctx) => {
      const body = ctx.body as { settings: MonitorSettings }
      settings = { ...settings, ...body.settings }
      return ok(settings)
    },
  },
  { method: 'POST', pattern: '/system-monitor:test', handler: () => ok({ ok: true }) },
]
