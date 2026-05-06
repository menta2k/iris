<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  message,
  Popconfirm,
  Radio,
  RadioGroup,
  Space,
  Spin,
  Tag,
  Typography,
} from 'ant-design-vue';

import { policyApi } from '#/api/kumo';

defineOptions({ name: 'Policy' });

// "active" = init.lua on disk, what kumomta is running right now.
// "preview" = the renderer's output from the current DB snapshot,
// which is what the next /v1/policy/apply will write. Operators usually
// want active (effective state); preview is the diff target before they
// hit Apply.
type View = 'active' | 'preview';
const view = ref<View>('active');

const source = ref('');
const sha = ref('');
const loading = ref(false);
const validating = ref(false);
const applying = ref(false);
const regenerating = ref(false);
const validation = ref<{ valid: boolean; issues?: string[] } | null>(null);

const shaShort = computed(() => (sha.value ? sha.value.slice(0, 12) : ''));
const isEmptyActive = computed(
  () => view.value === 'active' && !source.value && !loading.value,
);

async function load() {
  loading.value = true;
  validation.value = null;
  try {
    const r =
      view.value === 'active'
        ? await policyApi.active()
        : await policyApi.render();
    source.value = r.lua ?? '';
    sha.value = r.sha256 ?? '';
  } finally {
    loading.value = false;
  }
}

async function validate() {
  // The backend validates the current DB snapshot (not the textarea
  // contents). Kept as a sanity check — useful before clicking Apply
  // because Apply also re-renders from DB and writes regardless of
  // what's in the editor.
  validating.value = true;
  try {
    validation.value = await policyApi.validate();
    if (validation.value.valid) {
      message.success('Policy is valid');
    } else {
      message.warning(
        `Policy has ${validation.value.issues?.length ?? 0} issue(s)`,
      );
    }
  } finally {
    validating.value = false;
  }
}

async function regenerate() {
  // Forces a fresh render of init.lua from the current DB snapshot and
  // shows it in the Preview pane so operators can review before Apply.
  regenerating.value = true;
  try {
    view.value = 'preview';
    const r = await policyApi.render();
    source.value = r.lua ?? '';
    sha.value = r.sha256 ?? '';
    validation.value = null;
    message.success(`Regenerated — ${(r.sha256 ?? '').slice(0, 12)}…`);
  } finally {
    regenerating.value = false;
  }
}

async function apply() {
  applying.value = true;
  try {
    const r = await policyApi.apply('applied from editor');
    message.success(`Policy applied — ${r.sha256.slice(0, 12)}…`);
    validation.value = null;
    // Refresh the active view so the operator sees the new on-disk file
    // (and any drift between preview and active goes to zero).
    view.value = 'active';
    await load();
  } finally {
    applying.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page
    title="Policy Editor"
    description="View, validate, and apply the kumomta Lua delivery policy"
  >
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3" wrap>
        <RadioGroup
          v-model:value="view"
          option-type="button"
          button-style="solid"
          :disabled="loading"
          @change="load"
        >
          <Radio value="active">Active (init.lua)</Radio>
          <Radio value="preview">Preview (from DB)</Radio>
        </RadioGroup>

        <Tag v-if="shaShort" color="blue">sha256:{{ shaShort }}</Tag>

        <Button :loading="loading" @click="load">Reload</Button>
        <Button :loading="regenerating" @click="regenerate">Regenerate</Button>
        <Button :loading="validating" @click="validate">Validate</Button>
        <Popconfirm
          title="Apply the current DB snapshot to all kumomta nodes?"
          ok-text="Apply"
          ok-type="danger"
          @confirm="apply"
        >
          <Button type="primary" :loading="applying" danger>Apply</Button>
        </Popconfirm>
      </Space>

      <Alert
        v-if="isEmptyActive"
        type="info"
        message="No policy applied yet"
        description="kumomta will not have a customised init.lua until you click Apply."
        show-icon
        class="mb-3"
      />
      <Alert
        v-if="validation?.valid"
        type="success"
        message="Policy is valid"
        show-icon
        class="mb-3"
      />
      <Alert
        v-else-if="validation && !validation.valid"
        type="error"
        :message="`Policy has ${validation.issues?.length ?? 0} issue(s)`"
        show-icon
        class="mb-3"
      >
        <template #description>
          <ul style="margin: 0; padding-left: 20px">
            <li v-for="(err, i) in validation.issues" :key="i">
              {{ err }}
            </li>
          </ul>
        </template>
      </Alert>

      <Spin :spinning="loading">
        <Typography.Paragraph>
          <textarea
            v-model="source"
            class="policy-editor"
            spellcheck="false"
            :placeholder="
              view === 'active'
                ? '-- (no init.lua on disk yet — click Apply to generate one)'
                : '-- (DB snapshot empty — preview will be a near-empty policy)'
            "
            readonly
          />
        </Typography.Paragraph>
      </Spin>
    </Card>
  </Page>
</template>

<style scoped>
.policy-editor {
  width: 100%;
  min-height: 480px;
  padding: 12px;
  font-family: ui-monospace, 'SFMono-Regular', 'Menlo', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.55;
  color: var(--ant-color-text);
  background: var(--ant-color-bg-elevated);
  border: 1px solid var(--ant-color-border);
  border-radius: 6px;
  resize: vertical;
}
</style>
