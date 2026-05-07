<script lang="ts" setup>
import type { AcmeAccount } from '#/api/kumo';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Form,
  FormItem,
  Input,
  message,
  Radio,
  RadioGroup,
  Space,
  Spin,
  Tag,
} from 'ant-design-vue';

import { acmeApi } from '#/api/kumo';

defineOptions({ name: 'AcmeSettings' });

const LE_PROD = 'https://acme-v02.api.letsencrypt.org/directory';
const LE_STAGING = 'https://acme-staging-v02.api.letsencrypt.org/directory';

const loading = ref(false);
const saving = ref(false);
const account = ref<AcmeAccount>({
  email: '',
  server_url: '',
  has_registration: false,
});

const form = reactive({
  email: '',
  server_url: LE_STAGING,
});

// Convenience radio: "staging / prod / custom" derived from server_url.
const directoryPreset = computed<'custom' | 'prod' | 'staging'>({
  get() {
    if (form.server_url === LE_PROD) return 'prod';
    if (form.server_url === LE_STAGING) return 'staging';
    return 'custom';
  },
  set(v) {
    if (v === 'prod') form.server_url = LE_PROD;
    else if (v === 'staging') form.server_url = LE_STAGING;
    // 'custom' leaves whatever's there — operator types it
  },
});

async function load() {
  loading.value = true;
  try {
    const r = await acmeApi.getAccount();
    account.value = r;
    form.email = r.email;
    if (r.server_url) form.server_url = r.server_url;
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!form.email.trim()) {
    message.warning('Email is required');
    return;
  }
  if (!form.server_url.trim()) {
    message.warning('ACME directory URL is required');
    return;
  }
  saving.value = true;
  try {
    const r = await acmeApi.saveAccount({
      email: form.email.trim(),
      server_url: form.server_url.trim(),
    });
    account.value = r;
    message.success('ACME account saved');
  } finally {
    saving.value = false;
  }
}
onMounted(load);
</script>

<template>
  <Page
    title="ACME Settings"
    description="ACME account used to issue and renew TLS certificates. One account, multiple certs. The account-level RSA key is generated automatically on first save and stored on the singleton acme_account row."
  >
    <Spin :spinning="loading">
      <Card title="Account" :body-style="{ padding: '20px' }" class="mb-4">
        <Alert
          v-if="!account.email"
          type="info"
          show-icon
          message="No ACME account configured yet."
          description="Fill the form below and click Save. The first save also generates the RSA-2048 account key. Use staging while testing — Let's Encrypt's prod has tight rate limits and burnt issuance quota for a domain isn't reset for a week."
          class="mb-3"
        />
        <Alert
          v-else-if="!account.has_registration"
          type="warning"
          show-icon
          message="Account configured but not yet registered."
          description="Registration runs lazily on the first issue request. If that worries you, issue a test cert against staging now to force a Register call."
          class="mb-3"
        />
        <Alert
          v-else
          type="success"
          show-icon
          message="Account is registered with the directory."
          class="mb-3"
        />

        <Form :model="form" layout="vertical" :colon="false">
          <FormItem
            label="Contact email"
            name="email"
            help="Used for ToS acceptance and expiry / revocation notices from the CA."
            :rules="[{ required: true, message: 'Email is required' }]"
          >
            <Input
              v-model:value="form.email"
              placeholder="ops@example.com"
              style="max-width: 360px"
            />
          </FormItem>

          <FormItem label="ACME directory">
            <RadioGroup v-model:value="directoryPreset">
              <Radio value="staging">Let's Encrypt (staging)</Radio>
              <Radio value="prod">Let's Encrypt (prod)</Radio>
              <Radio value="custom">Custom</Radio>
            </RadioGroup>
            <Input
              v-model:value="form.server_url"
              :disabled="directoryPreset !== 'custom'"
              placeholder="https://acme-v02.api.letsencrypt.org/directory"
              style="margin-top: 8px"
            />
            <div class="hint">
              <strong>Use staging for first-time setup.</strong> Validate the
              full pipeline (HTTP-01 reachability or DNS provider creds) on
              staging, then switch to prod. Switching servers wipes the saved
              registration; the next issue re-registers.
            </div>
          </FormItem>

          <Space>
            <Button type="primary" :loading="saving" @click="save">Save</Button>
          </Space>

          <div v-if="account.updated_at" class="meta">
            Last updated: <span class="when">{{ account.updated_at }}</span>
          </div>
        </Form>
      </Card>

      <Card title="Next steps" :body-style="{ padding: '20px' }">
        <ul class="next">
          <li>
            <strong>HTTP-01:</strong> the iris admin-service hosts the challenge
            listener on <Tag color="geekblue">:80</Tag> automatically (set
            <code>IRIS_ACME_HTTP_BIND=off</code> if you front iris with a reverse
            proxy that forwards <code>/.well-known/acme-challenge/*</code>).
            Each domain you issue must resolve to that public IP.
          </li>
          <li>
            <strong>DNS-01:</strong> head to <strong>DNS Providers</strong> to
            save credentials for one of the 10 supported providers, then
            issue with <code>challenge_type: dns-01</code>.
          </li>
          <li>
            Issue your first cert from the <strong>Certificates</strong> page.
            PEMs land at
            <code>/opt/kumomta/etc/tls/&lt;domain&gt;/{fullchain,privkey}.pem</code>
            so kumomta listeners can reference them directly.
          </li>
        </ul>
      </Card>
    </Spin>
  </Page>
</template>

<style scoped>
.hint {
  font-size: 12px;
  color: var(--ant-color-text-tertiary);
  margin-top: 8px;
  line-height: 1.45;
}
.meta {
  font-size: 12px;
  color: var(--ant-color-text-tertiary);
  margin-top: 16px;
}
.meta .when {
  font-family: ui-monospace, 'SFMono-Regular', 'Menlo', 'Consolas', monospace;
}
.next {
  margin: 0;
  padding-left: 20px;
  font-size: 13px;
  line-height: 1.6;
}
.next code {
  font-size: 12px;
}
</style>
