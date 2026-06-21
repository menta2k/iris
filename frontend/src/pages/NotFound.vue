<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { Button } from '@/components/ui/button'
</script>

<template>
  <div class="flex flex-col items-center justify-center py-24 text-center">
    <p class="text-5xl font-bold text-muted-foreground">404</p>
    <p class="mt-2 text-lg">Page not found</p>
    <RouterLink to="/" class="mt-6">
      <Button variant="outline">Back to Dashboard</Button>
    </RouterLink>
  </div>
</template>
