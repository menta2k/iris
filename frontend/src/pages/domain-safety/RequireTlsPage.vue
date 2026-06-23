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
import { Select } from '@/components/ui/select'
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
  { value: 'required', label: 'required (STARTTLS + valid cert)' },
  { value: 'required_insecure', label: 'required_insecure (STARTTLS, skip cert)' },
]

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
    toast({ title: 'Require-TLS domain added', description: form.value.domain, variant: 'success' })
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
    toast({ title: 'Require-TLS domain removed', description: p.domain, variant: 'success' })
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
      title="Require TLS"
      description="Destination domains that must be delivered over TLS. When STARTTLS is unavailable, kumod refuses to send in cleartext (the delivery is rejected and logged)."
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
      empty-message="No require-TLS domains configured."
    >
      <Card>
        <CardContent class="p-0">
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
                <TableCell class="font-medium">{{ p.domain }}</TableCell>
                <TableCell><Badge variant="outline">{{ p.mode }}</Badge></TableCell>
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
        <DialogTitle>Add Require-TLS Domain</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="tls-domain">Destination domain</Label>
          <Input id="tls-domain" v-model="form.domain" placeholder="secure.example.com" />
        </div>
        <div class="space-y-1.5">
          <Label for="tls-mode">Mode</Label>
          <Select id="tls-mode" v-model="form.mode">
            <option v-for="m in MODES" :key="m.value" :value="m.value">{{ m.label }}</option>
          </Select>
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
