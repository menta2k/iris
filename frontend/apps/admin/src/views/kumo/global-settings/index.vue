<script lang="ts" setup>
import type { AcmeCertificate, GlobalSettings } from '#/api/kumo';

import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Form,
  FormItem,
  Input,
  message,
  Popconfirm,
  Radio,
  RadioGroup,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
} from 'ant-design-vue';

import { acmeApi, globalSettingsApi, policyApi } from '#/api/kumo';

defineOptions({ name: 'GlobalSettings' });

const router = useRouter();
const loading = ref(false);
const saving = ref(false);
const applying = ref(false);
const updatedAt = ref<string>('');
const updatedBy = ref<string>('');

// Local form state. Lists are kept as arrays so the ant-design "tags"
// Select can drive them (operators paste comma-separated values; the
// Select normalises into chips).
const form = reactive<GlobalSettings>({
  kumo_http_listen: '',
  esmtp_listen_addr: '',
  esmtp_relay_hosts: [],
  http_trusted_hosts: [],
  bounce_domain: '',
  bounce_sender_domains: [],
  bounce_prefix: '',
  mail_class_header: '',
  egress_ehlo_domain: '',
  https_listen: '',
  https_cert_pem_path: '',
  https_key_pem_path: '',
});

// HTTPS termination state. Operator picks an issued ACME cert from the
// dropdown; we mirror its on-disk paths into the form, same approach
// the Listener drawer uses (paths are stable across renewals).
const acmeCerts = ref<AcmeCertificate[]>([]);
const httpsEnabled = ref(false);
const httpsSource = ref<'acme' | 'manual'>('acme');
const selectedHttpsCertId = ref<null | number>(null);

const acmeCertsAvailable = computed(() => acmeCerts.value.length > 0);

function detectHttpsState(s: GlobalSettings) {
  httpsEnabled.value = !!s.https_listen;
  if (!httpsEnabled.value) {
    httpsSource.value = 'acme';
    selectedHttpsCertId.value = null;
    return;
  }
  // Try to map the saved paths to one of the issued certs; if it
  // matches we open in ACME mode, otherwise the operator typed paths
  // by hand and we leave it in Manual.
  const match = acmeCerts.value.find(
    (c) =>
      c.cert_pem_path === s.https_cert_pem_path &&
      c.key_pem_path === s.https_key_pem_path,
  );
  if (match?.id) {
    httpsSource.value = 'acme';
    selectedHttpsCertId.value = match.id;
  } else {
    httpsSource.value = 'manual';
    selectedHttpsCertId.value = null;
  }
}

watch(selectedHttpsCertId, (id) => {
  if (httpsSource.value !== 'acme' || id == null) return;
  const cert = acmeCerts.value.find((c) => c.id === id);
  if (!cert) return;
  form.https_cert_pem_path = cert.cert_pem_path ?? '';
  form.https_key_pem_path = cert.key_pem_path ?? '';
});

watch(httpsSource, (mode) => {
  if (mode === 'manual') selectedHttpsCertId.value = null;
});

function labelForCert(c: AcmeCertificate): string {
  const sans = (c.alt_names ?? []).length > 0 ? ` (+${c.alt_names!.length} SAN)` : '';
  return `${c.domain}${sans}`;
}

// Bounce mode is derived from the form, not stored separately. Keeping
// it computed avoids a desync between the radio and the underlying
// fields when the operator switches modes.
const bounceMode = ref<'disabled' | 'multi' | 'single'>('disabled');
function detectMode(s: GlobalSettings) {
  if (s.bounce_sender_domains?.length) return 'multi';
  if (s.bounce_domain) return 'single';
  return 'disabled';
}

