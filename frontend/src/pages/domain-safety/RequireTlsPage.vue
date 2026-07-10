<script setup lang="ts">
import { ref } from 'vue'
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
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { domainSafetyService } from '@/services'
import { ApiError } from '@/services/http'
import type { TLSPolicy, TLSPolicyMode } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<TLSPolicy>({
  loader: () => domainSafetyService.listTLSPolicies(),
})
const { toast } = useToast()

const MODES: { value: TLSPolicyMode; label: string }[] = [
  { value: 'required', label: 'Required — STARTTLS + valid cert (fail if unavailable)' },
  { value: 'required_insecure', label: 'Required insecure — STARTTLS, skip cert (fail if unavailable)' },
  { value: 'opportunistic_insecure', label: 'Opportunistic insecure — try TLS, fall back to cleartext' },
  { value: 'disabled', label: 'Disabled — never use TLS, deliver in cleartext' },
]
const MODE_ITEMS = MODES.map((m) => ({ title: m.label, value: m.value }))

const dialogOpen = ref(false)
const saving = ref(false)
const deletingId = ref<string | null>(null)
const form = ref<{ domain: string; mode: TLSPolicyMode }>({ domain: '', mode: 'required' })

function openCreate() {
  form.value = { domain: '', mode: 'required' }
  dialogOpen.value = true
}

async function submit() {
  if (!form.value.domain) return
  saving.value = true
  try {
    await domainSafetyService.createTLSPolicy({ domain: form.value.domain, mode: form.value.mode })
    toast({ title: 'TLS policy added', description: form.value.domain, variant: 'success' })
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save policy.'
    toast({ title: 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function remove(p: TLSPolicy) {
  deletingId.value = p.id
  try {
    await domainSafetyService.deleteTLSPolicy(p.id)
    toast({ title: 'TLS policy removed', description: p.domain, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete policy.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  } finally {
    deletingId.value = null
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="TLS Policy"
      description="Per-destination-domain TLS policy for outbound delivery. Require TLS (fail rather than send in cleartext) for sensitive domains, or relax/disable it for receivers whose broken or legacy certificate kumod cannot negotiate — Disabled delivers in cleartext so mail gets through instead of deferring and bouncing."
    >
      <template #actions>
        <Button data-testid="create-tls-policy" @click="openCreate">Add Domain</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No TLS policies configured. Domains without a policy use opportunistic TLS (encrypt if offered, never hard-fail)."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="p in items" :key="p.id">
                <TableCell class="font-weight-medium">{{ p.domain }}</TableCell>
                <TableCell>
                  <Badge :variant="p.mode === 'disabled' ? 'warning' : p.mode === 'opportunistic_insecure' ? 'secondary' : 'success'">{{ p.mode }}</Badge>
                </TableCell>
                <TableCell><StatusBadge :status="p.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :disabled="deletingId === p.id"
                    :data-testid="`delete-tls-policy-${p.id}`"
                    @click="remove(p)"
                  >
                    {{ deletingId === p.id ? 'Removing…' : 'Remove' }}
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>Add TLS Policy</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="tls-domain">Destination domain</Label>
          <Input id="tls-domain" v-model="form.domain" placeholder="secure.example.com" />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="tls-mode">Mode</Label>
          <v-select
            id="tls-mode"
            v-model="form.mode"
            :items="MODE_ITEMS"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.domain">
            {{ saving ? 'Saving…' : 'Add Domain' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
