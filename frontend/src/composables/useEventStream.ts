import { ref } from 'vue'
import { getToken } from '@/services/token'

// useEventStream subscribes to a backend SSE stream (mounted at /sse on the
// admin origin). EventSource cannot send an Authorization header, so the
// session token is passed as the `token` query parameter (validated server-side,
// requires mail:read). The browser reconnects automatically on transient errors.
export function useEventStream<T = unknown>(stream: string, onMessage: (data: T) => void) {
  const connected = ref(false)
  let es: EventSource | null = null

  function start() {
    if (es) return
    const token = getToken() ?? ''
    const url = `/sse?stream=${encodeURIComponent(stream)}&token=${encodeURIComponent(token)}`
    es = new EventSource(url)
    es.onopen = () => {
      connected.value = true
    }
    es.onerror = () => {
      // EventSource retries on its own; reflect the transient disconnect.
      connected.value = false
    }
    es.onmessage = (ev) => {
      if (!ev.data) return
      try {
        onMessage(JSON.parse(ev.data) as T)
      } catch {
        // Ignore malformed frames.
      }
    }
  }

  function stop() {
    if (es) {
      es.close()
      es = null
    }
    connected.value = false
  }

  return { connected, start, stop }
}
