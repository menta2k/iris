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
  <v-alert
    v-if="drift"
    type="warning"
    variant="tonal"
    density="compact"
    rounded="0"
    data-testid="config-drift-banner"
  >
    <div class="d-flex flex-wrap align-center justify-space-between ga-3">
      <span class="text-body-2">
        <span class="font-weight-medium">Configuration changes are pending.</span>
        <template v-if="restartRequired">
          These changes affect KumoMTA's init block (listeners / spool / log hook) and
          require a <span class="font-weight-medium">restart</span>, not just a reload.
        </template>
        <template v-else> Regenerate and apply the KumoMTA config to activate them. </template>
      </span>
      <v-btn
        :to="{ name: 'kumomta-config' }"
        size="small"
        variant="outlined"
        color="warning"
        class="flex-shrink-0"
      >
        Review &amp; apply
      </v-btn>
    </div>
  </v-alert>
</template>
