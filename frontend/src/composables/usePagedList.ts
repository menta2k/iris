import { computed, onMounted, ref, shallowRef } from 'vue'
import { ApiError } from '@/services/http'
import type { ListResponse } from '@/types'
import type { PageParams } from '@/services/pagination'

interface PagedListOptions<T> {
  // loader receives the page params and returns one page of results.
  loader: (page: PageParams) => Promise<ListResponse<T>>
  pageSize?: number
  immediate?: boolean
}

/**
 * Loads a paginated list endpoint using the API's opaque offset tokens. Keeps a
 * stack of the tokens used for prior pages so "Previous" can walk back, and
 * tolerates empty `{page}` responses and "not implemented" (501/503) states.
 */
export function usePagedList<T>({ loader, pageSize = 50, immediate = true }: PagedListOptions<T>) {
  const items = shallowRef<T[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const notImplemented = ref(false)

  const size = ref(pageSize)
  const prevTokens = ref<string[]>([])
  const currentToken = ref('')
  const nextToken = ref('')

  const pageNumber = computed(() => prevTokens.value.length + 1)
  const hasPrev = computed(() => prevTokens.value.length > 0)
  const hasNext = computed(() => nextToken.value !== '')

  async function load() {
    loading.value = true
    error.value = null
    notImplemented.value = false
    try {
      const res = await loader({
        pageSize: size.value,
        pageToken: currentToken.value || undefined,
      })
      items.value = res?.items ?? []
      nextToken.value = res?.page?.nextPageToken ?? res?.page?.next_page_token ?? ''
    } catch (err) {
      items.value = []
      nextToken.value = ''
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

  // Restart at the first page (after a filter or page-size change).
  function reload() {
    prevTokens.value = []
    currentToken.value = ''
    return load()
  }

  function nextPage() {
    if (!hasNext.value) return
    prevTokens.value.push(currentToken.value)
    currentToken.value = nextToken.value
    load()
  }

  function prevPage() {
    if (!hasPrev.value) return
    currentToken.value = prevTokens.value.pop() ?? ''
    load()
  }

  function setPageSize(n: number) {
    if (n > 0 && n !== size.value) {
      size.value = n
      reload()
    }
  }

  if (immediate) onMounted(load)

  return {
    items,
    loading,
    error,
    notImplemented,
    pageSize: size,
    pageNumber,
    hasPrev,
    hasNext,
    load,
    reload,
    nextPage,
    prevPage,
    setPageSize,
  }
}
