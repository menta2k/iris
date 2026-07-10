<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { useToast } from '@/composables/useToast'
import { settingsService } from '@/services'
import { acmeService } from '@/services/acme'
import { formatDateTime } from '@/composables/useTimezone'
import { ApiError } from '@/services/http'
import type { AcmeCertificate, GlobalSettings } from '@/types'

const { toast } = useToast()

// Issued certificates available to serve the admin UI over HTTPS.
const issuedCerts = ref<AcmeCertificate[]>([])

const RSPAMD_MODE_ITEMS = [
  { title: 'Off (disabled)', value: '' },
  { title: 'Tag — scan & add X-Spam headers, never reject', value: 'tag' },
  { title: 'Enforce — honor reject (550) / greylist (451)', value: 'enforce' },
]

const certItems = computed(() =>
  issuedCerts.value.map((c) => ({
    title: c.expiresAt ? `${c.domain} (expires ${formatDateTime(c.expiresAt)})` : c.domain,
    value: c.domain,
  })),
)

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
  pin_egress_per_message: false,
  bounce_domain: '',
  bounce_domain_template: '',
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
  classify_subjects: false,
  classify_model: '',
  classify_threshold: 0.45,
  classify_api_base: '',
  injection_enabled: false,
  injection_listen_addr: '',
  injection_path: '',
  injection_tls_enabled: false,
  injection_tls_cert_domain: '',
  monitoring_from: '',
  monitoring_reconcile_lookback: '',
  monitoring_fetch_timeout: '',
  monitoring_fetch_giveup: '',
})

// Snapshot of the last saved/loaded form, for the unsaved-changes indicator.
const savedForm = ref('')
const dirty = computed(() => savedForm.value !== '' && JSON.stringify(form.value) !== savedForm.value)

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
    pin_egress_per_message: s.pinEgressPerMessage ?? false,
    bounce_domain: s.bounceDomain || '',
    bounce_domain_template: s.bounceDomainTemplate || '',
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
    classify_subjects: s.classifySubjects ?? false,
    classify_model: s.classifyModel || '',
    classify_threshold: s.classifyThreshold ?? 0.45,
    classify_api_base: s.classifyApiBase || '',
    injection_enabled: s.injectionEnabled ?? false,
    injection_listen_addr: s.injectionListenAddr || '',
    injection_path: s.injectionPath || '',
    injection_tls_enabled: s.injectionTlsEnabled ?? false,
    injection_tls_cert_domain: s.injectionTlsCertDomain || '',
    monitoring_from: s.monitoringFrom || '',
    monitoring_reconcile_lookback: s.monitoringReconcileLookback || '',
    monitoring_fetch_timeout: s.monitoringFetchTimeout || '',
    monitoring_fetch_giveup: s.monitoringFetchGiveup || '',
  }
  updatedBy.value = s.updatedBy || ''
  updatedAt.value = s.updatedAt || ''
  savedForm.value = JSON.stringify(form.value)
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
  // The section cards only mount once the spinner clears — sync the
  // scrollspy after that.
  await nextTick()
  observeSections()
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
        pin_egress_per_message: form.value.pin_egress_per_message,
        bounce_domain: form.value.bounce_domain,
        bounce_domain_template: form.value.bounce_domain_template,
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
        classify_subjects: form.value.classify_subjects,
        classify_model: form.value.classify_model,
        classify_threshold: Number(form.value.classify_threshold),
        classify_api_base: form.value.classify_api_base,
        injection_enabled: form.value.injection_enabled,
        injection_listen_addr: form.value.injection_listen_addr,
        injection_path: form.value.injection_path,
        injection_tls_enabled: form.value.injection_tls_enabled,
        injection_tls_cert_domain: form.value.injection_tls_cert_domain,
        monitoring_from: form.value.monitoring_from,
        monitoring_reconcile_lookback: form.value.monitoring_reconcile_lookback,
        monitoring_fetch_timeout: form.value.monitoring_fetch_timeout,
        monitoring_fetch_giveup: form.value.monitoring_fetch_giveup,
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

// ---- Section navigation: filter, jump links, scrollspy ----

