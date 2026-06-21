<script setup lang="ts">
import { ref, watch } from 'vue'
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description?: string
    /** When set, the user must type this exact value to enable confirmation. */
    confirmText?: string
    confirmLabel?: string
    variant?: 'default' | 'destructive'
    loading?: boolean
  }>(),
  {
    confirmLabel: 'Confirm',
    variant: 'destructive',
    loading: false,
  },
)

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const typed = ref('')

watch(
  () => props.open,
  (open) => {
    if (open) typed.value = ''
  },
)

function canConfirm() {
  if (props.loading) return false
  if (props.confirmText) return typed.value.trim() === props.confirmText
  return true
}

function onConfirm() {
  if (!canConfirm()) return
  emit('confirm')
}

function onCancel() {
  emit('cancel')
  emit('update:open', false)
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => emit('update:open', v)">
    <DialogHeader>
      <DialogTitle>{{ title }}</DialogTitle>
      <DialogDescription v-if="description">{{ description }}</DialogDescription>
    </DialogHeader>

    <div v-if="confirmText" class="space-y-2">
      <Label for="confirm-input">
        Type <span class="font-mono text-foreground">{{ confirmText }}</span> to confirm
      </Label>
      <Input
        id="confirm-input"
        v-model="typed"
        :placeholder="confirmText"
        autocomplete="off"
      />
    </div>

    <DialogFooter>
      <Button variant="outline" :disabled="loading" @click="onCancel">Cancel</Button>
      <Button :variant="variant" :disabled="!canConfirm()" @click="onConfirm">
        <span v-if="loading">Working…</span>
        <span v-else>{{ confirmLabel }}</span>
      </Button>
    </DialogFooter>
  </Dialog>
</template>
