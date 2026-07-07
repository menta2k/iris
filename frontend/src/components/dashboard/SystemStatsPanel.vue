<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import UsageMeter from '@/components/monitor/UsageMeter.vue'
import { systemMonitorService } from '@/services'
import { ApiError } from '@/services/http'
import type { MonitorSettings, SystemSnapshot } from '@/types'

const snapshot = ref<SystemSnapshot | null>(null)
const settings = ref<MonitorSettings | null>(null)
const error = ref<string | null>(null)
const notImplemented = ref(false)
let timer: ReturnType<typeof setInterval> | undefined

function gb(bytes?: string): string {
  const n = Number(bytes || 0)
  if (n <= 0) return '0'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

async function load() {
  try {
    const res = await systemMonitorService.get()
    snapshot.value = res.snapshot ?? null
    settings.value = res.settings ?? null
    error.value = null
    notImplemented.value = false
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else error.value = err instanceof Error ? err.message : 'Failed to load system stats.'
  }
}

onMounted(() => {
  load()
  timer = setInterval(load, 15000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <Card data-testid="system-stats-panel" class="h-100">
    <CardHeader class="pb-2">
      <CardTitle>System Resources</CardTitle>
      <p class="text-caption text-medium-emphasis mb-0">Host CPU, memory and disk</p>
    </CardHeader>
    <CardContent>
      <p v-if="notImplemented" class="py-4 text-center text-body-2 text-medium-emphasis">
        System monitoring not available.
      </p>
      <p v-else-if="error" class="py-4 text-center text-body-2 text-error">{{ error }}</p>
      <p
        v-else-if="!snapshot?.available"
        class="py-4 text-center text-body-2 text-medium-emphasis"
      >
        Collecting first sample…
      </p>
      <div v-else class="d-flex flex-column ga-3">
        <UsageMeter label="CPU" :value="snapshot.cpuPercent" :threshold="settings?.cpuThreshold" />
        <UsageMeter
          label="Memory"
          :value="snapshot.memPercent"
          :threshold="settings?.memThreshold"
          :detail="`${gb(snapshot.memUsedBytes)} / ${gb(snapshot.memTotalBytes)}`"
        />
        <UsageMeter
          v-for="d in snapshot.disks ?? []"
          :key="d.path"
          :label="`Disk ${d.path}`"
          :value="d.usedPercent"
          :threshold="settings?.diskThreshold"
          :detail="`${gb(d.usedBytes)} / ${gb(d.totalBytes)}`"
        />
      </div>
    </CardContent>
  </Card>
</template>
