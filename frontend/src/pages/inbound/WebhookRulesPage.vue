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
import { StatusBadge, Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { inboundAutomationService } from '@/services'
import { ApiError } from '@/services/http'
import type { WebhookRule } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<WebhookRule>({
  loader: () => inboundAutomationService.listWebhookRules(),
})
const { toast } = useToast()

const WEBHOOK_STATUSES = ['active', 'disabled']

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref<{
  name: string
  match_type: 'recipient_email' | 'recipient_domain'
  match_value: string
  destination_url: string
  secret_ref: string
  status: string
  timeout_seconds: number
}>({
  name: '',
  match_type: 'recipient_domain',
  match_value: '',
  destination_url: '',
  secret_ref: '',
  status: 'active',
  timeout_seconds: 30,
})

const isEdit = computed(() => mode.value === 'edit')

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = {
    name: '',
    match_type: 'recipient_domain',
    match_value: '',
    destination_url: '',
    secret_ref: '',
    status: 'active',
    timeout_seconds: 30,
  }
  dialogOpen.value = true
}

function openEdit(w: WebhookRule) {
  mode.value = 'edit'
  editId.value = w.id
  form.value = {
    name: w.name,
    match_type: (w.matchType as 'recipient_email' | 'recipient_domain') || 'recipient_domain',
    match_value: w.matchValue,
    destination_url: w.destinationUrl,
    // Never display the existing secret; blank preserves it.
    secret_ref: '',
    status: (w.status || 'active').toLowerCase(),
    timeout_seconds: w.timeoutSeconds,
  }
  dialogOpen.value = true
}

async function submit() {
  if (!form.value.name || !form.value.destination_url) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await inboundAutomationService.updateWebhookRule(editId.value, {
        ...form.value,
        timeout_seconds: Number(form.value.timeout_seconds),
      })
      toast({ title: 'Webhook rule updated', description: form.value.name, variant: 'success' })
    } else {
      await inboundAutomationService.createWebhookRule({
        name: form.value.name,
        match_type: form.value.match_type,
        match_value: form.value.match_value,
        destination_url: form.value.destination_url,
        secret_ref: form.value.secret_ref,
        timeout_seconds: Number(form.value.timeout_seconds),
      })
      toast({ title: 'Webhook rule created', description: form.value.name, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save webhook rule.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Webhook Rules" description="Forward inbound events to external https endpoints.">
      <template #actions>
        <Button data-testid="create-webhook-rule" @click="openCreate">New Rule</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No webhook rules configured."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Match Type</TableHead>
                <TableHead>Match Value</TableHead>
                <TableHead>Destination</TableHead>
                <TableHead>Timeout</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="w in items" :key="w.id">
                <TableCell class="font-medium">{{ w.name }}</TableCell>
                <TableCell><Badge variant="outline">{{ w.matchType }}</Badge></TableCell>
                <TableCell class="font-mono text-xs">{{ w.matchValue }}</TableCell>
                <TableCell class="font-mono text-xs">{{ w.destinationUrl }}</TableCell>
                <TableCell class="tabular-nums">{{ w.timeoutSeconds }}s</TableCell>
                <TableCell><StatusBadge :status="w.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-webhook-rule-${w.id}`"
                    @click="openEdit(w)"
                  >
                    Edit
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
        <DialogTitle>{{ isEdit ? 'Edit Webhook Rule' : 'Create Webhook Rule' }}</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="wh-name">Name</Label>
          <Input id="wh-name" v-model="form.name" placeholder="notify-billing" />
        </div>
        <div class="space-y-1.5">
          <Label for="wh-match-type">Match Type</Label>
          <Select id="wh-match-type" v-model="form.match_type">
            <option value="recipient_email">recipient_email</option>
            <option value="recipient_domain">recipient_domain</option>
          </Select>
        </div>
        <div class="space-y-1.5">
          <Label for="wh-match-value">Match Value</Label>
          <Input id="wh-match-value" v-model="form.match_value" placeholder="example.com" />
        </div>
        <div class="space-y-1.5">
          <Label for="wh-url">Destination URL (https)</Label>
          <Input id="wh-url" v-model="form.destination_url" placeholder="https://hooks.example.com/iris" />
        </div>
        <div class="space-y-1.5">
          <Label for="wh-secret">Secret Reference</Label>
          <Input id="wh-secret" v-model="form.secret_ref" placeholder="secret://webhooks/billing" />
          <p v-if="isEdit" class="text-xs text-muted-foreground">
            Leave blank to keep the existing secret.
          </p>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="wh-timeout">Timeout (seconds)</Label>
            <Input id="wh-timeout" v-model="form.timeout_seconds" type="number" min="1" placeholder="30" />
          </div>
          <div v-if="isEdit" class="space-y-1.5">
            <Label for="wh-status">Status</Label>
            <Select id="wh-status" v-model="form.status">
              <option v-for="s in WEBHOOK_STATUSES" :key="s" :value="s">{{ s }}</option>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.name || !form.destination_url">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
