<script setup lang="ts">
defineProps<{ colspan: number; message?: string }>()
</script>

<template>
  <tr>
    <td :colspan="colspan" class="px-3 py-10 text-center text-sm text-muted-foreground">
      <slot>{{ message ?? 'No records found.' }}</slot>
    </td>
  </tr>
</template>
