import { onMounted, ref, shallowRef } from 'vue'
import { ApiError } from '@/services/http'
import type { ListResponse } from '@/types'

interface AsyncListState<T> {
  loader: () => Promise<ListResponse<T>>
}

/**
 * Loads a list endpoint, tolerating empty `{page}` responses and
 * "not implemented yet" (501/503) backend states without crashing.
 */
export function useAsyncList<T>({ loader }: AsyncListState<T>) {
  const items = shallowRef<T[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const notImplemented = ref(false)

  async function load() {
    loading.value = true
    error.value = null
    notImplemented.value = false
    try {
      const res = await loader()
      items.value = res?.items ?? []
    } catch (err) {
      items.value = []
      if (err instanceof ApiError && err.notImplemented) {
        notImplemented.value = true
      } else if (err instanceof ApiError && err.status === 0) {
        error.value = 'Cannot reach the backend. Is the API server running?'
      } else {
        error.value = err instanceof Error ? err.message : 'Failed to load data.'
      }
    } finally {
      loading.value = false
    }
  }

  onMounted(load)

  return { items, loading, error, notImplemented, load }
}
