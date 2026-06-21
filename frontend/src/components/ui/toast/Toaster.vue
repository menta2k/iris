<script setup lang="ts">
import { useToast } from '@/composables/useToast'
import { cn } from '@/lib/utils'

const { toasts, dismiss } = useToast()

const variantClasses: Record<string, string> = {
  default: 'border-border bg-card',
  success: 'border-success/40 bg-card',
  destructive: 'border-destructive/40 bg-card',
  warning: 'border-warning/40 bg-card',
}
</script>

<template>
  <Teleport to="body">
    <div class="fixed bottom-0 right-0 z-[100] flex w-full max-w-sm flex-col gap-2 p-4">
      <TransitionGroup name="toast">
        <div
          v-for="t in toasts"
          :key="t.id"
          :class="
            cn(
              'pointer-events-auto rounded-md border p-4 shadow-lg',
              variantClasses[t.variant] ?? variantClasses.default,
            )
          "
          role="status"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-sm font-semibold">{{ t.title }}</p>
              <p v-if="t.description" class="mt-1 text-sm text-muted-foreground">
                {{ t.description }}
              </p>
            </div>
            <button
              class="text-muted-foreground hover:text-foreground"
              aria-label="Dismiss"
              @click="dismiss(t.id)"
            >
              &times;
            </button>
          </div>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
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
