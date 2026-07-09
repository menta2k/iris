<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import StatTile from '@/components/dashboard/StatTile.vue'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableEmpty,
} from '@/components/ui/table'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { domainSafetyService, domainCheckService } from '@/services'
import { ApiError } from '@/services/http'
import type { DkimDomain, DomainCheckItem } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<DkimDomain>({
  loader: () => domainSafetyService.listDkimDomains(),
})
const { toast } = useToast()

// ---- Filters (client-side; the list is small and unpaginated) ----

const search = ref('')
const statusFilter = ref('')

const STATUS_FILTER_ITEMS = [
  { title: 'All statuses', value: '' },
  { title: 'Ready', value: 'ready' },
  { title: 'Needs attention', value: 'needs_attention' },
  { title: 'Disabled', value: 'disabled' },
]

const visible = computed(() => {
  const term = (search.value ?? '').trim().toLowerCase()
  const st = statusFilter.value
  return items.value.filter(
    (d) =>
      (!term || d.domain.toLowerCase().includes(term) || d.selector.toLowerCase().includes(term)) &&
      (!st || (d.status || '').toLowerCase() === st),
  )
})

const hasActiveFilters = computed(() => (search.value ?? '') !== '' || statusFilter.value !== '')

function resetFilters() {
  search.value = ''
  statusFilter.value = ''
}

// ---- KPI tiles ----

const countBy = (s: string) => items.value.filter((d) => (d.status || '').toLowerCase() === s).length
const readyCount = computed(() => countBy('ready'))
const attentionCount = computed(() => countBy('needs_attention'))
const disabledCount = computed(() => countBy('disabled'))

// ---- Live DNS verification (expandable row, per domain) ----

type CheckState = { loading: boolean; error: string | null; items: DomainCheckItem[] }
const expandedId = ref<string | null>(null)
const checkByDomain = ref<Record<string, CheckState>>({})

async function verifyDns(d: DkimDomain, force = false) {
  if (expandedId.value === d.id && !force) {
    expandedId.value = null
    return
  }
  expandedId.value = d.id
  if (checkByDomain.value[d.id] && !force) return // cached from a prior run
  checkByDomain.value = { ...checkByDomain.value, [d.id]: { loading: true, error: null, items: [] } }
  try {
    const res = await domainCheckService.check(d.domain)
    checkByDomain.value = {
      ...checkByDomain.value,
      [d.id]: { loading: false, error: null, items: res.items ?? [] },
    }
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'DNS check failed.'
    checkByDomain.value = { ...checkByDomain.value, [d.id]: { loading: false, error: msg, items: [] } }
  }
}

function checkVariant(status: string) {
  switch ((status || '').toLowerCase()) {
    case 'pass':
      return 'success' as const
    case 'warn':
      return 'warning' as const
    default:
      return 'destructive' as const
  }
}

// Summary chip after a check ran: worst status across the items.
function checkSummary(d: DkimDomain): { label: string; variant: 'success' | 'warning' | 'destructive' } | null {
  const c = checkByDomain.value[d.id]
  if (!c || c.loading || c.error || c.items.length === 0) return null
  const statuses = c.items.map((i) => (i.status || '').toLowerCase())
  if (statuses.includes('fail')) return { label: 'DNS issues', variant: 'destructive' }
  if (statuses.includes('warn')) return { label: 'DNS warnings', variant: 'warning' }
  return { label: 'DNS ok', variant: 'success' }
}

// ---- Clipboard ----

async function copy(text: string, what: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast({ title: `${what} copied`, variant: 'success', duration: 2000 })
  } catch {
    toast({ title: 'Could not copy to clipboard', variant: 'destructive' })
  }
}

// ---- Create/edit dialog ----

