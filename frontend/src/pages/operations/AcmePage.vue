<script setup lang="ts">
import { onMounted, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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
import { useToast } from '@/composables/useToast'
import { acmeService } from '@/services/acme'
import { ApiError } from '@/services/http'
import type { AcmeAccount, AcmeCertificate } from '@/types'

const { toast } = useToast()

const DIRECTORIES = [
  { label: "Let's Encrypt (production)", url: 'https://acme-v02.api.letsencrypt.org/directory' },
  { label: "Let's Encrypt (staging)", url: 'https://acme-staging-v02.api.letsencrypt.org/directory' },
]

const account = ref<AcmeAccount | null>(null)
const certs = ref<AcmeCertificate[]>([])
const form = ref({ email: '', server_url: DIRECTORIES[1].url })
const newDomain = ref('')
const newAltNames = ref('')
const savingAccount = ref(false)
const requesting = ref(false)

function msg(err: unknown) {
  return err instanceof ApiError ? err.message : 'Unexpected error.'
}

async function load() {
  try {
    account.value = await acmeService.getAccount()
    if (account.value.email) form.value.email = account.value.email
    if (account.value.serverUrl) form.value.server_url = account.value.serverUrl
    const list = await acmeService.listCertificates()
    certs.value = list.items ?? []
  } catch (err) {
    toast({ title: 'Failed to load ACME', description: msg(err), variant: 'destructive' })
  }
}

async function saveAccount() {
  savingAccount.value = true
  try {
    account.value = await acmeService.saveAccount({ ...form.value })
    toast({ title: 'ACME account saved', variant: 'success' })
  } catch (err) {
    toast({ title: 'Save failed', description: msg(err), variant: 'destructive' })
  } finally {
    savingAccount.value = false
  }
}

async function requestCert() {
  if (!newDomain.value) return
  requesting.value = true
  try {
    const altNames = newAltNames.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    await acmeService.requestCertificate({ domain: newDomain.value.trim(), alt_names: altNames })
    newDomain.value = ''
    newAltNames.value = ''
    toast({ title: 'Certificate issued', variant: 'success' })
    await load()
  } catch (err) {
    toast({ title: 'Issuance failed', description: msg(err), variant: 'destructive' })
    await load()
  } finally {
    requesting.value = false
  }
}

async function removeCert(c: AcmeCertificate) {
  try {
    await acmeService.deleteCertificate(c.id)
    toast({ title: 'Certificate removed', description: c.domain, variant: 'success' })
    await load()
  } catch (err) {
    toast({ title: 'Delete failed', description: msg(err), variant: 'destructive' })
  }
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader
      title="TLS Certificates (ACME)"
      description="Issue and auto-renew Let's Encrypt certificates for listener TLS via HTTP-01."
    />

    <Card class="mb-4 max-w-2xl">
      <CardHeader>
        <CardTitle>ACME Account</CardTitle>
        <CardDescription>
          The account used to request certificates. HTTP-01 validation requires the challenge
          listener (port 80) to be reachable by the CA — front it with your proxy or set
          <code>IRIS_ACME_HTTP_BIND</code>.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="space-y-1.5">
          <Label for="acme-email">Account email</Label>
          <Input id="acme-email" v-model="form.email" placeholder="ops@example.com" />
        </div>
        <div class="space-y-1.5">
          <Label for="acme-dir">ACME directory</Label>
          <Select id="acme-dir" v-model="form.server_url">
            <option v-for="d in DIRECTORIES" :key="d.url" :value="d.url">{{ d.label }}</option>
          </Select>
          <p class="text-xs text-muted-foreground">
            Use staging to validate the pipeline without burning the production rate limit.
          </p>
        </div>
        <div class="flex items-center gap-3">
          <Button :disabled="savingAccount || !form.email" @click="saveAccount">
            {{ savingAccount ? 'Saving…' : 'Save account' }}
          </Button>
          <span v-if="account?.registered" class="text-xs text-muted-foreground">Registered with the directory</span>
        </div>
      </CardContent>
    </Card>

    <Card class="mb-4 max-w-2xl">
      <CardHeader>
        <CardTitle>Request a certificate</CardTitle>
        <CardDescription>Issues immediately via HTTP-01 and mirrors the PEMs to disk for KumoMTA.</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="space-y-1.5">
          <Label for="acme-domain">Domain</Label>
          <Input id="acme-domain" v-model="newDomain" placeholder="mail.example.com" />
        </div>
        <div class="space-y-1.5">
          <Label for="acme-sans">Additional names (comma-separated)</Label>
          <Input id="acme-sans" v-model="newAltNames" placeholder="smtp.example.com, mx.example.com" />
        </div>
        <Button :disabled="requesting || !newDomain || !account?.configured" @click="requestCert">
          {{ requesting ? 'Requesting…' : 'Request certificate' }}
        </Button>
        <p v-if="!account?.configured" class="text-xs text-muted-foreground">Save the account first.</p>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle>Certificates</CardTitle>
      </CardHeader>
      <CardContent class="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Domain</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead>Cert path</TableHead>
              <TableHead class="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-if="certs.length === 0">
              <TableCell colspan="5" class="text-center text-sm text-muted-foreground">
                No certificates yet.
              </TableCell>
            </TableRow>
            <TableRow v-for="c in certs" :key="c.id">
              <TableCell class="font-medium">
                {{ c.domain }}
                <span v-if="c.altNames?.length" class="text-xs text-muted-foreground">
                  (+{{ c.altNames.length }})
                </span>
              </TableCell>
              <TableCell>
                <StatusBadge :status="c.status" />
                <span v-if="c.lastError" class="block text-xs text-destructive">{{ c.lastError }}</span>
              </TableCell>
              <TableCell class="whitespace-nowrap text-muted-foreground">{{ c.expiresAt || '—' }}</TableCell>
              <TableCell class="font-mono text-xs text-muted-foreground">{{ c.certPath || '—' }}</TableCell>
              <TableCell class="text-right">
                <Button variant="outline" size="sm" @click="removeCert(c)">Remove</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>
</template>
