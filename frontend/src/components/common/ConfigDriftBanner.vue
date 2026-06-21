<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { kumoConfigService } from '@/services'

// Shows a reminder when the live configuration has changed but the KumoMTA
// policy has not been regenerated/applied yet. It re-checks on navigation and
// window focus (so it updates right after a change), plus a slow poll.
const drift = ref(false)
const restartRequired = ref(false)
const route = useRoute()
let timer: number | undefined

async function check() {
  try {
    const s = await kumoConfigService.status()
    drift.value = s.drift
    restartRequired.value = s.restartRequired
  } catch {
    // Insufficient permissions or backend unavailable: no banner.
    drift.value = false
    restartRequired.value = false
  }
}

onMounted(() => {
  check()
  timer = window.setInterval(check, 20000)
  window.addEventListener('focus', check)
})
onUnmounted(() => {
  if (timer) window.clearInterval(timer)
  window.removeEventListener('focus', check)
})
watch(() => route.fullPath, check)
</script>

<template>
  <div
    v-if="drift"
    class="flex items-center justify-between gap-3 border-b border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200"
    data-testid="config-drift-banner"
  >
    <span>
      <span class="font-medium">Configuration changes are pending.</span>
      <template v-if="restartRequired">
        These changes affect KumoMTA's init block (listeners / spool / log hook) and
        require a <span class="font-medium">restart</span>, not just a reload.
      </template>
      <template v-else> Regenerate and apply the KumoMTA config to activate them. </template>
    </span>
    <RouterLink
      :to="{ name: 'kumomta-config' }"
      class="shrink-0 rounded-md border border-amber-400 px-3 py-1 text-xs font-medium hover:bg-amber-100 dark:hover:bg-amber-900"
    >
      Review &amp; apply
    </RouterLink>
  </div>
</template>
