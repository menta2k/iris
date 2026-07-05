<script setup lang="ts">
import { computed } from 'vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// queued is an int64 count which proto-JSON serializes as a string.
const props = defineProps<{ queued?: string | number }>()
const formatted = computed(() => Number(props.queued ?? 0).toLocaleString())
</script>

<template>
  <Card data-testid="queue-health-widget">
    <CardHeader class="pb-2">
      <CardTitle class="text-body-2 text-medium-emphasis">Queued Messages</CardTitle>
    </CardHeader>
    <CardContent>
      <span class="text-h5 font-weight-bold tabular-nums">{{ formatted }}</span>
    </CardContent>
  </Card>
</template>