const SECTIONS = [
  { id: 'sec-rspamd', title: 'Inbound Spam Filtering', icon: 'mdi-shield-bug-outline', keywords: 'rspamd spam scan tag enforce inbound' },
  { id: 'sec-outbound', title: 'Outbound & Logging', icon: 'mdi-email-arrow-right-outline', keywords: 'ehlo egress redis log stream mail logs' },
  { id: 'sec-retry', title: 'Delivery Retry', icon: 'mdi-timer-refresh-outline', keywords: 'retry interval backoff max age pin egress ip' },
  { id: 'sec-bounce', title: 'Bounce / DSN Pipeline', icon: 'mdi-email-remove-outline', keywords: 'bounce domain dsn suppress soft hard fbl suppression ttl dmarc report maildir' },
  { id: 'sec-listeners', title: 'Listeners', icon: 'mdi-lan-connect', keywords: 'esmtp http listen bind port' },
  { id: 'sec-observability', title: 'Observability', icon: 'mdi-chart-line', keywords: 'prometheus metrics dashboard charts' },
  { id: 'sec-classify', title: 'Subject Classification', icon: 'mdi-label-outline', keywords: 'classify subject openai model llm threshold' },
  { id: 'sec-admin', title: 'Admin Server', icon: 'mdi-monitor-lock', keywords: 'admin ui https tls certificate bind' },
  { id: 'sec-injection', title: 'Injection API', icon: 'mdi-email-fast-outline', keywords: 'injection greenarrow inject listener port https tls api credentials mail' },
  { id: 'sec-monitoring', title: 'Inbox Monitoring', icon: 'mdi-email-search-outline', keywords: 'inbox monitoring probe seed mailbox placement spam from sender fetch timeout giveup reconcile lookback' },
  { id: 'sec-acme', title: 'ACME Auto-Renew', icon: 'mdi-certificate-outline', keywords: 'acme renew certificate expiry scan' },
] as const

type SectionId = (typeof SECTIONS)[number]['id']

const sectionSearch = ref('')
const activeSection = ref<SectionId>('sec-rspamd')

const visibleSections = computed(() => {
  const term = (sectionSearch.value ?? '').trim().toLowerCase()
  if (!term) return SECTIONS
  return SECTIONS.filter(
    (s) => s.title.toLowerCase().includes(term) || s.keywords.includes(term),
  )
})

const sectionShown = computed(() => new Set(visibleSections.value.map((s) => s.id)))

// While a click-initiated smooth scroll is running, the clicked section wins —
// otherwise the spy would land on whatever passes the line mid-animation (and
// bottom sections can never reach it once the page hits max scroll).
let spyLockUntil = 0

