<script setup lang="ts">
import { Button } from '@/components/ui/button'

withDefaults(
  defineProps<{
    pageNumber: number
    hasPrev: boolean
    hasNext: boolean
    loading?: boolean
    pageSize: number
    pageSizes?: number[]
  }>(),
  { pageSizes: () => [25, 50, 100, 200] },
)

const emit = defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
  (e: 'pageSizeChange', size: number): void
}>()
</script>

<template>
  <div class="d-flex align-center justify-space-between mt-3 text-body-2">
    <div class="d-flex align-center ga-2 text-medium-emphasis">
      <v-select
        :model-value="pageSize"
        :items="pageSizes"
        label="Rows"
        variant="outlined"
        density="compact"
        hide-details
        style="max-width: 110px"
        @update:model-value="emit('pageSizeChange', Number($event))"
      />
      <span class="ml-2">Page {{ pageNumber }}</span>
    </div>
    <div class="d-flex align-center ga-2">
      <Button
        variant="outline"
        size="sm"
        :disabled="!hasPrev || loading"
        data-testid="prev-page"
        @click="emit('prev')"
      >
        Previous
      </Button>
      <Button
        variant="outline"
        size="sm"
        :disabled="!hasNext || loading"
        data-testid="next-page"
        @click="emit('next')"
      >
        Next
      </Button>
    </div>
  </div>
</template>
