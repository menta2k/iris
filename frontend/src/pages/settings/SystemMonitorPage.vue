<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import StatTile from '@/components/dashboard/StatTile.vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableEmpty,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import UsageMeter from '@/components/monitor/UsageMeter.vue'
import SystemMetricsPanel from '@/components/monitor/SystemMetricsPanel.vue'
import { useToast } from '@/composables/useToast'
import { formatDateTime } from '@/composables/useTimezone'
import { systemMonitorService } from '@/services'
import { ApiError } from '@/services/http'
import type { MonitorAlert, MonitorSettings, Mount, SystemSnapshot } from '@/types'

const { toast } = useToast()

const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const saving = ref(false)
const testing = ref(false)

const snapshot = ref<SystemSnapshot | null>(null)
const alerts = ref<MonitorAlert[]>([])
const mounts = ref<Mount[]>([])
const spoolPath = ref('')

function bytes(b?: string): string {
  const n = Number(b || 0)
  if (n <= 0) return '0'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${u[i]}`
}

// Which paths are currently in the monitor set (from the free-text field).
const monitored = computed(() => new Set(parseList(form.value.disk_paths)))

function toggleMonitor(path: string) {
  const set = new Set(parseList(form.value.disk_paths))
  if (set.has(path)) set.delete(path)
  else set.add(path)
  form.value.disk_paths = [...set].join(', ')
}

const emptyForm = () => ({
  enabled: false,
  cpu_threshold: 90,
  mem_threshold: 90,
  disk_threshold: 85,
  disk_paths: '/',
  notify_emails: '',
  from_email: '',
  smtp_host: 'localhost:25',
  cooldown_minutes: 30,
  sample_seconds: 30,
})
const form = ref(emptyForm())

// Snapshot of the last saved/loaded form, for the unsaved-changes indicator.
const savedForm = ref(JSON.stringify(emptyForm()))
const dirty = computed(() => JSON.stringify(form.value) !== savedForm.value)

function parseList(s: string): string[] {
  return s.split(/[\s,]+/).map((v) => v.trim()).filter(Boolean)
}

function toSettings(): MonitorSettings {
  return {
    enabled: form.value.enabled,
    cpuThreshold: Number(form.value.cpu_threshold) || 0,
    memThreshold: Number(form.value.mem_threshold) || 0,
    diskThreshold: Number(form.value.disk_threshold) || 0,
    diskPaths: parseList(form.value.disk_paths),
    notifyEmails: parseList(form.value.notify_emails),
    fromEmail: form.value.from_email,
    smtpHost: form.value.smtp_host,
    cooldownMinutes: Number(form.value.cooldown_minutes) || 0,
    sampleSeconds: Number(form.value.sample_seconds) || 0,
  }
}

function applySettings(s: MonitorSettings) {
  form.value = {
    enabled: s.enabled,
    cpu_threshold: s.cpuThreshold,
    mem_threshold: s.memThreshold,
    disk_threshold: s.diskThreshold,
    disk_paths: (s.diskPaths ?? []).join(', '),
    notify_emails: (s.notifyEmails ?? []).join(', '),
    from_email: s.fromEmail,
    smtp_host: s.smtpHost,
    cooldown_minutes: s.cooldownMinutes,
    sample_seconds: s.sampleSeconds,
  }
  savedForm.value = JSON.stringify(form.value)
}

async function load(withSpinner = true) {
  if (withSpinner) loading.value = true
  try {
    const res = await systemMonitorService.get()
    snapshot.value = res.snapshot ?? null
    alerts.value = res.recentAlerts ?? []
    mounts.value = res.mounts ?? []
    spoolPath.value = res.spoolPath ?? ''
    if (res.settings && withSpinner) applySettings(res.settings)
    error.value = null
    notImplemented.value = false
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else error.value = err instanceof Error ? err.message : 'Failed to load system monitor.'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const res = await systemMonitorService.updateSettings(toSettings())
    applySettings(res)
    toast({ title: 'Monitoring settings saved', variant: 'success' })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save settings.'
    toast({ title: 'Save failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function sendTest() {
  testing.value = true
  try {
    const res = await systemMonitorService.test(toSettings())
    if (res.ok) toast({ title: 'Test alert sent', variant: 'success' })
    else toast({ title: 'Test delivery failed', description: res.error, variant: 'destructive' })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Test failed.'
    toast({ title: 'Test failed', description: msg, variant: 'destructive' })
  } finally {
    testing.value = false
  }
}

// ---- KPI tiles (colored against the configured thresholds) ----

function usageColor(value: number | undefined, threshold: number): string {
  if (value === undefined || !snapshot.value?.available) return 'secondary'
  const t = threshold > 0 ? threshold : 90
  if (value >= t) return 'error'
  if (value >= t * 0.85) return 'warning'
  return 'success'
}

const worstDisk = computed(() => {
  const disks = snapshot.value?.disks ?? []
  return disks.reduce<(typeof disks)[number] | null>(
    (worst, d) => (worst === null || d.usedPercent > worst.usedPercent ? d : worst),
    null,
  )
})

const pctText = (v?: number) =>
  snapshot.value?.available && v !== undefined ? `${v.toFixed(1)}%` : '—'

// ---- Live refresh (default on — this is a live operational view) ----

const REFRESH_MS = 15_000

const live = ref(true)
let timer: ReturnType<typeof setInterval> | undefined

watch(
  live,
  (on) => {
    clearInterval(timer)
    if (on) timer = setInterval(() => load(false), REFRESH_MS)
  },
  { immediate: true },
)

onMounted(() => load())
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div>
    <PageHeader
      title="System Monitor"
      description="Host CPU, memory, and disk usage with email alerts when a resource crosses its threshold."
    >
      <template #actions>
        <v-switch
          v-model="live"
          label="Live"
          color="primary"
          density="compact"
          hide-details
          class="mr-2 flex-grow-0"
        />
        <v-btn
          icon="mdi-refresh"
          variant="text"
          size="small"
          :loading="loading"
          aria-label="Refresh"
          title="Refresh"
          @click="load()"
        />
      </template>
    </PageHeader>

    <DataState :loading="loading && !snapshot" :error="error" :not-implemented="notImplemented" :empty="false">
      <div class="d-flex flex-column ga-4">
        <v-row dense>
          <v-col cols="12" sm="6" lg="3">
            <StatTile
              label="Monitoring"
              :value="form.enabled ? 'Enabled' : 'Disabled'"
              :caption="form.enabled ? `Alerts every ≤ ${form.cooldown_minutes} min per resource` : 'Email alerts are off'"
              icon="mdi-monitor-eye"
              :color="form.enabled ? 'success' : 'secondary'"
            />
          </v-col>
          <v-col cols="12" sm="6" lg="3">
            <StatTile
              label="CPU"
              :value="pctText(snapshot?.cpuPercent)"
              :caption="`Threshold ${form.cpu_threshold}%`"
              icon="mdi-chip"
              :color="usageColor(snapshot?.cpuPercent, form.cpu_threshold)"
            />
          </v-col>
          <v-col cols="12" sm="6" lg="3">
            <StatTile
              label="Memory"
              :value="pctText(snapshot?.memPercent)"
              :caption="snapshot?.available ? `${bytes(snapshot.memUsedBytes)} / ${bytes(snapshot.memTotalBytes)}` : `Threshold ${form.mem_threshold}%`"
              icon="mdi-memory"
              :color="usageColor(snapshot?.memPercent, form.mem_threshold)"
            />
          </v-col>
          <v-col cols="12" sm="6" lg="3">
            <StatTile
              label="Busiest Disk"
              :value="pctText(worstDisk?.usedPercent)"
              :caption="worstDisk ? worstDisk.path : `Threshold ${form.disk_threshold}%`"
              icon="mdi-harddisk"
              :color="usageColor(worstDisk?.usedPercent, form.disk_threshold)"
            />
          </v-col>
        </v-row>

        <!-- Live usage -->
        <Card>
          <CardHeader class="pb-2">
            <div class="d-flex flex-wrap align-center justify-space-between ga-2">
              <div>
                <CardTitle>Current Usage</CardTitle>
                <p class="text-caption text-medium-emphasis mb-0">
                  <template v-if="snapshot?.available">Sampled {{ formatDateTime(snapshot.collectedAt) }}</template>
                  <template v-else>Waiting for the first sample</template>
                </p>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <p v-if="!snapshot?.available" class="py-2 text-body-2 text-medium-emphasis">Collecting first sample…</p>
            <div v-else class="d-flex flex-column ga-3">
              <UsageMeter label="CPU" :value="snapshot.cpuPercent" :threshold="form.cpu_threshold" />
              <UsageMeter
                label="Memory"
                :value="snapshot.memPercent"
                :threshold="form.mem_threshold"
                :detail="`${bytes(snapshot.memUsedBytes)} / ${bytes(snapshot.memTotalBytes)}`"
              />
              <UsageMeter
                v-for="d in snapshot.disks ?? []"
                :key="d.path"
                :label="`Disk ${d.path}`"
                :value="d.usedPercent"
                :threshold="form.disk_threshold"
                :detail="`${bytes(d.usedBytes)} / ${bytes(d.totalBytes)}`"
              />
            </div>
            <p class="mt-3 text-caption text-medium-emphasis">
              These metrics are also exported to Prometheus (<span class="font-mono">iris_system_cpu_percent</span>,
              <span class="font-mono">iris_system_memory_percent</span>,
              <span class="font-mono">iris_system_disk_used_percent{'{'}path{'}'}</span>) for charting in Grafana.
            </p>
          </CardContent>
        </Card>

        <!-- History -->
        <SystemMetricsPanel />

        <!-- Settings -->
        <Card>
          <CardHeader class="pb-2">
            <div class="d-flex flex-wrap align-center justify-space-between ga-2">
              <div>
                <CardTitle>Alerting</CardTitle>
                <p class="text-caption text-medium-emphasis mb-0">
                  Thresholds, monitored disks and email delivery
                </p>
              </div>
              <v-chip v-if="dirty" size="small" color="warning" variant="tonal">Unsaved changes</v-chip>
            </div>
          </CardHeader>
          <CardContent>
            <form class="d-flex flex-column ga-5" @submit.prevent="save">
              <v-switch
                v-model="form.enabled"
                color="primary"
                density="compact"
                hide-details
                label="Enable email alerts on threshold breaches"
              />

              <div>
                <p class="text-overline text-medium-emphasis mb-2">Thresholds</p>
                <v-row dense>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-cpu"
                      v-model.number="form.cpu_threshold"
                      label="CPU"
                      type="number"
                      suffix="%"
                      placeholder="90"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-mem"
                      v-model.number="form.mem_threshold"
                      label="Memory"
                      type="number"
                      suffix="%"
                      placeholder="90"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-disk"
                      v-model.number="form.disk_threshold"
                      label="Disk"
                      type="number"
                      suffix="%"
                      placeholder="85"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                </v-row>
                <p class="mt-1 text-caption text-medium-emphasis mb-0">
                  A threshold of 0 disables that resource's check.
                </p>
              </div>

              <div class="d-flex flex-column ga-2">
                <p class="text-overline text-medium-emphasis mb-0">Disks to monitor</p>
                <div v-if="mounts.length" class="rounded border">
                  <div
                    v-for="m in mounts"
                    :key="m.path"
                    class="d-flex align-center ga-3 px-3 py-2 mount-row"
                  >
                    <v-checkbox
                      :model-value="monitored.has(m.path)"
                      color="primary"
                      density="compact"
                      hide-details
                      @update:model-value="toggleMonitor(m.path)"
                    />
                    <div class="flex-grow-1">
                      <div class="d-flex align-center ga-2">
                        <span class="font-mono text-body-2">{{ m.path }}</span>
                        <span v-if="spoolPath && (m.path === spoolPath || spoolPath.startsWith(m.path === '/' ? '/' : m.path + '/'))" class="text-caption text-primary">· holds KumoMTA spool</span>
                      </div>
                      <div class="text-caption text-medium-emphasis">
                        {{ m.device }} · {{ m.fstype }} · {{ bytes(m.usedBytes) }} / {{ bytes(m.totalBytes) }}
                      </div>
                    </div>
                    <span class="text-body-2 tabular-nums" :class="m.usedPercent >= form.disk_threshold ? 'text-error' : 'text-medium-emphasis'">
                      {{ m.usedPercent.toFixed(1) }}%
                    </span>
                  </div>
                </div>
                <p v-else class="text-caption text-medium-emphasis mb-0">No filesystems detected yet.</p>
                <div v-if="spoolPath && !monitored.has(spoolPath)" class="d-flex align-center ga-2">
                  <span class="text-caption text-medium-emphasis">Spool is at {{ spoolPath }} —</span>
                  <Button type="button" variant="outline" size="sm" @click="toggleMonitor(spoolPath)">
                    Monitor spool path
                  </Button>
                </div>
                <v-text-field
                  v-model="form.disk_paths"
                  label="Monitored paths"
                  placeholder="/, /var/spool/kumomta"
                  variant="outlined"
                  density="compact"
                  hide-details
                />
                <p class="text-caption text-medium-emphasis mb-0">
                  Tick a detected filesystem, or type paths directly (comma-separated). A path that
                  isn't its own mount is measured on the filesystem that contains it.
                </p>
              </div>

              <div>
                <p class="text-overline text-medium-emphasis mb-2">Notifications</p>
                <v-row dense>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-to"
                      v-model="form.notify_emails"
                      label="Notify emails"
                      placeholder="ops@example.com"
                      prepend-inner-icon="mdi-email-outline"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-from"
                      v-model="form.from_email"
                      label="From address"
                      placeholder="iris@example.com"
                      prepend-inner-icon="mdi-email-arrow-right-outline"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-smtp"
                      v-model="form.smtp_host"
                      label="SMTP host"
                      placeholder="localhost:25"
                      prepend-inner-icon="mdi-server-network-outline"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                </v-row>
              </div>

              <div>
                <p class="text-overline text-medium-emphasis mb-2">Timing</p>
                <v-row dense>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-cooldown"
                      v-model.number="form.cooldown_minutes"
                      label="Alert cooldown"
                      type="number"
                      suffix="min"
                      placeholder="30"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" md="4">
                    <v-text-field
                      id="m-sample"
                      v-model.number="form.sample_seconds"
                      label="Sample interval"
                      type="number"
                      suffix="s"
                      placeholder="30"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                </v-row>
                <p class="mt-1 text-caption text-medium-emphasis mb-0">
                  Alerts repeat at most once per cooldown per resource; a recovery notice is sent
                  when it drops back. SMTP host defaults to the local KumoMTA loopback.
                </p>
              </div>

              <div class="d-flex ga-2">
                <Button type="submit" :disabled="saving">{{ saving ? 'Saving…' : 'Save settings' }}</Button>
                <Button type="button" variant="outline" :disabled="testing" @click="sendTest">
                  {{ testing ? 'Sending…' : 'Send test alert' }}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>

        <!-- Recent alerts -->
        <Card>
          <CardHeader class="pb-2">
            <CardTitle>Recent Alerts</CardTitle>
            <p class="text-caption text-medium-emphasis mb-0">Threshold breaches and recoveries</p>
          </CardHeader>
          <CardContent class="pa-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>Level</TableHead>
                  <TableHead class="text-right">Value</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Emailed</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableEmpty v-if="alerts.length === 0" :colspan="6" message="No alerts recorded." />
                <TableRow v-for="a in alerts" :key="a.id">
                  <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(a.createdAt) }}</TableCell>
                  <TableCell class="text-no-wrap">{{ a.resource }}{{ a.detail ? ` ${a.detail}` : '' }}</TableCell>
                  <TableCell>
                    <Badge :variant="a.level === 'recovered' ? 'success' : 'destructive'">{{ a.level }}</Badge>
                  </TableCell>
                  <TableCell class="text-right tabular-nums text-no-wrap">
                    {{ a.value.toFixed(1) }}%
                    <span v-if="a.threshold" class="text-caption text-medium-emphasis">/ {{ a.threshold }}%</span>
                  </TableCell>
                  <TableCell class="text-medium-emphasis">{{ a.message }}</TableCell>
                  <TableCell>
                    <Badge :variant="a.notified ? 'secondary' : 'outline'">{{ a.notified ? 'yes' : 'no' }}</Badge>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </DataState>
  </div>
</template>

<style scoped>
.mount-row:not(:last-child) {
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.1);
}
</style>
