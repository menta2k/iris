import { ref } from 'vue'

export type ToastVariant = 'default' | 'success' | 'destructive' | 'warning'

export interface Toast {
  id: number
  title: string
  description?: string
  variant: ToastVariant
}

const toasts = ref<Toast[]>([])
let counter = 0

export function useToast() {
  function toast(opts: {
    title: string
    description?: string
    variant?: ToastVariant
    duration?: number
  }) {
    const id = ++counter
    toasts.value.push({
      id,
      title: opts.title,
      description: opts.description,
      variant: opts.variant ?? 'default',
    })
    const duration = opts.duration ?? 4000
    if (duration > 0) {
      setTimeout(() => dismiss(id), duration)
    }
    return id
  }

  function dismiss(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  return { toasts, toast, dismiss }
}
