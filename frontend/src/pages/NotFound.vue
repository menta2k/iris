<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { Button } from '@/components/ui/button'
</script>

<template>
  <div class="d-flex flex-column align-center justify-center text-center" style="padding-top: 96px; padding-bottom: 96px">
    <p class="text-h2 font-weight-bold text-medium-emphasis">404</p>
    <p class="mt-2 text-h6">Page not found</p>
    <RouterLink to="/" class="mt-6">
      <Button variant="outline">Back to Dashboard</Button>
    </RouterLink>
  </div>
</template>
