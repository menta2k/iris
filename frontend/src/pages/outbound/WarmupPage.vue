<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useToast } from '@/composables/useToast'
import { warmupService, outboundConfigService } from '@/services'
import { ApiError } from '@/services/http'
import type { VMTA, WarmupCurve, WarmupSchedule, WarmupStage, WarmupStageInput } from '@/types'

const { toast } = useToast()

const BUCKETS = ['gmail', 'microsoft', 'yahoo', 'default'] as const

const items = ref<WarmupSchedule[]>([])
const curves = ref<WarmupCurve[]>([])
const vmtas = ref<VMTA[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref({
  vmta_id: '',
  curve: 'standard',
  start_date: todayISO(),
  stages: [] as WarmupStageInput[],
})

function todayISO(): string {
  return new Date().toISOString().slice(0, 10)
}

const curveNames = computed(() => curves.value.map((c) => c.name))
// The built-in templates plus the 'custom' option that reveals the stage editor.
const curveOptions = computed(() => [...curveNames.value, 'custom'])
const isEdit = computed(() => mode.value === 'edit')
const isCustom = computed(() => form.value.curve === 'custom')

// v-select item lists ({ title, value }) for the VMTA and curve dropdowns.
const vmtaItems = computed(() => [
  { title: '— Select a VMTA —', value: '' },
  ...vmtas.value.map((v) => ({ title: `${v.name} (${v.ipAddress})`, value: v.id })),
])
const curveItems = computed(() => curveOptions.value.map((c) => ({ title: c, value: c })))

function newStage(dayFrom: number): WarmupStageInput {
  return { day_from: dayFrom, day_to: dayFrom + 6, caps: { gmail: 50, microsoft: 50, yahoo: 50, default: 200 } }
}
function addStage() {
  const last = form.value.stages[form.value.stages.length - 1]
  form.value.stages.push(newStage(last ? last.day_to + 1 : 1))
}
function removeStage(i: number) {
  form.value.stages.splice(i, 1)
}

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await warmupService.list()
    items.value = res.items ?? []
    curves.value = res.curves ?? []
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else if (err instanceof ApiError && err.status === 0)
      error.value = 'Cannot reach the backend. Is the API server running?'
    else error.value = err instanceof Error ? err.message : 'Failed to load warmup schedules.'
  } finally {
    loading.value = false
  }
}

async function loadVmtas() {
  try {
    const res = await outboundConfigService.listVmtas('active')
    vmtas.value = res.items ?? []
  } catch {
    vmtas.value = []
  }
}

async function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = {
    vmta_id: '',
    curve: curveNames.value[0] ?? 'standard',
    start_date: todayISO(),
    stages: [newStage(1)],
  }
  dialogOpen.value = true
  await loadVmtas()
}

function openEdit(w: WarmupSchedule) {
  mode.value = 'edit'
  editId.value = w.id
  form.value = {
    vmta_id: w.vmtaId,
    curve: w.curve,
    start_date: w.startDate,
    // Seed the editor from the schedule's resolved stages (so a template can be
    // tweaked into a custom curve, or a custom curve edited).
    stages: (w.stages ?? []).map((s) => ({
      day_from: s.dayFrom,
      day_to: s.dayTo,
      caps: { ...s.caps },
    })),
  }
  if (form.value.stages.length === 0) form.value.stages = [newStage(1)]
  dialogOpen.value = true
}

