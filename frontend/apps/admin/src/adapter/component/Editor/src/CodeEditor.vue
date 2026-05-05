<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from 'vue';

import { $t } from '@vben/locales';
import { preferences } from '@vben/preferences';

import hljs from 'highlight.js';
import * as monaco from 'monaco-editor';

import { initMonacoWorkers } from './monaco-loader';
import { isDarkMode } from './utils';

// Tighter monaco-theme typing than the upstream `string` for safety.
type MonacoTheme = 'dark' | 'hc-black' | 'light' | 'vs' | 'vs-dark';
type EditorLanguage =
  | 'c'
  | 'cpp'
  | 'csharp'
  | 'css'
  | 'go'
  | 'html'
  | 'java'
  | 'javascript'
  | 'json'
  | 'plaintext'
  | 'python'
  | 'typescript'
  | string;

interface Props {
  modelValue: string;
  height?: number | string;
  disabled?: boolean;
  placeholder?: string;
  autoDetectLanguage?: boolean;
  options?: {
    fontSize?: number;
    language?: EditorLanguage;
    lineNumbers?: boolean;
    // Extend with more native monaco options as needed.
    minimap?: boolean;
    tabSize?: number;
    theme?: MonacoTheme;
    wordWrap?: 'bounded' | 'off' | 'on' | 'wordWrapColumn';
  };
}

// Props + defaults (must come before any non-import code).
const props = withDefaults(defineProps<Props>(), {
  autoDetectLanguage: true, // auto-detection on by default
  disabled: false,
  height: '100%',
  placeholder: $t('ui.editor.please_input_content'),
  options: () => ({
    language: 'javascript',
    theme: 'light',
    lineNumbers: true,
    tabSize: 2,
    minimap: false,
    fontSize: 14,
    wordWrap: 'on',
  }),
});

const emit = defineEmits<{
  (e: 'change', value: string): void;
  (e: 'error', error: Error): void;
  (e: 'ready'): void;
  (e: 'update:modelValue', value: string): void;
}>();

const languageMap: Record<string, EditorLanguage> = {
  javascript: 'javascript',
  js: 'javascript',
  typescript: 'typescript',
  ts: 'typescript',
  json: 'json',
  html: 'html',
  css: 'css',
  python: 'python',
  java: 'java',
  sql: 'sql',
  markdown: 'markdown',
  shell: 'shell',
  php: 'php',
  go: 'go',
  golang: 'go',
  ruby: 'ruby',
  c: 'c',
  'c++': 'cpp',
  cplusplus: 'cpp',
  cpp: 'cpp',
  'c#': 'csharp',
  csharp: 'csharp',
};

/**
 * Detect a programming language from the buffer content.
 */
const detectLanguage = (content: string): EditorLanguage => {
  try {
    if (!content || content.trim() === '') {
      return props.options?.language || 'plaintext';
    }

    // 1. Try JSON first — fastest and most accurate.
    if (content.trim().startsWith('{') || content.trim().startsWith('[')) {
      try {
        JSON.parse(content);
        return 'json';
      } catch {}
    }

    const detectedLanguage = hljs.highlightAuto(content);

    // 2. Fall back to highlight.js auto-detection.
    const detected = detectedLanguage.language;
    const detectedKey =
      typeof detected === 'string' ? detected.toLowerCase() : '';
    // 3. Map to a monaco-supported language ID; default to plaintext.
    return languageMap[detectedKey] || 'plaintext';
  } catch (error) {
    emit(
      'error',
      new Error(`Language detection failed: ${(error as Error).message}`),
    );
    return props.options?.language || 'plaintext';
  }
};

// Initialise monaco's web workers — once per page lifecycle.
if (typeof window !== 'undefined') {
  initMonacoWorkers();
}

// Reactive state.
const editorContainer = ref<HTMLDivElement | null>(null);
let editor: monaco.editor.IStandaloneCodeEditor | null = null;
let editorModel: monaco.editor.ITextModel | null = null; // tracked separately so we can dispose deterministically
const isUpdatingFromProp = ref(false); // reactive to avoid stale-closure traps

// Editor height as a CSS value, with a 200px floor.
const editorHeight = computed(() => {
  if (typeof props.height === 'number') {
    return `${Math.max(props.height, 200)}px`; // 200px minimum
  }
  // Pass-through for percentage / px strings; default 100%.
  return props.height?.toString() || '100%';
});

