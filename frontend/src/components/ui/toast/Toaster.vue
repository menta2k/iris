<script setup lang="ts">
import { useToast, type ToastVariant } from '@/composables/useToast'

const { toasts, dismiss } = useToast()

// Map legacy variants onto v-alert types; 'default' renders an untyped
// elevated surface.
const ALERT_TYPES: Partial<Record<ToastVariant, 'success' | 'error' | 'warning'>> = {
  success: 'success',
  destructive: 'error',
  warning: 'warning',
}
</script>

<template>
  <Teleport to="body">
    <div class="toaster d-flex flex-column ga-2 pa-4">
      <TransitionGroup name="toast">
        <v-alert
          v-for="t in toasts"
          :key="t.id"
          :type="ALERT_TYPES[t.variant]"
          variant="elevated"
          density="comfortable"
          closable
          class="toast-item"
          role="status"
          @click:close="dismiss(t.id)"
        >
          <p class="text-body-2 font-weight-bold">{{ t.title }}</p>
          <p v-if="t.description" class="mt-1 text-body-2">
            {{ t.description }}
          </p>
        </v-alert>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toaster {
  position: fixed;
  bottom: 0;
  right: 0;
  z-index: 3000;
  width: 100%;
  max-width: 400px;
  pointer-events: none;
}
.toast-item {
  pointer-events: auto;
}
.toast-enter-active,
.toast-leave-active {
  transition: all 0.2s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(1rem);
}
</style>
