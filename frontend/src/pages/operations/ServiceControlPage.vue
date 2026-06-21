<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import ConfirmDialog from '@/components/confirm/ConfirmDialog.vue'
import { useToast } from '@/composables/useToast'
import { mailOperationsService } from '@/services'
import { ApiError, newConfirmationId } from '@/services/http'
import type { ServiceOperation } from '@/types'

const { toast } = useToast()

const confirmOpen = ref(false)
const acting = ref(false)
const pending = ref<ServiceOperation | null>(null)

// op values are the backend enum (lowercase); label is for display.
const operations: { op: ServiceOperation; label: string; description: string; destructive: boolean }[] = [
  {
    op: 'reload',
    label: 'Reload Config',
    description: 'Hot-reload configuration without dropping connections.',
    destructive: false,
  },
  {
    op: 'restart',
    label: 'Restart Service',
    description: 'Fully restart KumoMTA. In-flight connections will be interrupted.',
    destructive: true,
  },
]

// Capitalized operation for display (the backend value stays lowercase).
const pendingLabel = computed(() =>
  pending.value ? pending.value.charAt(0).toUpperCase() + pending.value.slice(1) : '',
)

function request(op: ServiceOperation) {
  pending.value = op
  confirmOpen.value = true
}

async function confirm() {
  if (!pending.value) return
  acting.value = true
  try {
    const res = await mailOperationsService.serviceControl({
      operation: pending.value,
      confirmation_id: newConfirmationId(),
    })
    toast({
      title: `${pendingLabel.value} requested`,
      description: `Operation ${res.id} — ${res.status}`,
      variant: 'success',
    })
    confirmOpen.value = false
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Service control failed.'
    toast({ title: 'Service control failed', description: msg, variant: 'destructive' })
  } finally {
    acting.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Service Control" description="Reload or restart the KumoMTA service." />

    <div class="grid gap-4 md:grid-cols-2">
      <Card v-for="o in operations" :key="o.op">
        <CardHeader>
          <CardTitle>{{ o.label }}</CardTitle>
          <CardDescription>{{ o.description }}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            :variant="o.destructive ? 'destructive' : 'default'"
            :data-testid="`svc-${o.op.toLowerCase()}`"
            @click="request(o.op)"
          >
            {{ o.label }}
          </Button>
        </CardContent>
      </Card>
    </div>

    <ConfirmDialog
      v-model:open="confirmOpen"
      :title="pending ? `${pendingLabel} KumoMTA` : 'Confirm'"
      :description="
        pending === 'restart'
          ? 'Restarting will interrupt all in-flight SMTP connections. Type RESTART to confirm.'
          : 'Reload the KumoMTA configuration now?'
      "
      :confirm-label="pending === 'restart' ? 'Restart' : 'Reload'"
      :confirm-text="pending === 'restart' ? 'RESTART' : undefined"
      :variant="pending === 'restart' ? 'destructive' : 'default'"
      :loading="acting"
      @confirm="confirm"
    />
  </div>
</template>