// Resolve the theme name. Honours an explicit non-light/dark choice,
// otherwise tracks the project's dark-mode preference.
const themeName = computed<MonacoTheme>(() => {
  const propsTheme = props.options?.theme;
  // An explicit non-trivial theme wins.
  if (propsTheme && propsTheme !== 'light' && propsTheme !== 'dark') {
    return propsTheme;
  }
  // Otherwise mirror the global dark-mode flag.
  return isDarkMode() ? 'vs-dark' : 'vs';
});

// Sync v-model changes from outside the editor, with cursor preservation.
watch(
  () => props.modelValue,
  async (newVal) => {
    if (!editor || isUpdatingFromProp.value) return;

    // Wait for the DOM to settle before mutating the editor.
    await nextTick();

    // Skip if the editor already has the same value.
    const currentValue = editor.getValue();
    if (currentValue === newVal) {
      return;
    }

    isUpdatingFromProp.value = true;

    // Save the current caret so it survives the setValue.
    const currentPosition = editor.getPosition();

    // monaco has no native placeholder; we emulate it (see below).
    const valueToSet = newVal || '';
    editor.setValue(valueToSet);

    // Re-detect language on content replace if enabled.
    if (props.autoDetectLanguage && editorModel && valueToSet.trim()) {
      const detectedLanguage = detectLanguage(valueToSet);
      monaco.editor.setModelLanguage(editorModel, detectedLanguage);
    }

    // Restore the caret so the user doesn't jump to column 1.
    if (currentPosition && valueToSet.length >= currentPosition.column) {
      editor.setPosition(currentPosition);
    }

    isUpdatingFromProp.value = false;
  },
  { immediate: true, flush: 'post' }, // run after the DOM update
);

// React to theme-name changes from props.
watch(
  () => themeName.value,
  (newTheme) => {
    if (editor) {
      // Set theme globally, then force a layout repaint.
      monaco.editor.setTheme(newTheme);
      editor.layout();
    }
  },
  { immediate: true },
);

// React to global dark-mode preference changes.
watch(
  () => preferences.theme.mode,
  () => {
    // The themeName computed re-runs and triggers the watcher above —
    // this watcher also flips the theme directly so the layout repaints
    // even when the explicit `options.theme` prop pinned an override.
    if (editor) {
      const newTheme = isDarkMode() ? 'vs-dark' : 'vs';
      monaco.editor.setTheme(newTheme);
      editor.layout();
    }
  },
);

watch(
  () => props.disabled,
  (disabled) => {
    if (editor) {
      editor.updateOptions({ readOnly: disabled });
    }
  },
);

// Initialise the monaco instance.
onMounted(async () => {
  try {
    if (!editorContainer.value) {
      const error = new Error('Editor container element not found');
      emit('error', error);
      console.error('Monaco editor initialization failed:', error);
      return;
    }

    await nextTick(); // wait for the container to mount

    // Create the model separately so we can dispose it explicitly.
    const initialLanguage = props.autoDetectLanguage
      ? detectLanguage(props.modelValue || '')
      : props.options?.language || 'javascript';

    editorModel = monaco.editor.createModel(
      props.modelValue || '',
      initialLanguage,
    );

    // Create the editor instance.
    editor = monaco.editor.create(editorContainer.value, {
      model: editorModel,
      theme: themeName.value,
      automaticLayout: true, // resize with container
      minimap: {
        enabled: props.options?.minimap !== false,
      },
      lineNumbers: props.options?.lineNumbers === false ? 'off' : 'on',
      tabSize: props.options?.tabSize || 2,
      insertSpaces: true,
      readOnly: props.disabled,
      scrollBeyondLastLine: false,
      wordWrap: props.options?.wordWrap || 'on',
      fontSize: props.options?.fontSize || 14,
      fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace",
      lineHeight: 1.6,
      // UX touches.
      quickSuggestions: !props.disabled, // suggestions off when disabled
      codeLens: !props.disabled,
      folding: true, // enable code folding
      colorDecorators: true,
      // Perf.
      renderLineHighlight: 'gutter',
      scrollbar: {
        vertical: 'visible',
        horizontal: 'auto',
      },
    });

    // Forward content changes upward, debounced.
    let changeTimeout: null | number = null;
    editor.onDidChangeModelContent(() => {
      if (isUpdatingFromProp.value || !editor) return;

      // Debounce so high-frequency edits don't spam parent re-renders.
      if (changeTimeout) clearTimeout(changeTimeout);
      changeTimeout = window.setTimeout(() => {
        const newValue = editor!.getValue() || '';
        emit('update:modelValue', newValue);
        emit('change', newValue);
      }, 100);
    });

    // Synthesise a placeholder — monaco doesn't ship one natively.
    const updatePlaceholder = () => {
      if (!editor || !props.placeholder) return;
      const value = editor.getValue().trim();
      const domNode = editor.getDomNode();
      if (domNode) {
        const placeholderEl = domNode.querySelector('.monaco-placeholder');
        if (!value && !placeholderEl) {
          const placeholder = document.createElement('div');
          placeholder.className = 'monaco-placeholder';
          placeholder.style.position = 'absolute';
          placeholder.style.top = '10px';
          placeholder.style.left = '10px';
          placeholder.style.color = '#999';
          placeholder.style.pointerEvents = 'none';
          placeholder.style.fontSize = `${props.options?.fontSize || 14}px`;
          placeholder.textContent = props.placeholder;
          domNode.append(placeholder);
        } else if (value && placeholderEl) {
          placeholderEl.remove();
        }
      }
    };

    // Initialise the placeholder and re-evaluate on every change.
    updatePlaceholder();
    editor.onDidChangeModelContent(updatePlaceholder);

    // Signal readiness to the parent.
    emit('ready');
  } catch (error) {
    emit('error', error as Error);
    console.error('Monaco editor initialization failed:', error);
  }
});

