<script setup lang="ts">
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'

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

function onPageSize(event: Event) {
  emit('pageSizeChange', Number((event.target as HTMLSelectElement).value))
}
</script>

<template>
  <div class="mt-3 flex items-center justify-between text-sm">
    <div class="flex items-center gap-2 text-muted-foreground">
      <Label for="page-size" class="text-xs">Rows</Label>
      <Select id="page-size" :model-value="String(pageSize)" class="h-8 w-20" @change="onPageSize">
        <option v-for="s in pageSizes" :key="s" :value="String(s)">{{ s }}</option>
      </Select>
      <span class="ml-2">Page {{ pageNumber }}</span>
    </div>
    <div class="flex items-center gap-2">
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