async function load() {
  loading.value = true;
  try {
    const [settings, certs] = await Promise.all([
      globalSettingsApi.get(),
      acmeApi.listCertificates().catch(() => ({ items: [] })),
    ]);
    Object.assign(form, settings);
    bounceMode.value = detectMode(settings);
    acmeCerts.value = (certs.items ?? []).filter(
      (c) => c.status === 'issued' && c.cert_pem_path && c.key_pem_path,
    );
    detectHttpsState(settings);
    updatedAt.value = settings.updated_at ?? '';
    updatedBy.value = settings.updated_by ?? '';
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    // Mode gating: only send the fields relevant to the chosen mode.
    // Switching from multi → single without this would silently leave
    // both fields populated; the renderer treats multi as winning, so
    // the operator's "single" choice would be ignored.
    const payload: GlobalSettings = {
      ...form,
      bounce_domain: bounceMode.value === 'single' ? form.bounce_domain : '',
      bounce_sender_domains:
        bounceMode.value === 'multi' ? form.bounce_sender_domains : [],
      // Wipe HTTPS fields when disabled so the next backend boot
      // doesn't keep stale paths around (the validator would reject a
      // half-set state too).
      https_listen: httpsEnabled.value ? form.https_listen : '',
      https_cert_pem_path: httpsEnabled.value ? form.https_cert_pem_path : '',
      https_key_pem_path: httpsEnabled.value ? form.https_key_pem_path : '',
    };
    const r = await globalSettingsApi.update(payload);
    Object.assign(form, r);
    bounceMode.value = detectMode(r);
    updatedAt.value = r.updated_at ?? '';
    updatedBy.value = r.updated_by ?? '';
    message.success('Saved');
  } finally {
    saving.value = false;
  }
}

async function saveAndApply() {
  await save();
  applying.value = true;
  try {
    const r = await policyApi.apply('global-settings update');
    message.success(`Policy applied — ${(r.sha256 ?? '').slice(0, 12)}…`);
  } finally {
    applying.value = false;
  }
}

function gotoPolicyEditor() {
  router.push({ path: '/policy/editor' });
}

onMounted(load);
</script>

