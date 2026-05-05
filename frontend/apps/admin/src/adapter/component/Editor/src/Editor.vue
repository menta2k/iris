<script setup lang="ts">
import type { EditorEmits, EditorProps } from '../types';

import { computed, defineAsyncComponent } from 'vue';

import { $t } from '@vben/locales';

import { EditorType } from '../types';

const props = withDefaults(defineProps<EditorProps>(), {
  editorType: EditorType.PLAIN_TEXT,
  height: '100%',
  disabled: false,
  placeholder: $t('ui.editor.please_input_content'),
});

const emit = defineEmits<EditorEmits>();

const PlainTextEditor = defineAsyncComponent(
  () => import('./PlainTextEditor.vue'),
);
const CodeEditor = defineAsyncComponent(() => import('./CodeEditor.vue'));

const currentEditorComponent = computed(() => {
  switch (props.editorType) {
    case EditorType.CODE:
    case EditorType.VISUAL_BUILDER: {
      return CodeEditor;
    }
    default: {
      return PlainTextEditor;
    }
  }
});

const handleUpdate = (value: string) => {
  emit('update:modelValue', value);
};

const handleChange = (value: string) => {
  emit('change', value);
};

const handleReady = () => {
  emit('ready');
};

const currentOptions = computed(() => {
  return props.editorType === EditorType.CODE ? props.codeOptions : undefined;
});
</script>

<template>
  <div class="editor-container">
    <component
      :is="currentEditorComponent"
      :model-value="modelValue"
      :height="height"
      :disabled="disabled"
      :placeholder="placeholder"
      :upload-image="uploadImage"
      :options="currentOptions"
      @update:model-value="handleUpdate"
      @change="handleChange"
      @ready="handleReady"
    />
  </div>
</template>

<style scoped>
.editor-container {
  width: 100%;
  height: 100%;
  min-height: 300px;
}
</style>