function scrollTo(id: SectionId) {
  activeSection.value = id
  spyLockUntil = Date.now() + 1200
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// Scrollspy: the current section is the last one whose top has passed the
// line just under the app bar (where scroll-margin lands anchored sections).
function updateActiveSection() {
  if (Date.now() < spyLockUntil) return
  const shown = visibleSections.value
  if (shown.length === 0) return
  // At max scroll the bottom sections can never cross the top line — treat
  // the last one as current.
  const doc = document.documentElement
  if (window.innerHeight + window.scrollY >= doc.scrollHeight - 24) {
    activeSection.value = shown[shown.length - 1].id
    return
  }
  let current: SectionId = shown[0].id
  for (const s of shown) {
    const el = document.getElementById(s.id)
    if (el && el.getBoundingClientRect().top <= 120) current = s.id
  }
  activeSection.value = current
}

function observeSections() {
  updateActiveSection()
}

onMounted(() => {
  window.addEventListener('scroll', updateActiveSection, { passive: true })
  load()
})
onBeforeUnmount(() => window.removeEventListener('scroll', updateActiveSection))
</script>

<template>
  <div>
    <PageHeader
      title="Global Settings"
      description="Deployment-level KumoMTA policy knobs. Changes apply when you next generate/apply the KumoMTA config."
    />

    <DataState :loading="loading" :error="error" :not-implemented="notImplemented">
      <v-row dense>
        <!-- Sticky section nav + save -->
        <v-col cols="12" md="3" class="d-none d-md-block">
          <div class="settings-nav">
            <Card>
              <CardContent class="pa-3 d-flex flex-column ga-1">
                <v-text-field
                  v-model="sectionSearch"
                  placeholder="Find a setting"
                  prepend-inner-icon="mdi-magnify"
                  variant="outlined"
                  density="compact"
                  hide-details
                  clearable
                  class="mb-2"
                />
                <v-btn
                  v-for="s in visibleSections"
                  :key="s.id"
                  variant="text"
                  size="small"
                  class="justify-start text-none"
                  :color="activeSection === s.id ? 'primary' : undefined"
                  :prepend-icon="s.icon"
                  @click="scrollTo(s.id)"
                >
                  {{ s.title }}
                </v-btn>
                <p v-if="visibleSections.length === 0" class="text-caption text-medium-emphasis px-2 mb-0">
                  No sections match.
                </p>
                <v-divider class="my-2" />
                <v-chip v-if="dirty" size="small" color="warning" variant="tonal" class="mb-2 align-self-start">
                  Unsaved changes
                </v-chip>
                <Button data-testid="save-settings" :disabled="saving" @click="save">
                  {{ saving ? 'Saving…' : 'Save settings' }}
                </Button>
                <p v-if="updatedBy" class="text-caption text-medium-emphasis mt-2 mb-0">
                  Last updated by {{ updatedBy }}<span v-if="updatedAt"> at {{ updatedAt }}</span>
                </p>
              </CardContent>
            </Card>
          </div>
        </v-col>

        <v-col cols="12" md="9">
          <form class="d-flex flex-column ga-4" style="max-width: 860px" @submit.prevent="save">
            <Card v-show="sectionShown.has('sec-rspamd')" id="sec-rspamd" class="scroll-target">
              <CardHeader>
                <CardTitle>Inbound Spam Filtering (rspamd)</CardTitle>
                <CardDescription>
                  Scan inbound mail for hosted domains through rspamd. Scoped to hosted domains and
                  fail-open if rspamd is unreachable.
                </CardDescription>
              </CardHeader>
              <CardContent class="d-flex flex-column ga-4">
                <v-select
                  v-model="form.rspamd_mode"
                  :items="RSPAMD_MODE_ITEMS"
                  label="Mode"
                  variant="outlined"
                  density="compact"
                  hide-details
                  data-testid="rspamd-mode"
                />
                <div>
                  <v-text-field
                    v-model="form.rspamd_url"
                    label="rspamd URL"
                    placeholder="http://rspamd:11334"
                    variant="outlined"
                    density="compact"
                    hide-details
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">Required when mode is Tag or Enforce.</p>
                </div>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-outbound')" id="sec-outbound" class="scroll-target">
              <CardHeader>
                <CardTitle>Outbound &amp; Logging</CardTitle>
              </CardHeader>
              <CardContent class="d-flex flex-column ga-4">
                <v-text-field
                  v-model="form.egress_ehlo_domain"
                  label="Default egress EHLO domain"
                  placeholder="mail.example.com"
                  variant="outlined"
                  density="compact"
                  hide-details
                />
                <div>
                  <v-text-field
                    v-model="form.log_stream_redis_url"
                    label="Log-stream Redis URL"
                    placeholder="redis://redis:6379"
                    variant="outlined"
                    density="compact"
                    hide-details
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Enables the KumoMTA log_hook that feeds the Mail Logs. The address KumoMTA reaches
                    Redis at.
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-retry')" id="sec-retry" class="scroll-target">
              <CardHeader>
                <CardTitle>Delivery Rates &amp; Retry</CardTitle>
                <CardDescription>
                  Outbound retry schedule applied to the default egress queue. Durations use KumoMTA
                  syntax (e.g. <code>20m</code>, <code>2h</code>, <code>1d</code>). Leave blank for
                  KumoMTA defaults.
                </CardDescription>
              </CardHeader>
              <CardContent class="d-flex flex-column ga-4">
                <v-row dense>
                  <v-col cols="12" sm="4">
                    <v-text-field
                      v-model="form.egress_retry_interval"
                      label="Retry interval"
                      placeholder="20m"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                    <p class="mt-1 text-caption text-medium-emphasis mb-0">Base interval; backs off exponentially.</p>
                  </v-col>
                  <v-col cols="12" sm="4">
                    <v-text-field
                      v-model="form.egress_max_retry_interval"
                      label="Max retry interval"
                      placeholder="2h"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                    <p class="mt-1 text-caption text-medium-emphasis mb-0">Caps the exponential backoff.</p>
                  </v-col>
                  <v-col cols="12" sm="4">
                    <v-text-field
                      v-model="form.egress_max_age"
                      label="Max message age"
                      placeholder="1d"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                    <p class="mt-1 text-caption text-medium-emphasis mb-0">Bounces if still undeliverable after this.</p>
                  </v-col>
                </v-row>
                <div>
                  <v-switch
                    v-model="form.pin_egress_per_message"
                    color="primary"
                    density="compact"
                    hide-details
                    label="Pin egress IP per message across retries"
                    data-testid="pin-egress"
                  />
                  <p class="text-caption text-medium-emphasis mb-0">
                    Keep each message on a single sending IP for its whole lifecycle (the source is
                    chosen deterministically by a hash of the message id, weighted by the pool). Off =
                    KumoMTA's default weighted round-robin, which may retry the same message from
                    different IPs. Only affects multi-IP pools.
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-bounce')" id="sec-bounce" class="scroll-target">
              <CardHeader>
                <CardTitle>Bounce / DSN Pipeline</CardTitle>
                <CardDescription>
                  Capture asynchronous bounces (DSNs) at a dedicated domain and automatically suppress
                  repeatedly-failing recipients. Requires the log-stream Redis URL above.
                </CardDescription>
              </CardHeader>
              <CardContent class="d-flex flex-column ga-4">
                <div>
                  <v-text-field
                    v-model="form.bounce_domain"
                    label="Bounce domain"
                    placeholder="bounce.example.com"
                    variant="outlined"
                    density="compact"
                    hide-details
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Mail to this domain is routed to the DSN catcher instead of being relayed. Leave
                    blank to disable the bounce pipeline.
                  </p>
                </div>
                <div>
                  <v-text-field
                    v-model="form.bounce_domain_template"
                    label="Per-domain bounce template"
                    placeholder="bounce.kumo.{domain}"
                    variant="outlined"
                    density="compact"
                    hide-details
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Optional. When set, the return-path is derived per sending (DKIM) domain by
                    substituting <code>{domain}</code> — e.g. mail from <code>@example.com</code> uses
                    <code>bounce.kumo.example.com</code>, aligning SPF with the From-domain so it backs up
                    DKIM for DMARC. Each derived domain needs its own MX (to the bounce listener) and SPF
                    records. Leave blank to use the single bounce domain above for all mail.
                  </p>
                </div>
                <v-switch
                  v-model="form.auto_suppress_hard_bounces"
                  color="primary"
                  density="compact"
                  hide-details
                  label="Auto-suppress recipients on a hard (5xx) bounce"
                  data-testid="auto-suppress"
                />
                <div>
                  <v-text-field
                    v-model.number="form.soft_bounce_threshold"
                    label="Soft-bounce suppression threshold"
                    type="number"
                    min="0"
                    max="1000"
                    placeholder="0"
                    variant="outlined"
                    density="compact"
                    hide-details
                    style="max-width: 280px"
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Suppress a recipient after this many soft (4xx) bounces. 0 disables soft-bounce
                    suppression.
                  </p>
                </div>
                <div>
                  <v-switch
                    v-model="form.fbl_require_verification"
                    color="primary"
                    density="compact"
                    hide-details
                    label="Require FBL verification before auto-suppressing"
                    data-testid="fbl-require-verification"
                  />
                  <p class="text-caption text-medium-emphasis mb-0">
                    Only suppress a complainant when the feedback report is proven to be about mail we
                    sent (X-KumoRef trace, send log, or our DKIM signature). Off = suppress every
                    complaint.
                  </p>
                </div>
                <div>
                  <v-text-field
                    v-model="form.suppression_ttl"
                    label="Suppression record lifetime"
                    placeholder="30d"
                    variant="outlined"
                    density="compact"
                    hide-details
                    style="max-width: 280px"
                    data-testid="suppression-ttl"
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    How long a suppression entry blocks a recipient before it ages out (duration form,
                    e.g. <code>720h</code>, <code>30d</code>). Leave blank to keep suppressions
                    permanent. Applied as the Redis key TTL on the live suppression list.
                  </p>
                </div>
                <div>
                  <v-text-field
                    v-model="form.dmarc_report_email"
                    label="DMARC report address"
                    placeholder="dmarc@kmx.example.com"
                    variant="outlined"
                    density="compact"
                    hide-details
                    data-testid="dmarc-report-email"
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Address to advertise as <code>rua=</code> in your domains' DMARC DNS records.
                    Inbound aggregate reports arriving here are parsed into the DMARC Reports page. One
                    address serves all your domains. Leave blank to disable.
                  </p>
                </div>
                <div>
                  <v-text-field
                    v-model="form.inbound_maildir_base_path"
                    label="Inbound maildir base path"
                    placeholder="/var/spool/iris/maildirs"
                    variant="outlined"
                    density="compact"
                    hide-details
                    data-testid="inbound-maildir-base-path"
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Filesystem root for inbound <strong>maildir</strong> routes that don't set their own
                    path. kumod writes one Maildir per recipient under
                    <code>&lt;base&gt;/&lt;domain&gt;/&lt;local-part&gt;</code>. Leave blank for the
                    default <code>/var/spool/iris/maildirs</code>.
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-listeners')" id="sec-listeners" class="scroll-target">
              <CardHeader>
                <CardTitle>Listeners</CardTitle>
                <CardDescription>Default binds emitted in the generated policy.</CardDescription>
              </CardHeader>
              <CardContent>
                <v-row dense>
                  <v-col cols="12" sm="6">
                    <v-text-field
                      v-model="form.esmtp_listen"
                      label="ESMTP listen (host:port)"
                      placeholder="0.0.0.0:2525"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" sm="6">
                    <v-text-field
                      v-model="form.http_listen"
                      label="HTTP listen (host:port)"
                      placeholder="0.0.0.0:8000"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                </v-row>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-observability')" id="sec-observability" class="scroll-target">
              <CardHeader>
                <CardTitle>Observability</CardTitle>
                <CardDescription>Metrics source for the dashboard charts.</CardDescription>
              </CardHeader>
              <CardContent>
                <v-text-field
                  v-model="form.prometheus_url"
                  label="Prometheus URL"
                  placeholder="http://localhost:9090"
                  variant="outlined"
                  density="compact"
                  hide-details
                />
                <p class="mt-1 text-caption text-medium-emphasis mb-0">
                  Base URL of the Prometheus that scrapes Iris/KumoMTA. When set, the dashboard
                  shows mail-flow charts. Leave blank to disable.
                </p>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-classify')" id="sec-classify" class="scroll-target">
              <CardHeader>
                <CardTitle>Subject Classification</CardTitle>
                <CardDescription>
                  Optionally label received mail by its subject (≤2 words) via trigram similarity
                  against your rules, falling back to an OpenAI-compatible model. Off by default. The
                  raw subject is never stored on the mail log — only the label.
                </CardDescription>
              </CardHeader>
              <CardContent class="d-flex flex-column ga-4">
                <div>
                  <v-switch
                    v-model="form.classify_subjects"
                    color="primary"
                    density="compact"
                    hide-details
                    label="Classify received mail by subject"
                    data-testid="classify-subjects"
                  />
                  <p class="text-caption text-medium-emphasis mb-0">
                    Requires the IRIS_OPENAI_API_KEY environment variable for the LLM fallback;
                    without it, only your existing rules match.
                  </p>
                </div>
                <v-row dense>
                  <v-col cols="12" sm="6">
                    <v-text-field
                      v-model="form.classify_model"
                      label="Model"
                      placeholder="gpt-4o-mini"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" sm="6">
                    <v-text-field
                      v-model.number="form.classify_threshold"
                      label="Similarity threshold"
                      type="number"
                      min="0"
                      max="1"
                      step="0.05"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                </v-row>
                <p class="text-caption text-medium-emphasis mb-0 mt-n2">
                  Trigram similarity (0–1) required to reuse an existing label before calling the
                  model. Higher = stricter matches, more model calls. Default 0.45.
                </p>
                <div>
                  <v-text-field
                    v-model="form.classify_api_base"
                    label="API base URL"
                    placeholder="https://api.openai.com/v1"
                    variant="outlined"
                    density="compact"
                    hide-details
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    OpenAI-compatible endpoint (OpenAI, Azure OpenAI, or a local gateway).
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-admin')" id="sec-admin" class="scroll-target">
              <CardHeader>
                <CardTitle>Iris Admin Server (this UI)</CardTitle>
                <CardDescription>
                  The address Iris serves this console + API on, and optional HTTPS. Changes apply on a
                  service restart (the listening socket is bound at startup).
                </CardDescription>
              </CardHeader>
              <CardContent class="d-flex flex-column ga-4">
                <div>
                  <v-text-field
                    v-model="form.admin_http_addr"
                    label="Admin bind (host:port)"
                    placeholder=":8080"
                    variant="outlined"
                    density="compact"
                    hide-details
                    style="max-width: 280px"
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Overrides the configured HTTP bind. Leave blank to keep the startup config.
                  </p>
                </div>
                <v-switch
                  v-model="form.admin_tls_enabled"
                  color="primary"
                  density="compact"
                  hide-details
                  label="Serve HTTPS using an issued certificate"
                  data-testid="admin-tls-enabled"
                />
                <div v-if="form.admin_tls_enabled">
                  <v-select
                    v-model="form.admin_tls_cert_domain"
                    :items="certItems"
                    label="Certificate"
                    variant="outlined"
                    density="compact"
                    hide-details
                    :placeholder="issuedCerts.length ? 'Select a certificate…' : 'No issued certificates'"
                    no-data-text="No issued certificates"
                    data-testid="admin-cert"
                  />
                  <p class="mt-1 text-caption text-medium-emphasis mb-0">
                    Issue certificates under TLS Certificates (ACME) first. If the selected cert can't be
                    loaded at startup, Iris falls back to plain HTTP (so a bad pick won't lock you out).
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-injection')" id="sec-injection" class="scroll-target">
              <CardHeader>
                <CardTitle>Injection API (GreenArrow-compatible)</CardTitle>
                <CardDescription>
                  A dedicated mail-injection listener on its own port, isolated from this admin API.
                  Applications authenticate with a username + password (managed under
                  <RouterLink to="/security/injection-credentials" class="text-primary">Access → Injection API</RouterLink>)
                  and mail is forwarded to KumoMTA. Changes apply on a service restart (the socket is
                  bound at startup).
                </CardDescription>
              </CardHeader>
              <CardContent class="d-flex flex-column ga-4">
                <v-switch
                  v-model="form.injection_enabled"
                  color="primary"
                  density="compact"
                  hide-details
                  label="Enable the injection listener"
                  data-testid="injection-enabled"
                />
                <template v-if="form.injection_enabled">
                  <div class="d-flex flex-wrap ga-4">
                    <div>
                      <v-text-field
                        v-model="form.injection_listen_addr"
                        label="Listen address (host:port)"
                        placeholder=":8025"
                        variant="outlined"
                        density="compact"
                        hide-details
                        style="max-width: 240px"
                        data-testid="injection-addr"
                      />
                      <p class="mt-1 text-caption text-medium-emphasis mb-0">
                        Must differ from the admin port. Blank uses :8025.
                      </p>
                    </div>
                    <div>
                      <v-text-field
                        v-model="form.injection_path"
                        label="Request path"
                        placeholder="/api/inject"
                        variant="outlined"
                        density="compact"
                        hide-details
                        style="max-width: 240px"
                        data-testid="injection-path"
                      />
                      <p class="mt-1 text-caption text-medium-emphasis mb-0">
                        The route clients POST to. Blank uses /api/inject.
                      </p>
                    </div>
                  </div>
                  <v-switch
                    v-model="form.injection_tls_enabled"
                    color="primary"
                    density="compact"
                    hide-details
                    label="Serve HTTPS using an issued certificate"
                    data-testid="injection-tls-enabled"
                  />
                  <div v-if="form.injection_tls_enabled">
                    <v-select
                      v-model="form.injection_tls_cert_domain"
                      :items="certItems"
                      label="Certificate"
                      variant="outlined"
                      density="compact"
                      hide-details
                      :placeholder="issuedCerts.length ? 'Select a certificate…' : 'No issued certificates'"
                      no-data-text="No issued certificates"
                      data-testid="injection-cert"
                    />
                    <p class="mt-1 text-caption text-medium-emphasis mb-0">
                      Issue certificates under TLS Certificates (ACME) first. If the cert can't be
                      loaded at startup the service refuses to start (it never serves the injection
                      API in plaintext once HTTPS is requested).
                    </p>
                  </div>
                </template>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-acme')" id="sec-acme" class="scroll-target">
              <CardHeader>
                <CardTitle>ACME Auto-Renew</CardTitle>
                <CardDescription>
                  Certificates auto-renew in the background. Tune the schedule here (duration form, e.g.
                  <code>12h</code>, <code>30d</code>); applies on a service restart. Blank uses the
                  defaults (scan every 12h, renew within 30d of expiry).
                </CardDescription>
              </CardHeader>
              <CardContent>
                <v-row dense>
                  <v-col cols="12" sm="6">
                    <v-text-field
                      v-model="form.acme_renew_interval"
                      label="Scan interval"
                      placeholder="12h"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" sm="6">
                    <v-text-field
                      v-model="form.acme_renew_before"
                      label="Renew before expiry"
                      placeholder="30d"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                </v-row>
              </CardContent>
            </Card>

            <Card v-show="sectionShown.has('sec-monitoring')" id="sec-monitoring" class="scroll-target">
              <CardHeader>
                <CardTitle>Inbox Monitoring</CardTitle>
                <CardDescription>
                  Policy for inbox-placement probes (Monitoring → Inbox Monitoring). The default sender
                  is used for accounts that don't set their own <em>from address</em> — it must be a
                  domain you can send and DKIM-sign from. The durations use duration form (e.g.
                  <code>30s</code>, <code>1h</code>, <code>2h</code>); blank uses the built-in defaults.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <v-row dense>
                  <v-col cols="12">
                    <v-text-field
                      v-model="form.monitoring_from"
                      label="Default probe sender"
                      placeholder="probe@monitor.example.com"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" sm="4">
                    <v-text-field
                      v-model="form.monitoring_reconcile_lookback"
                      label="Reconcile lookback"
                      placeholder="1h"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" sm="4">
                    <v-text-field
                      v-model="form.monitoring_fetch_timeout"
                      label="Mailbox fetch timeout"
                      placeholder="30s"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                  <v-col cols="12" sm="4">
                    <v-text-field
                      v-model="form.monitoring_fetch_giveup"
                      label="Fetch give-up window"
                      placeholder="2h"
                      variant="outlined"
                      density="compact"
                      hide-details
                    />
                  </v-col>
                </v-row>
              </CardContent>
            </Card>

            <!-- Mobile save bar (the sticky nav is hidden below md) -->
            <div class="d-flex d-md-none align-center justify-space-between">
              <v-chip v-if="dirty" size="small" color="warning" variant="tonal">Unsaved changes</v-chip>
              <span v-else />
              <Button type="submit" :disabled="saving">
                {{ saving ? 'Saving…' : 'Save settings' }}
              </Button>
            </div>
          </form>
        </v-col>
      </v-row>
    </DataState>
  </div>
</template>

<style scoped>
.settings-nav {
  position: sticky;
  top: 80px;
}
/* Anchor targets land below the fixed app bar. */
.scroll-target {
  scroll-margin-top: 80px;
}
</style>
