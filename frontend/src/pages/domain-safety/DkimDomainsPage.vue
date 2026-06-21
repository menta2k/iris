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
import { Select } from '@/components/ui/select'
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { domainSafetyService } from '@/services'
import { ApiError } from '@/services/http'
import type { DkimDomain } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<DkimDomain>({
  loader: () => domainSafetyService.listDkimDomains(),
})
const { toast } = useToast()

const DKIM_STATUSES = ['ready', 'disabled', 'needs_attention']

const dialogOpen = ref(false)
const saving = ref(false)
const generating = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref({
  domain: '',
  selector: '',
  public_key_fingerprint: '',
  private_key_ref: '',
  status: 'ready',
})
// The DNS TXT record to publish, shown after generating a key pair.
const dnsRecord = ref<{ name: string; value: string } | null>(null)

const isEdit = computed(() => mode.value === 'edit')

function resetForm() {
  form.value = {
    domain: '',
    selector: '',
    public_key_fingerprint: '',
    private_key_ref: '',
    status: 'ready',
  }
  dnsRecord.value = null
}

function openCreate() {
  mode.value = 'create'
  editId.value = null
  resetForm()
  dialogOpen.value = true
}

function openEdit(d: DkimDomain) {
  mode.value = 'edit'
  editId.value = d.id
  form.value = {
    domain: d.domain,
    selector: d.selector,
    public_key_fingerprint: d.publicKeyFingerprint,
    // Never display existing key material; leave blank to preserve it.
    private_key_ref: '',
    status: (d.status || 'ready').toLowerCase(),
  }
  dnsRecord.value = null
  dialogOpen.value = true
}

async function generateKey() {
  if (!form.value.domain || !form.value.selector) {
    toast({ title: 'Domain and selector required', description: 'Enter both before generating.', variant: 'destructive' })
    return
  }
  generating.value = true
  try {
    const res = await domainSafetyService.generateDkimKey({
      domain: form.value.domain,
      selector: form.value.selector,
    })
    form.value.private_key_ref = res.privateKeyPem
    form.value.public_key_fingerprint = res.publicKeyFingerprint
    dnsRecord.value = { name: res.recordName, value: res.recordValue }
    toast({ title: 'Key pair generated', description: 'Publish the DNS record, then save.', variant: 'success' })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to generate key.'
    toast({ title: 'Generate failed', description: msg, variant: 'destructive' })
  } finally {
    generating.value = false
  }
}

async function submit() {
  if (!form.value.domain || !form.value.selector) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await domainSafetyService.updateDkimDomain(editId.value, {
        selector: form.value.selector,
        public_key_fingerprint: form.value.public_key_fingerprint,
        private_key_ref: form.value.private_key_ref,
        status: form.value.status,
      })
      toast({ title: 'DKIM domain updated', description: form.value.domain, variant: 'success' })
    } else {
      await domainSafetyService.createDkimDomain({
        domain: form.value.domain,
        selector: form.value.selector,
        public_key_fingerprint: form.value.public_key_fingerprint,
        private_key_ref: form.value.private_key_ref,
      })
      toast({ title: 'DKIM domain added', description: form.value.domain, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save DKIM domain.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="DKIM Domains" description="Signing domains and their selector configuration.">
      <template #actions>
        <Button data-testid="create-dkim-domain" @click="openCreate">Add Domain</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No DKIM domains configured."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Selector</TableHead>
                <TableHead>Public Key Fingerprint</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="d in items" :key="d.id">
                <TableCell class="font-medium">{{ d.domain }}</TableCell>
                <TableCell class="font-mono text-xs">{{ d.selector }}</TableCell>
                <TableCell class="font-mono text-xs text-muted-foreground">{{ d.publicKeyFingerprint }}</TableCell>
                <TableCell><StatusBadge :status="d.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-dkim-domain-${d.id}`"
                    @click="openEdit(d)"
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
        <DialogTitle>{{ isEdit ? 'Edit DKIM Domain' : 'Add DKIM Domain' }}</DialogTitle>
        <DialogDescription>
          Paste a PEM-encoded RSA private key, or generate a new key pair and publish the DNS record.
        </DialogDescription>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="dkim-domain">Domain</Label>
          <Input
            id="dkim-domain"
            v-model="form.domain"
            placeholder="example.com"
            :disabled="isEdit"
          />
          <p v-if="isEdit" class="text-xs text-muted-foreground">The domain is immutable.</p>
        </div>
        <div class="space-y-1.5">
          <Label for="dkim-selector">Selector</Label>
          <Input id="dkim-selector" v-model="form.selector" placeholder="s1" />
        </div>
        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <Label for="dkim-key">Private Key (PEM)</Label>
            <Button
              type="button"
              variant="outline"
              size="sm"
              data-testid="generate-dkim-key"
              :disabled="generating || !form.domain || !form.selector"
              @click="generateKey"
            >
              {{ generating ? 'Generating…' : 'Generate key pair' }}
            </Button>
          </div>
          <textarea
            id="dkim-key"
            v-model="form.private_key_ref"
            rows="5"
            spellcheck="false"
            placeholder="-----BEGIN RSA PRIVATE KEY-----"
            class="flex w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
          />
          <p class="text-xs text-muted-foreground">
            <span v-if="isEdit">Leave blank to keep the existing key. </span>
            The key is stored server-side and never shown again after saving.
          </p>
          <p v-if="form.public_key_fingerprint" class="text-xs text-muted-foreground">
            Fingerprint: <span class="font-mono">{{ form.public_key_fingerprint }}</span>
          </p>
        </div>
        <div
          v-if="dnsRecord"
          class="space-y-1.5 rounded-md border border-amber-300 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950"
        >
          <p class="text-xs font-medium">Publish this DNS TXT record, then save:</p>
          <div class="space-y-1 font-mono text-xs break-all">
            <div><span class="text-muted-foreground">name:</span> {{ dnsRecord.name }}</div>
            <div><span class="text-muted-foreground">type:</span> TXT</div>
            <div><span class="text-muted-foreground">value:</span> {{ dnsRecord.value }}</div>
          </div>
        </div>
        <div v-if="isEdit" class="space-y-1.5">
          <Label for="dkim-status">Status</Label>
          <Select id="dkim-status" v-model="form.status">
            <option v-for="s in DKIM_STATUSES" :key="s" :value="s">{{ s }}</option>
          </Select>
          <p class="text-xs text-muted-foreground">
            Setting status to <span class="font-medium">ready</span> activates DKIM signing for this
            domain in the generated KumoMTA config.
          </p>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.domain || !form.selector">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add Domain' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