async function submit() {
  if (!form.value.curve || (!isEdit.value && !form.value.vmta_id)) return
  saving.value = true
  try {
    const stages = isCustom.value ? form.value.stages : undefined
    if (isEdit.value && editId.value) {
      await warmupService.update(editId.value, {
        start_date: form.value.start_date,
        curve: form.value.curve,
        stages,
      })
      toast({ title: 'Warmup updated', variant: 'success' })
    } else {
      await warmupService.create({
        vmta_id: form.value.vmta_id,
        start_date: form.value.start_date,
        curve: form.value.curve,
        stages,
      })
      toast({ title: 'Warmup scheduled', variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save warmup.'
    toast({ title: 'Save failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function pause(w: WarmupSchedule) {
  await act(() => warmupService.pause(w.id, { reason: 'paused by operator' }), 'Warmup paused')
}
async function resume(w: WarmupSchedule) {
  await act(() => warmupService.resume(w.id), 'Warmup resumed')
}
async function act(fn: () => Promise<unknown>, ok: string) {
  try {
    await fn()
    toast({ title: ok, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Action failed.'
    toast({ title: 'Action failed', description: msg, variant: 'destructive' })
  }
}

// ---- client-side ramp math (mirrors the backend for display only) ----
function durationOf(w: WarmupSchedule): number {
  return w.stages.reduce((m, s) => Math.max(m, s.dayTo), 0)
}
function currentDay(w: WarmupSchedule): number {
  if (w.status === 'paused' && w.heldDay) return w.heldDay
  const start = new Date(w.startDate + 'T00:00:00Z').getTime()
  const today = new Date(todayISO() + 'T00:00:00Z').getTime()
  return Math.floor((today - start) / 86_400_000) + 1
}
function stageForDay(w: WarmupSchedule, day: number): WarmupStage | undefined {
  return w.stages.find((s) => day >= s.dayFrom && day <= s.dayTo)
}
function capsToday(w: WarmupSchedule): string {
  const day = currentDay(w)
  const s = stageForDay(w, day)
  if (!s || w.status === 'completed') return 'no cap'
  return BUCKETS.map((b) => `${b[0].toUpperCase()}:${(s.caps[b] || s.caps.default || 0).toLocaleString()}`).join('  ')
}
function progress(w: WarmupSchedule): string {
  const dur = durationOf(w)
  if (w.status === 'completed') return `done (${dur}d)`
  if (w.status === 'scheduled') return `starts ${w.startDate}`
  const day = Math.min(Math.max(currentDay(w), 1), dur)
  return `day ${day} / ${dur}`
}

load()
</script>

<template>
  <div>
    <PageHeader
      title="IP Warmup"
      description="Ramp a VMTA's outbound volume per mailbox provider over a curve to build sender reputation."
    >
      <template #actions>
        <Button data-testid="create-warmup" @click="openCreate">New warmup</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No warmup schedules yet."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>VMTA</TableHead>
                <TableHead>Curve</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Progress</TableHead>
                <TableHead>Today's caps (per day)</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="w in items" :key="w.id">
                <TableCell class="font-weight-medium">{{ w.vmtaName }}</TableCell>
                <TableCell>{{ w.curve }}</TableCell>
                <TableCell><StatusBadge :status="w.status" /></TableCell>
                <TableCell class="tabular-nums">{{ progress(w) }}</TableCell>
                <TableCell class="font-mono text-caption">{{ capsToday(w) }}</TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-1">
                    <Button
                      v-if="w.status === 'scheduled' || w.status === 'active'"
                      variant="outline"
                      size="sm"
                      @click="openEdit(w)"
                    >
                      Edit
                    </Button>
                    <Button
                      v-if="w.status === 'active'"
                      variant="outline"
                      size="sm"
                      @click="pause(w)"
                    >
                      Pause
                    </Button>
                    <Button v-if="w.status === 'paused'" variant="outline" size="sm" @click="resume(w)">
                      Resume
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit warmup' : 'New warmup' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div v-if="!isEdit" class="d-flex flex-column ga-1">
          <Label for="warmup-vmta">VMTA</Label>
          <v-select
            id="warmup-vmta"
            v-model="form.vmta_id"
            :items="vmtaItems"
            data-testid="warmup-vmta"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="warmup-curve">Curve</Label>
          <v-select
            id="warmup-curve"
            v-model="form.curve"
            :items="curveItems"
            variant="outlined"
            density="compact"
            hide-details
          />
          <p class="text-caption text-medium-emphasis">
            Built-in templates: standard (~21d), conservative (~30d), aggressive (~12d). Pick
            <code>custom</code> to define your own stages. Caps are per receiving-domain family
            (Gmail, Microsoft, Yahoo, default).
          </p>
        </div>

        <!-- Custom stage editor: contiguous day ranges with a per-MBP daily cap. -->
        <div v-if="isCustom" class="d-flex flex-column ga-2 rounded border pa-3">
          <div class="d-flex align-center justify-space-between">
            <Label>Stages (messages/day)</Label>
            <Button type="button" variant="outline" size="sm" @click="addStage">Add stage</Button>
          </div>
          <table class="w-100 text-caption">
            <thead class="text-medium-emphasis">
              <tr>
                <th class="text-left font-weight-regular">Day from</th>
                <th class="text-left font-weight-regular">Day to</th>
                <th class="text-left font-weight-regular">Gmail</th>
                <th class="text-left font-weight-regular">Microsoft</th>
                <th class="text-left font-weight-regular">Yahoo</th>
                <th class="text-left font-weight-regular">Default</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(s, i) in form.stages" :key="i">
                <td class="pr-1"><Input v-model.number="s.day_from" type="number" /></td>
                <td class="pr-1"><Input v-model.number="s.day_to" type="number" /></td>
                <td class="pr-1"><Input v-model.number="s.caps.gmail" type="number" /></td>
                <td class="pr-1"><Input v-model.number="s.caps.microsoft" type="number" /></td>
                <td class="pr-1"><Input v-model.number="s.caps.yahoo" type="number" /></td>
                <td class="pr-1"><Input v-model.number="s.caps.default" type="number" /></td>
                <td>
                  <Button type="button" variant="ghost" size="sm" @click="removeStage(i)">✕</Button>
                </td>
              </tr>
            </tbody>
          </table>
          <p class="text-caption text-medium-emphasis">
            Stages must be 1-based and contiguous (each starts the day after the previous ends).
            After the last day the warmup completes and the cap is removed.
          </p>
        </div>

        <div class="d-flex flex-column ga-1">
          <Label for="warmup-start">Start date</Label>
          <Input id="warmup-start" v-model="form.start_date" type="date" />
          <p class="text-caption text-medium-emphasis">Day 1 of the ramp (UTC).</p>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.curve || (!isEdit && !form.vmta_id)">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
