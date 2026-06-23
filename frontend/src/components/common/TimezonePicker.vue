<script setup lang="ts">
import { computed } from 'vue'
import { Select } from '@/components/ui/select'
import { useTimezone, listTimezones } from '@/composables/useTimezone'

const { timezone, systemTimezone, setTimezone } = useTimezone()

const zones = listTimezones()

const model = computed({
  get: () => timezone.value,
  set: (v: string) => setTimezone(v),
})
</script>

<template>
  <Select
    v-model="model"
    class="h-8 w-auto text-xs"
    aria-label="Display timezone"
    title="Display timezone"
  >
    <option value="system">System ({{ systemTimezone }})</option>
    <option value="UTC">UTC</option>
    <option v-for="z in zones" :key="z" :value="z">{{ z }}</option>
  </Select>
</template>
