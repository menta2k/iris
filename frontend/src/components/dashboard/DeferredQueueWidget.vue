<script setup lang="ts">
import { computed } from 'vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// deferred is an int64 count which proto-JSON serializes as a string. These are
// messages still in the queue after a transient failure — retrying, not bounced.
const props = defineProps<{ deferred?: string | number }>()
const count = computed(() => Number(props.deferred ?? 0))
const formatted = computed(() => count.value.toLocaleString())
</script>

<template>
  <Card data-testid="deferred-queue-widget">
    <CardHeader class="pb-2">
      <CardTitle class="text-body-2 text-medium-emphasis">Deferred (in queue)</CardTitle>
    </CardHeader>
    <CardContent>
      <span
        class="text-h5 font-weight-bold tabular-nums"
        :class="count > 0 ? 'text-warning' : ''"
      >{{ formatted }}</span>
    </CardContent>
  </Card>
</template>
