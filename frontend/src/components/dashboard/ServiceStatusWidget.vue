<script setup lang="ts">
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { StatusBadge } from '@/components/ui/badge'

defineProps<{ state?: string }>()
</script>

<template>
  <Card data-testid="service-status-widget">
    <CardHeader class="pb-2">
      <CardTitle class="text-sm text-muted-foreground">KumoMTA Service</CardTitle>
    </CardHeader>
    <CardContent>
      <div class="flex items-center justify-between">
        <span class="text-2xl font-semibold">{{ state ?? 'Unknown' }}</span>
        <StatusBadge :status="state" />
      </div>
    </CardContent>
  </Card>
</template>