const DKIM_STATUSES = ['ready', 'disabled', 'needs_attention']
const DKIM_STATUS_ITEMS = DKIM_STATUSES.map((s) => ({ title: s, value: s }))

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

    <v-row dense class="mb-2">
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Signing Domains"
          :value="items.length.toLocaleString()"
          caption="Configured in Iris"
          icon="mdi-web"
          color="primary"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Ready"
          :value="readyCount.toLocaleString()"
          caption="Signing outbound mail"
          icon="mdi-shield-check-outline"
          color="success"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Needs Attention"
          :value="attentionCount.toLocaleString()"
          caption="Not signing until resolved"
          icon="mdi-shield-alert-outline"
          :color="attentionCount > 0 ? 'warning' : 'secondary'"
          :value-class="attentionCount > 0 ? 'text-warning' : ''"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Disabled"
          :value="disabledCount.toLocaleString()"
          caption="Signing switched off"
          icon="mdi-shield-off-outline"
          color="secondary"
        />
      </v-col>
    </v-row>

    <Card>
      <div class="d-flex flex-wrap align-center ga-2 px-4 py-2">
        <div class="mr-1">
          <span class="text-subtitle-1 font-weight-bold">Domains</span>
          <p class="text-caption text-medium-emphasis mb-0">Verify DNS runs a live MX/SPF/DKIM check</p>
        </div>
        <v-spacer />
        <v-text-field
          v-model="search"
          placeholder="Search domain or selector"
          prepend-inner-icon="mdi-magnify"
          variant="outlined"
          density="compact"
          hide-details
          clearable
          class="flex-grow-0"
          style="width: 240px"
        />
        <v-select
          v-model="statusFilter"
          :items="STATUS_FILTER_ITEMS"
          variant="outlined"
          density="compact"
          hide-details
          class="flex-grow-0"
          style="width: 180px"
        />
        <v-btn
          v-if="hasActiveFilters"
          variant="text"
          size="small"
          @click="resetFilters"
        >
          Reset
        </v-btn>
        <v-btn
          icon="mdi-refresh"
          variant="text"
          size="small"
          :loading="loading"
          aria-label="Refresh"
          title="Refresh"
          @click="load"
        />
      </div>
      <v-divider />
      <v-progress-linear :active="loading" indeterminate color="primary" height="2" />
      <CardContent class="pa-0">
        <DataState
          :loading="loading && items.length === 0"
          :error="error"
          :not-implemented="notImplemented"
          :empty="items.length === 0"
          empty-message="No DKIM domains configured."
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Selector</TableHead>
                <TableHead>Public Key Fingerprint</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>DNS</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableEmpty
                v-if="visible.length === 0"
                :colspan="6"
                message="No domains match the selected filters."
              />
              <template v-for="d in visible" :key="d.id">
                <TableRow class="row-clickable" @click="verifyDns(d)">
                  <TableCell class="font-weight-medium">{{ d.domain }}</TableCell>
                  <TableCell class="font-mono text-caption">{{ d.selector }}</TableCell>
                  <TableCell style="max-width: 260px">
                    <span
                      class="d-block text-truncate font-mono text-caption text-medium-emphasis"
                      :title="d.publicKeyFingerprint"
                    >
                      {{ d.publicKeyFingerprint || '—' }}
                    </span>
                  </TableCell>
                  <TableCell><StatusBadge :status="d.status" /></TableCell>
                  <TableCell>
                    <Badge v-if="checkSummary(d)" :variant="checkSummary(d)!.variant">
                      {{ checkSummary(d)!.label }}
                    </Badge>
                    <span v-else-if="checkByDomain[d.id]?.loading" class="text-caption text-medium-emphasis">
                      Checking…
                    </span>
                    <span v-else class="text-caption text-medium-emphasis">—</span>
                  </TableCell>
                  <TableCell class="text-right">
                    <div class="d-flex justify-end ga-1">
                      <v-btn
                        icon="mdi-dns-outline"
                        variant="text"
                        size="small"
                        :loading="checkByDomain[d.id]?.loading"
                        aria-label="Verify DNS"
                        title="Verify DNS (live MX/SPF/DKIM lookup)"
                        :data-testid="`verify-dkim-domain-${d.id}`"
                        @click.stop="verifyDns(d, expandedId === d.id)"
                      />
                      <v-btn
                        icon="mdi-pencil-outline"
                        variant="text"
                        size="small"
                        aria-label="Edit domain"
                        title="Edit domain"
                        :data-testid="`edit-dkim-domain-${d.id}`"
                        @click.stop="openEdit(d)"
                      />
                    </div>
                  </TableCell>
                </TableRow>
                <tr v-if="expandedId === d.id">
                  <td :colspan="6" class="px-4 py-3">
                    <p class="mb-2 text-caption text-uppercase text-medium-emphasis">
                      Live DNS check — {{ d.domain }}
                    </p>
                    <div v-if="checkByDomain[d.id]?.loading" class="text-caption text-medium-emphasis">
                      Looking up DNS records…
                    </div>
                    <div v-else-if="checkByDomain[d.id]?.error" class="text-caption text-error">
                      {{ checkByDomain[d.id]?.error }}
                    </div>
                    <div v-else class="d-flex flex-column ga-2">
                      <div
                        v-for="item in checkByDomain[d.id]?.items ?? []"
                        :key="item.name"
                        class="d-flex flex-wrap align-center ga-2"
                      >
                        <Badge :variant="checkVariant(item.status)" style="min-width: 64px; justify-content: center">
                          {{ item.status }}
                        </Badge>
                        <span class="text-body-2 font-weight-medium" style="min-width: 140px">{{ item.name }}</span>
                        <span class="text-body-2 text-medium-emphasis">{{ item.detail }}</span>
                        <span
                          v-for="r in item.records ?? []"
                          :key="r"
                          class="font-mono text-caption text-medium-emphasis text-break"
                        >
                          {{ r }}
                        </span>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </TableBody>
          </Table>
        </DataState>
      </CardContent>
    </Card>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit DKIM Domain' : 'Add DKIM Domain' }}</DialogTitle>
        <DialogDescription>
          Paste a PEM-encoded RSA private key, or generate a new key pair and publish the DNS record.
        </DialogDescription>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="dkim-domain">Domain</Label>
          <Input
            id="dkim-domain"
            v-model="form.domain"
            placeholder="example.com"
            :disabled="isEdit"
          />
          <p v-if="isEdit" class="text-caption text-medium-emphasis">The domain is immutable.</p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="dkim-selector">Selector</Label>
          <Input id="dkim-selector" v-model="form.selector" placeholder="s1" />
        </div>
        <div class="d-flex flex-column ga-1">
          <div class="d-flex align-center justify-space-between">
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
          <v-textarea
            id="dkim-key"
            v-model="form.private_key_ref"
            rows="5"
            spellcheck="false"
            placeholder="-----BEGIN RSA PRIVATE KEY-----"
            variant="outlined"
            density="compact"
            hide-details
            auto-grow
            class="font-mono text-caption"
          />
          <p class="text-caption text-medium-emphasis">
            <span v-if="isEdit">Leave blank to keep the existing key. </span>
            The key is stored server-side and never shown again after saving.
          </p>
          <p v-if="form.public_key_fingerprint" class="text-caption text-medium-emphasis">
            Fingerprint: <span class="font-mono">{{ form.public_key_fingerprint }}</span>
          </p>
        </div>
        <v-alert v-if="dnsRecord" type="warning" variant="tonal" density="comfortable">
          <div class="d-flex align-center justify-space-between ga-2">
            <p class="text-caption font-weight-medium mb-0">Publish this DNS TXT record, then save:</p>
            <v-btn
              variant="text"
              size="x-small"
              prepend-icon="mdi-content-copy"
              @click="copy(`${dnsRecord!.name} TXT ${dnsRecord!.value}`, 'DNS record')"
            >
              Copy record
            </v-btn>
          </div>
          <div class="d-flex flex-column ga-1 font-mono text-caption text-break">
            <div class="d-flex align-center ga-1">
              <span class="text-medium-emphasis">name:</span> {{ dnsRecord.name }}
              <v-btn
                icon="mdi-content-copy"
                variant="text"
                size="x-small"
                aria-label="Copy record name"
                title="Copy record name"
                @click="copy(dnsRecord!.name, 'Record name')"
              />
            </div>
            <div><span class="text-medium-emphasis">type:</span> TXT</div>
            <div class="d-flex align-center ga-1">
              <span class="text-medium-emphasis">value:</span>
              <span class="text-break flex-grow-1">{{ dnsRecord.value }}</span>
              <v-btn
                icon="mdi-content-copy"
                variant="text"
                size="x-small"
                aria-label="Copy record value"
                title="Copy record value"
                @click="copy(dnsRecord!.value, 'Record value')"
              />
            </div>
          </div>
        </v-alert>
        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="dkim-status">Status</Label>
          <v-select
            id="dkim-status"
            v-model="form.status"
            :items="DKIM_STATUS_ITEMS"
            variant="outlined"
            density="compact"
            hide-details
          />
          <p class="text-caption text-medium-emphasis">
            Setting status to <span class="font-weight-medium">ready</span> activates DKIM signing for this
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

<style scoped>
.row-clickable {
  cursor: pointer;
}
</style>
