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
import { acmeService } from '@/services/acme'
import { formatDateTime } from '@/composables/useTimezone'
import { ApiError } from '@/services/http'
import type { AcmeCertificate, GlobalSettings } from '@/types'

const { toast } = useToast()

// Issued certificates available to serve the admin UI over HTTPS.
const issuedCerts = ref<AcmeCertificate[]>([])

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
  fbl_require_verification: false,
  inbound_maildir_base_path: '',
  suppression_ttl: '',
  dmarc_report_email: '',
  admin_http_addr: '',
  admin_tls_enabled: false,
  admin_tls_cert_domain: '',
  acme_renew_interval: '',
  acme_renew_before: '',
  prometheus_url: '',
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
    fbl_require_verification: s.fblRequireVerification ?? false,
    inbound_maildir_base_path: s.inboundMaildirBasePath || '',
    suppression_ttl: s.suppressionTtl || '',
    dmarc_report_email: s.dmarcReportEmail || '',
    admin_http_addr: s.adminHttpAddr || '',
    admin_tls_enabled: s.adminTlsEnabled ?? false,
    admin_tls_cert_domain: s.adminTlsCertDomain || '',
    acme_renew_interval: s.acmeRenewInterval || '',
    acme_renew_before: s.acmeRenewBefore || '',
    prometheus_url: s.prometheusUrl || '',
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
    // Best-effort: the cert dropdown for admin HTTPS.
    try {
      const certs = await acmeService.listCertificates()
      issuedCerts.value = (certs.items ?? []).filter((c) => c.status === 'issued')
    } catch {
      issuedCerts.value = []
    }
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
    apply(
      await settingsService.updateSettings({
        rspamd_mode: form.value.rspamd_mode,
        rspamd_url: form.value.rspamd_url,
        egress_ehlo_domain: form.value.egress_ehlo_domain,
        log_stream_redis_url: form.value.log_stream_redis_url,
        esmtp_listen: form.value.esmtp_listen,
        http_listen: form.value.http_listen,
        egress_retry_interval: form.value.egress_retry_interval,
        egress_max_retry_interval: form.value.egress_max_retry_interval,
        egress_max_age: form.value.egress_max_age,
        bounce_domain: form.value.bounce_domain,
        auto_suppress_hard_bounces: form.value.auto_suppress_hard_bounces,
        soft_bounce_threshold: form.value.soft_bounce_threshold,
        fbl_require_verification: form.value.fbl_require_verification,
        inbound_maildir_base_path: form.value.inbound_maildir_base_path,
        suppression_ttl: form.value.suppression_ttl,
        dmarc_report_email: form.value.dmarc_report_email,
        admin_http_addr: form.value.admin_http_addr,
        admin_tls_enabled: form.value.admin_tls_enabled,
        admin_tls_cert_domain: form.value.admin_tls_cert_domain,
        acme_renew_interval: form.value.acme_renew_interval,
        acme_renew_before: form.value.acme_renew_before,
        prometheus_url: form.value.prometheus_url,
      }),
    )
    toast({
      title: 'Settings saved',
      description:
        'KumoMTA settings apply on the next config apply; admin server / renew changes apply on service restart.',
      variant: 'success',
    })
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
            <div class="flex items-start gap-2">
              <input
                id="fbl-require-verification"
                v-model="form.fbl_require_verification"
                type="checkbox"
                class="mt-1"
                data-testid="fbl-require-verification"
              />
              <Label for="fbl-require-verification" class="font-normal">
                Require FBL verification before auto-suppressing
                <span class="block text-xs text-muted-foreground">
                  Only suppress a complainant when the feedback report is proven to be about mail we
                  sent (X-KumoRef trace, send log, or our DKIM signature). Off = suppress every
                  complaint.
                </span>
              </Label>
            </div>
            <div class="space-y-1.5">
              <Label for="suppression-ttl">Suppression record lifetime</Label>
              <Input
                id="suppression-ttl"
                v-model="form.suppression_ttl"
                placeholder="30d"
                data-testid="suppression-ttl"
              />
              <p class="text-xs text-muted-foreground">
                How long a suppression entry blocks a recipient before it ages out (duration form,
                e.g. <code>720h</code>, <code>30d</code>). Leave blank to keep suppressions
                permanent. Applied as the Redis key TTL on the live suppression list.
              </p>
            </div>
            <div class="space-y-1.5">
              <Label for="dmarc-email">DMARC report address</Label>
              <Input
                id="dmarc-email"
                v-model="form.dmarc_report_email"
                placeholder="dmarc@kmx.example.com"
                data-testid="dmarc-report-email"
              />
              <p class="text-xs text-muted-foreground">
                Address to advertise as <code>rua=</code> in your domains' DMARC DNS records.
                Inbound aggregate reports arriving here are parsed into the DMARC Reports page. One
                address serves all your domains. Leave blank to disable.
              </p>
            </div>
            <div class="space-y-1.5">
              <Label for="maildir-base">Inbound maildir base path</Label>
              <Input
                id="maildir-base"
                v-model="form.inbound_maildir_base_path"
                placeholder="/var/spool/iris/maildirs"
                data-testid="inbound-maildir-base-path"
              />
              <p class="text-xs text-muted-foreground">
                Filesystem root for inbound <strong>maildir</strong> routes that don't set their own
                path. kumod writes one Maildir per recipient under
                <code>&lt;base&gt;/&lt;domain&gt;/&lt;local-part&gt;</code>. Leave blank for the
                default <code>/var/spool/iris/maildirs</code>.
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

        <Card>
          <CardHeader>
            <CardTitle>Observability</CardTitle>
            <CardDescription>Metrics source for the dashboard charts.</CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-1.5">
              <Label for="prometheus-url">Prometheus URL</Label>
              <Input
                id="prometheus-url"
                v-model="form.prometheus_url"
                placeholder="http://localhost:9090"
              />
              <p class="text-xs text-muted-foreground">
                Base URL of the Prometheus that scrapes Iris/KumoMTA. When set, the dashboard
                shows mail-flow charts. Leave blank to disable.
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Iris admin server (this UI)</CardTitle>
            <CardDescription>
              The address Iris serves this console + API on, and optional HTTPS. Changes apply on a
              service restart (the listening socket is bound at startup).
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-1.5">
              <Label for="admin-addr">Admin bind (host:port)</Label>
              <Input id="admin-addr" v-model="form.admin_http_addr" placeholder=":8080" />
              <p class="text-xs text-muted-foreground">
                Overrides the configured HTTP bind. Leave blank to keep the startup config.
              </p>
            </div>
            <div class="flex items-start gap-2">
              <input
                id="admin-tls"
                v-model="form.admin_tls_enabled"
                type="checkbox"
                class="mt-1"
                data-testid="admin-tls-enabled"
              />
              <Label for="admin-tls" class="font-normal">
                Serve HTTPS using an issued certificate
              </Label>
            </div>
            <div v-if="form.admin_tls_enabled" class="space-y-1.5">
              <Label for="admin-cert">Certificate</Label>
              <Select id="admin-cert" v-model="form.admin_tls_cert_domain" data-testid="admin-cert">
                <option value="" disabled>
                  {{ issuedCerts.length ? 'Select a certificate…' : 'No issued certificates' }}
                </option>
                <option v-for="c in issuedCerts" :key="c.id" :value="c.domain">
                  {{ c.domain }}<span v-if="c.expiresAt"> (expires {{ formatDateTime(c.expiresAt) }})</span>
                </option>
              </Select>
              <p class="text-xs text-muted-foreground">
                Issue certificates under TLS Certificates (ACME) first. If the selected cert can't be
                loaded at startup, Iris falls back to plain HTTP (so a bad pick won't lock you out).
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>ACME auto-renew</CardTitle>
            <CardDescription>
              Certificates auto-renew in the background. Tune the schedule here (duration form, e.g.
              <code>12h</code>, <code>30d</code>); applies on a service restart. Blank uses the
              defaults (scan every 12h, renew within 30d of expiry).
            </CardDescription>
          </CardHeader>
          <CardContent class="grid gap-4 sm:grid-cols-2">
            <div class="space-y-1.5">
              <Label for="renew-interval">Scan interval</Label>
              <Input id="renew-interval" v-model="form.acme_renew_interval" placeholder="12h" />
            </div>
            <div class="space-y-1.5">
              <Label for="renew-before">Renew before expiry</Label>
              <Input id="renew-before" v-model="form.acme_renew_before" placeholder="30d" />
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
