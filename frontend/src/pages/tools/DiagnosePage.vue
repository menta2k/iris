<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useToast } from '@/composables/useToast'
import { toolsService } from '@/services'
import { ApiError } from '@/services/http'
import type { DiagnoseResult } from '@/types'

const { toast } = useToast()

const fromEmail = ref('')
const recipient = ref('')
const mailclass = ref('')
const running = ref(false)
const result = ref<DiagnoseResult | null>(null)

function badgeVariant(status: string): 'success' | 'warning' | 'destructive' | 'secondary' {
  if (status === 'pass') return 'success'
  if (status === 'warn') return 'warning'
  if (status === 'fail') return 'destructive'
  return 'secondary'
}

async function run() {
  const from = fromEmail.value.trim()
  if (!from) return
  running.value = true
  result.value = null
  try {
    result.value = await toolsService.diagnose({
      from_email: from,
      recipient: recipient.value.trim() || undefined,
      mailclass: mailclass.value.trim() || undefined,
    })
  } catch (err) {
    toast({
      title: 'Diagnose failed',
      description: err instanceof ApiError ? err.message : 'Unexpected error.',
      variant: 'destructive',
    })
  } finally {
    running.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Diagnose"
      description="See how mail from a given address would be handled by this deployment, and whether the sending domain is set up correctly (DKIM, SPF, DMARC, MX, feedback loop) and which egress route it would take."
    />

    <Card class="mb-4 max-w-2xl">
      <CardHeader>
        <CardTitle>Diagnose a sender</CardTitle>
        <CardDescription>
          From address is required. Recipient and mailclass are optional and only refine the
          routing preview (mailclass/routing are header- and recipient-driven, not set by the
          sender alone).
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form class="space-y-3" @submit.prevent="run">
          <div class="space-y-1.5">
            <Label for="dg-from">From address</Label>
            <Input id="dg-from" v-model="fromEmail" placeholder="newsletter@example.com" />
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div class="space-y-1.5">
              <Label for="dg-rcpt">Recipient (optional)</Label>
              <Input id="dg-rcpt" v-model="recipient" placeholder="user@dest.com" />
            </div>
            <div class="space-y-1.5">
              <Label for="dg-class">Mailclass header (optional)</Label>
              <Input id="dg-class" v-model="mailclass" placeholder="bulk" />
            </div>
          </div>
          <Button type="submit" :disabled="running || !fromEmail.trim()">
            {{ running ? 'Diagnosing…' : 'Diagnose' }}
          </Button>
        </form>
      </CardContent>
    </Card>

    <div v-if="result" class="grid max-w-2xl gap-4">
      <Card>
        <CardHeader>
          <CardTitle>
            Sending readiness for <span class="font-mono">{{ result.domain }}</span>
          </CardTitle>
        </CardHeader>
        <CardContent class="space-y-4">
          <div v-for="item in result.items ?? []" :key="item.name" class="rounded-md border p-3">
            <div class="flex items-center justify-between">
              <span class="font-medium">{{ item.name }}</span>
              <Badge :variant="badgeVariant(item.status)">{{ item.status }}</Badge>
            </div>
            <p class="mt-1 text-sm text-muted-foreground">{{ item.detail }}</p>
            <ul v-if="item.records?.length" class="mt-2 space-y-0.5">
              <li
                v-for="r in item.records"
                :key="r"
                class="break-all font-mono text-xs text-muted-foreground"
              >
                {{ r }}
              </li>
            </ul>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Routing preview</CardTitle>
          <CardDescription v-if="result.routing?.note">{{ result.routing.note }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-2 text-sm">
          <div>
            <span class="text-muted-foreground">Matched rule:</span>
            <span class="font-medium">{{ result.routing?.matchedRule || '— (default)' }}</span>
          </div>
          <div>
            <span class="text-muted-foreground">Egress pool:</span>
            <span class="font-mono">{{ result.routing?.egressPool || '—' }}</span>
          </div>
          <div>
            <span class="text-muted-foreground">VMTAs:</span>
            <span class="font-mono">{{ (result.routing?.vmtas ?? []).join(', ') || '—' }}</span>
          </div>
          <div>
            <span class="text-muted-foreground">Egress IPs:</span>
            <span class="font-mono">{{ (result.routing?.egressIps ?? []).join(', ') || '—' }}</span>
          </div>
          <div>
            <span class="text-muted-foreground">Listeners:</span>
            <span class="font-mono">{{ (result.routing?.listeners ?? []).join(', ') || '—' }}</span>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
