<script lang="ts" setup>
import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  Form,
  FormItem,
  Input,
  message,
  Space,
  Spin,
} from 'ant-design-vue';

import { listenerApi } from '#/api/kumo';

defineOptions({ name: 'Listener' });

const form = reactive({
  trusted_hosts_text: '',
  relay_hosts_text: '',
});
const loading = ref(false);
const saving = ref(false);

async function load() {
  loading.value = true;
  try {
    const cfg = await listenerApi.get();
    form.trusted_hosts_text = (cfg.trusted_hosts ?? []).join('\n');
    form.relay_hosts_text = (cfg.relay_hosts ?? []).join('\n');
  } catch {
    // backend may not yet expose this endpoint — keep fields empty
  } finally {
    loading.value = false;
  }
}

function parse(value: string): string[] {
  return value.split(/\s+/).map((s) => s.trim()).filter(Boolean);
}

async function save() {
  saving.value = true;
  try {
    await listenerApi.update({
      trusted_hosts: parse(form.trusted_hosts_text),
      relay_hosts: parse(form.relay_hosts_text),
    });
    message.success('Listener configuration saved');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page title="Listener" description="Inbound SMTP listener — trusted and relay hosts">
    <Card :body-style="{ padding: '16px' }">
      <Spin :spinning="loading">
        <Form :model="form" layout="vertical">
          <FormItem label="Trusted hosts (one per line)" name="trusted_hosts_text">
            <Input.TextArea
              v-model:value="form.trusted_hosts_text"
              :rows="6"
              placeholder="10.0.0.0/8&#10;192.168.0.0/16"
            />
          </FormItem>
          <FormItem label="Relay hosts (one per line)" name="relay_hosts_text">
            <Input.TextArea
              v-model:value="form.relay_hosts_text"
              :rows="6"
              placeholder="relay.example.com"
            />
          </FormItem>
          <Space>
            <Button type="primary" :loading="saving" @click="save">Save</Button>
            <Button :loading="loading" @click="load">Reload</Button>
          </Space>
        </Form>
      </Spin>
    </Card>
  </Page>
</template>
