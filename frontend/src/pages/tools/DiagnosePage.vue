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

    <Card class="mb-4" style="max-width: 672px">
      <CardHeader>
        <CardTitle>Diagnose a sender</CardTitle>
        <CardDescription>
          From address is required. Recipient and mailclass are optional and only refine the
          routing preview (mailclass/routing are header- and recipient-driven, not set by the
          sender alone).
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form class="d-flex flex-column ga-3" @submit.prevent="run">
          <div class="d-flex flex-column ga-1">
            <Label for="dg-from">From address</Label>
            <Input id="dg-from" v-model="fromEmail" placeholder="newsletter@example.com" />
          </div>
          <v-row dense>
            <v-col cols="12" sm="6">
              <div class="d-flex flex-column ga-1">
                <Label for="dg-rcpt">Recipient (optional)</Label>
                <Input id="dg-rcpt" v-model="recipient" placeholder="user@dest.com" />
              </div>
            </v-col>
            <v-col cols="12" sm="6">
              <div class="d-flex flex-column ga-1">
                <Label for="dg-class">Mailclass header (optional)</Label>
                <Input id="dg-class" v-model="mailclass" placeholder="bulk" />
              </div>
            </v-col>
          </v-row>
          <Button type="submit" :disabled="running || !fromEmail.trim()">
            {{ running ? 'Diagnosing…' : 'Diagnose' }}
          </Button>
        </form>
      </CardContent>
    </Card>

    <div v-if="result" class="d-flex flex-column ga-4" style="max-width: 672px">
      <Card>
        <CardHeader>
          <CardTitle>
            Sending readiness for <span class="font-mono">{{ result.domain }}</span>
          </CardTitle>
        </CardHeader>
        <CardContent class="d-flex flex-column ga-4">
          <div v-for="item in result.items ?? []" :key="item.name" class="rounded border pa-3">
            <div class="d-flex align-center justify-space-between">
              <span class="font-weight-medium">{{ item.name }}</span>
              <Badge :variant="badgeVariant(item.status)">{{ item.status }}</Badge>
            </div>
            <p class="mt-1 text-body-2 text-medium-emphasis">{{ item.detail }}</p>
            <ul v-if="item.records?.length" class="mt-2 d-flex flex-column ga-1">
              <li
                v-for="r in item.records"
                :key="r"
                class="text-break font-mono text-caption text-medium-emphasis"
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
        <CardContent class="d-flex flex-column ga-2 text-body-2">
          <div>
            <span class="text-medium-emphasis">Matched rule:</span>
            <span class="font-weight-medium">{{ result.routing?.matchedRule || '— (default)' }}</span>
          </div>
          <div>
            <span class="text-medium-emphasis">Egress pool:</span>
            <span class="font-mono">{{ result.routing?.egressPool || '—' }}</span>
          </div>
          <div>
            <span class="text-medium-emphasis">VMTAs:</span>
            <span class="font-mono">{{ (result.routing?.vmtas ?? []).join(', ') || '—' }}</span>
          </div>
          <div>
            <span class="text-medium-emphasis">Egress IPs:</span>
            <span class="font-mono">{{ (result.routing?.egressIps ?? []).join(', ') || '—' }}</span>
          </div>
          <div>
            <span class="text-medium-emphasis">Listeners:</span>
            <span class="font-mono">{{ (result.routing?.listeners ?? []).join(', ') || '—' }}</span>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
