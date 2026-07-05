<script setup lang="ts">
import { computed } from 'vue'
import { useTimezone, listTimezones } from '@/composables/useTimezone'

const { timezone, systemTimezone, setTimezone } = useTimezone()

const zones = listTimezones()

const items = [
  { title: `System (${systemTimezone})`, value: 'system' },
  { title: 'UTC', value: 'UTC' },
  ...zones.map((z) => ({ title: z, value: z })),
]

const model = computed({
  get: () => timezone.value,
  set: (v: string) => setTimezone(v),
})
</script>

<template>
  <v-select
    v-model="model"
    :items="items"
    variant="outlined"
    density="compact"
    hide-details
    style="max-width: 240px"
    aria-label="Display timezone"
    title="Display timezone"
  />
</template>
