<script setup lang="ts">
import { onMounted, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { useToast } from '@/composables/useToast'
import { settingsService } from '@/services'
import { ApiError } from '@/services/http'
import type { GlobalSettings } from '@/types'

const { toast } = useToast()

const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const saving = ref(false)
const updatedBy = ref('')
const updatedAt = ref('')

const form = ref({
  rspamd_mode: '',
  rspamd_url: '',
  egress_ehlo_domain: '',
  log_stream_redis_url: '',
  esmtp_listen: '',
  http_listen: '',
  egress_retry_interval: '',
  egress_max_retry_interval: '',
  egress_max_age: '',
  bounce_domain: '',
  auto_suppress_hard_bounces: true,
  soft_bounce_threshold: 0,
  fbl_domain: '',
})

function apply(s: GlobalSettings) {
  form.value = {
    rspamd_mode: s.rspamdMode || '',
    rspamd_url: s.rspamdUrl || '',
    egress_ehlo_domain: s.egressEhloDomain || '',
    log_stream_redis_url: s.logStreamRedisUrl || '',
    esmtp_listen: s.esmtpListen || '',
    http_listen: s.httpListen || '',
    egress_retry_interval: s.egressRetryInterval || '',
    egress_max_retry_interval: s.egressMaxRetryInterval || '',
    egress_max_age: s.egressMaxAge || '',
    bounce_domain: s.bounceDomain || '',
    auto_suppress_hard_bounces: s.autoSuppressHardBounces ?? true,
    soft_bounce_threshold: s.softBounceThreshold ?? 0,
    fbl_domain: s.fblDomain || '',
  }
  updatedBy.value = s.updatedBy || ''
  updatedAt.value = s.updatedAt || ''
}

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    apply(await settingsService.getSettings())
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else if (err instanceof ApiError && err.status === 0)
      error.value = 'Cannot reach the backend. Is the API server running?'
    else error.value = err instanceof Error ? err.message : 'Failed to load settings.'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    apply(await settingsService.updateSettings({ ...form.value }))
    toast({ title: 'Settings saved', description: 'Applies on the next KumoMTA config apply.', variant: 'success' })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save settings.'
    toast({ title: 'Save failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader
      title="Global Settings"
      description="Deployment-level KumoMTA policy knobs. Changes apply when you next generate/apply the KumoMTA config."
    />

    <DataState :loading="loading" :error="error" :not-implemented="notImplemented">
      <form class="max-w-2xl space-y-6" @submit.prevent="save">
        <Card>
          <CardHeader>
            <CardTitle>Inbound Spam Filtering (rspamd)</CardTitle>
            <CardDescription>
              Scan inbound mail for hosted domains through rspamd. Scoped to hosted domains and
              fail-open if rspamd is unreachable.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-1.5">
              <Label for="rspamd-mode">Mode</Label>
              <Select id="rspamd-mode" v-model="form.rspamd_mode" data-testid="rspamd-mode">
                <option value="">Off (disabled)</option>
                <option value="tag">Tag — scan &amp; add X-Spam headers, never reject</option>
                <option value="enforce">Enforce — honor reject (550) / greylist (451)</option>
              </Select>
            </div>
            <div class="space-y-1.5">
              <Label for="rspamd-url">rspamd URL</Label>
              <Input id="rspamd-url" v-model="form.rspamd_url" placeholder="http://rspamd:11334" />
              <p class="text-xs text-muted-foreground">Required when mode is Tag or Enforce.</p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Outbound &amp; Logging</CardTitle>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-1.5">
              <Label for="ehlo">Default egress EHLO domain</Label>
              <Input id="ehlo" v-model="form.egress_ehlo_domain" placeholder="mail.example.com" />
            </div>
            <div class="space-y-1.5">
              <Label for="redis">Log-stream Redis URL</Label>
              <Input id="redis" v-model="form.log_stream_redis_url" placeholder="redis://redis:6379" />
              <p class="text-xs text-muted-foreground">
                Enables the KumoMTA log_hook that feeds the Mail Logs. The address KumoMTA reaches
                Redis at.
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Delivery Rates &amp; Retry</CardTitle>
            <CardDescription>
              Outbound retry schedule applied to the default egress queue. Durations use KumoMTA
              syntax (e.g. <code>20m</code>, <code>2h</code>, <code>1d</code>). Leave blank for
              KumoMTA defaults.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-1.5">
              <Label for="retry">Retry interval</Label>
              <Input id="retry" v-model="form.egress_retry_interval" placeholder="20m" />
              <p class="text-xs text-muted-foreground">
                Base interval before the first retry; backs off exponentially.
              </p>
            </div>
            <div class="space-y-1.5">
              <Label for="max-retry">Max retry interval</Label>
              <Input id="max-retry" v-model="form.egress_max_retry_interval" placeholder="2h" />
              <p class="text-xs text-muted-foreground">Caps the exponential backoff.</p>
            </div>
            <div class="space-y-1.5">
              <Label for="max-age">Max message age</Label>
              <Input id="max-age" v-model="form.egress_max_age" placeholder="1d" />
              <p class="text-xs text-muted-foreground">
                A message bounces if still undeliverable after this age.
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Bounce / DSN Pipeline</CardTitle>
            <CardDescription>
              Capture asynchronous bounces (DSNs) at a dedicated domain and automatically suppress
              repeatedly-failing recipients. Requires the log-stream Redis URL above.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-1.5">
              <Label for="bounce-domain">Bounce domain</Label>
              <Input id="bounce-domain" v-model="form.bounce_domain" placeholder="bounce.example.com" />
              <p class="text-xs text-muted-foreground">
                Mail to this domain is routed to the DSN catcher instead of being relayed. Leave
                blank to disable the bounce pipeline.
              </p>
            </div>
            <div class="flex items-start gap-2">
              <input
                id="auto-suppress"
                v-model="form.auto_suppress_hard_bounces"
                type="checkbox"
                class="mt-1"
                data-testid="auto-suppress"
              />
              <Label for="auto-suppress" class="font-normal">
                Auto-suppress recipients on a hard (5xx) bounce
              </Label>
            </div>
            <div class="space-y-1.5">
              <Label for="soft-threshold">Soft-bounce suppression threshold</Label>
              <Input
                id="soft-threshold"
                v-model.number="form.soft_bounce_threshold"
                type="number"
                min="0"
                max="1000"
                placeholder="0"
              />
              <p class="text-xs text-muted-foreground">
                Suppress a recipient after this many soft (4xx) bounces. 0 disables soft-bounce
                suppression.
              </p>
            </div>
            <div class="space-y-1.5">
              <Label for="fbl-domain">Feedback (FBL/ARF) domain</Label>
              <Input id="fbl-domain" v-model="form.fbl_domain" placeholder="fbl.example.com" />
              <p class="text-xs text-muted-foreground">
                KumoMTA parses RFC 5965 ARF feedback reports sent to this domain and emits Feedback
                records, which auto-suppress the complainant. Requires the log-stream Redis URL.
                Leave blank to disable.
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Listeners</CardTitle>
            <CardDescription>Default binds emitted in the generated policy.</CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-1.5">
              <Label for="esmtp">ESMTP listen (host:port)</Label>
              <Input id="esmtp" v-model="form.esmtp_listen" placeholder="0.0.0.0:2525" />
            </div>
            <div class="space-y-1.5">
              <Label for="http">HTTP listen (host:port)</Label>
              <Input id="http" v-model="form.http_listen" placeholder="0.0.0.0:8000" />
            </div>
          </CardContent>
        </Card>

        <div class="flex items-center justify-between">
          <p v-if="updatedBy" class="text-xs text-muted-foreground">
            Last updated by {{ updatedBy }}<span v-if="updatedAt"> at {{ updatedAt }}</span>
          </p>
          <Button type="submit" data-testid="save-settings" :disabled="saving">
            {{ saving ? 'Saving…' : 'Save settings' }}
          </Button>
        </div>
      </form>
    </DataState>
  </div>
</template>
