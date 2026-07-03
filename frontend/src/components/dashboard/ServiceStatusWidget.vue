<script setup lang="ts">
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { StatusBadge } from '@/components/ui/badge'

defineProps<{ state?: string }>()
</script>

<template>
  <Card data-testid="service-status-widget">
    <CardHeader class="pb-2">
      <CardTitle class="text-body-2 text-medium-emphasis">KumoMTA Service</CardTitle>
    </CardHeader>
    <CardContent>
      <div class="d-flex align-center justify-space-between">
        <span class="text-h5 font-weight-bold">{{ state ?? 'Unknown' }}</span>
        <StatusBadge :status="state" />
      </div>
    </CardContent>
  </Card>
</template>
