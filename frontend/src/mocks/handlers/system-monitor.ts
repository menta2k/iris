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

const mounts = [
  { path: '/', device: '/dev/sda1', fstype: 'ext4', usedPercent: 62.3, usedBytes: String(Math.round(62.3 * GB)), totalBytes: String(100 * GB) },
  { path: '/var/spool/kumomta', device: '/dev/sdb1', fstype: 'xfs', usedPercent: 41.0, usedBytes: String(205 * GB), totalBytes: String(500 * GB) },
  { path: '/boot', device: '/dev/sda2', fstype: 'ext4', usedPercent: 18.5, usedBytes: String(Math.round(0.185 * GB)), totalBytes: String(1 * GB) },
]

function metrics(range: string) {
  const spanMs =
    ({ '1h': 3600e3, '6h': 6 * 3600e3, '24h': 24 * 3600e3, '7d': 7 * 24 * 3600e3 } as Record<string, number>)[
      range
    ] ?? 6 * 3600e3
  const n = 60
  const step = spanMs / n
  const now = Date.now()
  const line = (base: number, amp: number, phase: number) =>
    Array.from({ length: n }, (_, i) => ({
      timestamp: Math.round((now - (n - 1 - i) * step) / 1000),
      value: Number(Math.max(0, Math.min(100, base + amp * Math.sin(i / 6 + phase) + (Math.random() - 0.5) * 4)).toFixed(1)),
    }))
  return {
    range,
    stepSeconds: Math.round(step / 1000),
    prometheusAvailable: true,
    series: [
      { key: 'cpu', label: 'CPU %', points: line(35, 18, 0) },
      { key: 'memory', label: 'Memory %', points: line(60, 6, 1) },
      { key: 'disk:/', label: 'Disk /', points: line(62, 1.5, 2) },
      { key: 'disk:/var/spool/kumomta', label: 'Disk /var/spool/kumomta', points: line(41, 3, 3) },
    ],
  }
}

export const systemMonitorRoutes: Route[] = [
  {
    method: 'GET',
    pattern: '/system-monitor/metrics',
    handler: (ctx) => ok(metrics((ctx.query.range || '6h').toString())),
  },
  {
    method: 'GET',
    pattern: '/system-monitor',
    handler: () => ok({ snapshot: snapshot(), settings, recentAlerts, mounts, spoolPath: '/var/spool/kumomta' }),
  },
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