// Dispose the editor + model on unmount to prevent leaks.
onBeforeUnmount(() => {
  if (editor) {
    editor.dispose();
    editor = null;
  }
  if (editorModel) {
    editorModel.dispose();
    editorModel = null;
  }
  // Optionally drop every cached model — defensive on rapid re-mounts.
  monaco.editor.getModels().forEach((model) => model.dispose());
});

// Expose imperative editor methods to the parent.
defineExpose({
  focus: () => {
    editor?.focus();
    editor?.revealLine(1);
  },
  setLanguage: (language: EditorLanguage) => {
    if (editor && editorModel) {
      monaco.editor.setModelLanguage(editorModel, language);
    }
  },
  getValue: (): string => {
    return editor?.getValue() || '';
  },
  setValue: (value: string) => {
    if (editor) {
      isUpdatingFromProp.value = true;
      editor.setValue(value);
      isUpdatingFromProp.value = false;
    }
  },
  getEditorInstance: (): monaco.editor.IStandaloneCodeEditor | null => {
    return editor;
  },
  formatCode: () => {
    if (editor && editorModel && !props.disabled) {
      // Formatting requires an external formatter (e.g. prettier).
      // The interface is here; the integration is left to the caller.
      console.info('Format code - requires external formatter integration');
    }
  },
});
</script>

<template>
  <div class="code-editor-wrapper">
    <div
      ref="editorContainer"
      class="code-editor-container"
      :style="{ height: editorHeight }"
      :class="{ 'code-editor-disabled': disabled }"
    ></div>
  </div>
</template>

<style scoped>
.code-editor-wrapper {
  position: relative;
  box-sizing: border-box;
  width: 100%;
  height: 100%;
}

.code-editor-container {
  box-sizing: border-box;
  width: 100%;
  min-height: 200px; /* min height */
  overflow: hidden;
  border: 1px solid var(--border-color, #d9d9d9);
  border-radius: 6px;
  transition: border-color 0.2s ease;
}

/* Disabled state. */
.code-editor-disabled {
  cursor: not-allowed;
  opacity: 0.85;
}

/* Pierce-through styling for the monaco editor. */
.code-editor-container :deep(.monaco-editor) {
  font-size: 14px !important;
}

.code-editor-container :deep(.monaco-editor .monaco-scrollable-element) {
  scrollbar-width: thin;
}

/* Dark-mode scrollbar styling. */
.code-editor-container,
:deep(.monaco-editor .monaco-scrollable-element::-webkit-scrollbar-thumb) {
  background-color: #555 !important;
}

.code-editor-container,
:deep(.monaco-editor .monaco-scrollable-element::-webkit-scrollbar-track) {
  background-color: #222 !important;
}
</style>