<template>
  <Page
    title="Global Settings"
    description="Operator-tunable global knobs that previously required a redeploy. Saving here updates the DB row; click Save & Apply to regenerate init.lua and roll the change to kumomta."
  >
    <Spin :spinning="loading">
      <!-- ───── Listeners ───── -->
      <Card title="Listeners" :body-style="{ padding: '20px' }" class="mb-4">
        <Form :model="form" layout="vertical" :colon="false">
          <FormItem
            label="Kumomta HTTP admin listener"
            help="Bind spec emitted into kumo.start_http_listener. '0.0.0.0:8000' for compose; '127.0.0.1:8025' on host-native to avoid colliding with iris's own :8000."
          >
            <Input
              v-model:value="form.kumo_http_listen"
              placeholder="0.0.0.0:8000"
              style="max-width: 320px"
            />
          </FormItem>
          <FormItem
            label="ESMTP default listen address"
            help="Bind spec for the default kumo.start_esmtp_listener block (host:port; e.g. 0:25, 0:2525, 0.0.0.0:587). Empty defaults to '0:2525'. Only applies when no Listener rows exist on the Listeners page — those override this default."
          >
            <Input
              v-model:value="form.esmtp_listen_addr"
              placeholder="0:2525"
              style="max-width: 320px"
            />
          </FormItem>
          <FormItem
            label="ESMTP relay_hosts (CIDRs)"
            help="Allowed peers for the default ESMTP listener. Empty falls back to RFC1918 + loopback. Per-listener entries (Listeners page) override this default."
          >
            <Select
              v-model:value="form.esmtp_relay_hosts"
              mode="tags"
              placeholder="10.0.0.0/8, 192.168.0.0/16…"
              style="max-width: 540px"
              :token-separators="[',', ' ']"
            />
          </FormItem>
          <FormItem
            label="HTTP trusted_hosts (CIDRs)"
            help="Allowed peers for kumo.start_http_listener. The iris admin-service must be inside this range or it will get 403 on /v1/queues etc."
          >
            <Select
              v-model:value="form.http_trusted_hosts"
              mode="tags"
              placeholder="10.0.0.0/8…"
              style="max-width: 540px"
              :token-separators="[',', ' ']"
            />
          </FormItem>
        </Form>
      </Card>

      <!-- ───── Bounce / DSN ───── -->
      <Card title="Bounce / DSN pipeline" :body-style="{ padding: '20px' }" class="mb-4">
        <Alert
          v-if="bounceMode === 'disabled'"
          type="info"
          message="Bounce pipeline is disabled."
          description="Pick a mode below and configure the matching fields. Both modes also require IRIS_VERP_SECRET to be set (env-only). See README → Bounce / DSN setup for the DNS prerequisites."
          show-icon
          class="mb-3"
        />

        <Form :model="form" layout="vertical" :colon="false">
          <FormItem label="Mode">
            <Select
              v-model:value="bounceMode"
              style="max-width: 320px"
              :options="[
                { label: 'Disabled', value: 'disabled' },
                { label: 'Single-domain (legacy)', value: 'single' },
                { label: 'Multi-domain (per sender)', value: 'multi' },
              ]"
            />
          </FormItem>

          <template v-if="bounceMode === 'single'">
            <FormItem
              label="Bounce domain"
              help="Single-domain mode: every outbound funnels through this one bounce subdomain. Pick a same-org subdomain of your sending domain so DMARC's relaxed alignment passes for all senders."
            >
              <Input
                v-model:value="form.bounce_domain"
                placeholder="bounces.example.com"
                style="max-width: 420px"
              />
            </FormItem>
          </template>

          <template v-if="bounceMode === 'multi'">
            <FormItem
              label="Sender domains"
              help="Multi-domain mode: for each From: domain you send from, the renderer derives a bounce subdomain by convention (<prefix>.<sender>) and routes each sender's bounces to its own DMARC-aligned subdomain. Operator must publish DNS MX + SPF for every derived bounce subdomain."
            >
              <Select
                v-model:value="form.bounce_sender_domains"
                mode="tags"
                placeholder="test-1.com, test2.com…"
                style="max-width: 540px"
                :token-separators="[',', ' ']"
              />
            </FormItem>
            <FormItem
              label="Bounce prefix"
              help="Leading label prepended to each sender domain. Default 'bounces' fits most DNS schemes; override only if you already use a different label."
            >
              <Input
                v-model:value="form.bounce_prefix"
                placeholder="bounces"
                style="max-width: 240px"
              />
            </FormItem>
            <Alert
              v-if="form.bounce_sender_domains?.length"
              type="success"
              show-icon
              class="mb-3"
            >
              <template #message>
                <span style="font-size: 12px">
                  Will accept inbound DSNs at:
                  <Tag
                    v-for="d in form.bounce_sender_domains"
                    :key="d"
                    color="geekblue"
                  >
                    {{ form.bounce_prefix || 'bounces' }}.{{ d }}
                  </Tag>
                </span>
              </template>
            </Alert>
          </template>
        </Form>
      </Card>

      <!-- ───── Iris admin HTTPS ───── -->
      <Card title="Admin HTTPS" :body-style="{ padding: '20px' }" class="mb-4">
        <Alert
          v-if="!httpsEnabled"
          type="info"
          show-icon
          class="mb-3"
          message="Admin HTTPS is disabled."
          description="When enabled, iris terminates TLS on the bind below and reverse-proxies to the existing plain HTTP server (default :8000). Operator-side requirements: an issued ACME cert (or manually-supplied cert+key files), and the configured port reachable from your audience."
        />

        <Form :model="form" layout="vertical" :colon="false">
          <FormItem>
            <Space>
              <Switch v-model:checked="httpsEnabled" />
              <span>Enable admin HTTPS termination</span>
            </Space>
          </FormItem>

          <template v-if="httpsEnabled">
            <FormItem
              label="Listen"
              help="host:port the HTTPS server binds. Default :443. Inside the container the process runs as root in dev compose, so binding <1024 works; on prod with a non-root container add cap_net_bind_service or pick a port ≥1024 and forward from your reverse proxy."
              :rules="[{ required: true, message: 'Listen address is required' }]"
            >
              <Input
                v-model:value="form.https_listen"
                placeholder=":443"
                style="max-width: 240px"
              />
            </FormItem>

            <FormItem label="Certificate source">
              <RadioGroup v-model:value="httpsSource">
                <Radio value="acme" :disabled="!acmeCertsAvailable">
                  Issued ACME certificate
                </Radio>
                <Radio value="manual">Manual paths</Radio>
              </RadioGroup>
              <div v-if="!acmeCertsAvailable" class="hint">
                No issued ACME certificates found.
                <a href="/security/certificates">Issue one</a> first, or pick
                Manual to point at cert+key files you've already got on disk.
              </div>
            </FormItem>

            <template v-if="httpsSource === 'acme'">
              <FormItem
                label="Certificate"
                :rules="[{ required: true, message: 'Pick an issued certificate' }]"
              >
                <Select
                  :value="selectedHttpsCertId ?? undefined"
                  placeholder="Select an issued certificate…"
                  style="width: 100%; max-width: 540px"
                  @change="(v: any) => (selectedHttpsCertId = (v as null | number) ?? null)"
                >
                  <Select.Option
                    v-for="c in acmeCerts"
                    :key="c.id"
                    :value="c.id"
                  >
                    {{ labelForCert(c) }}
                  </Select.Option>
                </Select>
              </FormItem>
            </template>

            <FormItem label="Certificate path">
              <Input
                v-model:value="form.https_cert_pem_path"
                :disabled="httpsSource === 'acme'"
                placeholder="/opt/kumomta/etc/tls/iris.example.com/fullchain.pem"
                style="max-width: 540px"
              />
            </FormItem>
            <FormItem label="Private key path">
              <Input
                v-model:value="form.https_key_pem_path"
                :disabled="httpsSource === 'acme'"
                placeholder="/opt/kumomta/etc/tls/iris.example.com/privkey.pem"
                style="max-width: 540px"
              />
            </FormItem>

            <Alert
              type="info"
              show-icon
              class="mt-2"
              message="Plain HTTP stays available."
              description="Enabling HTTPS doesn't disable the existing plain :8000 listener — the HTTPS server reverse-proxies to it. Front iris with a load balancer that redirects http://… to https://… if you want HTTP-free access; iris itself keeps both paths open so internal /healthz callers don't break."
            />
          </template>
        </Form>
      </Card>

      <!-- ───── Misc ───── -->
      <Card title="Other" :body-style="{ padding: '20px' }" class="mb-4">
        <Form :model="form" layout="vertical" :colon="false">
          <FormItem
            label="Mail-class header"
            help="Header inspected by the mail-class router. Default 'X-Kumo-Mail-Class' fits the iris convention; override only when integrating with an existing system that uses a different header."
          >
            <Input
              v-model:value="form.mail_class_header"
              placeholder="X-Kumo-Mail-Class"
              style="max-width: 320px"
            />
          </FormItem>

          <FormItem
            label="Outbound EHLO hostname"
            help="Default FQDN announced on outbound SMTP (EHLO). Set this to a resolvable name (e.g. mail.example.com) so receivers don't penalise the bare system hostname (rspamd HFILTER_HELO_5). A per-VMTA HELO name overrides this. Also used as the domain for any Message-ID iris adds. Leave blank to keep kumomta's default."
          >
            <Input
              v-model:value="form.egress_ehlo_domain"
              placeholder="mail.example.com"
              style="max-width: 320px"
            />
          </FormItem>
        </Form>
      </Card>

      <!-- ───── Actions ───── -->
      <Card :body-style="{ padding: '16px 20px' }">
        <Space :size="16" wrap>
          <Button :loading="saving" @click="save">Save</Button>
          <Popconfirm
            title="Save and apply the policy now?"
            ok-text="Save & Apply"
            ok-type="primary"
            @confirm="saveAndApply"
          >
            <Button type="primary" :loading="saving || applying" danger>
              Save &amp; Apply policy
            </Button>
          </Popconfirm>
          <Button :disabled="loading" @click="gotoPolicyEditor">
            Open Policy Editor
          </Button>
          <span v-if="updatedAt" class="meta">
            Last updated:&nbsp;
            <span class="when">{{ updatedAt }}</span>
            <span v-if="updatedBy">&nbsp;by <code>{{ updatedBy }}</code></span>
          </span>
        </Space>
      </Card>
    </Spin>
  </Page>
</template>

<style scoped>
.meta {
  color: var(--ant-color-text-tertiary);
  font-size: 12px;
}
.meta .when {
  font-family: ui-monospace, 'SFMono-Regular', 'Menlo', 'Consolas', monospace;
}
.hint {
  font-size: 12px;
  color: var(--ant-color-text-tertiary);
  margin-top: 6px;
  line-height: 1.45;
}
.mt-2 {
  margin-top: 12px;
}
</style>
