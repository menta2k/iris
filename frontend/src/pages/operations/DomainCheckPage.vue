<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useToast } from '@/composables/useToast'
import { domainCheckService } from '@/services'
import { ApiError } from '@/services/http'
import type { DomainBounceCheck } from '@/types'

const { toast } = useToast()

const domain = ref('')
const running = ref(false)
const result = ref<DomainBounceCheck | null>(null)

function badgeVariant(status: string): 'success' | 'warning' | 'destructive' | 'secondary' {
  if (status === 'pass') return 'success'
  if (status === 'warn') return 'warning'
  if (status === 'fail') return 'destructive'
  return 'secondary'
}

async function run() {
  const d = domain.value.trim()
  if (!d) return
  running.value = true
  result.value = null
  try {
    result.value = await domainCheckService.check(d)
  } catch (err) {
    toast({
      title: 'Check failed',
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
      title="Domain Bounce Readiness"
      description="Verify a domain's DNS is set up to send mail and accept bounces for this deployment: MX (points here), SPF (authorizes our IPs), and DKIM (selector published)."
    />

    <Card class="mb-4 max-w-2xl">
      <CardHeader>
        <CardTitle>Check a domain</CardTitle>
        <CardDescription>Runs live DNS lookups against the domain you enter.</CardDescription>
      </CardHeader>
      <CardContent>
        <form class="flex items-end gap-3" @submit.prevent="run">
          <div class="flex-1 space-y-1.5">
            <Label for="dc-domain">Domain</Label>
            <Input id="dc-domain" v-model="domain" placeholder="bounce.kmx.jobs.bg" />
          </div>
          <Button type="submit" :disabled="running || !domain.trim()">
            {{ running ? 'Checking…' : 'Check' }}
          </Button>
        </form>
      </CardContent>
    </Card>

    <Card v-if="result" class="max-w-2xl">
      <CardHeader>
        <CardTitle>Results for <span class="font-mono">{{ result.domain }}</span></CardTitle>
      </CardHeader>
      <CardContent class="space-y-4">
        <div v-for="item in result.items ?? []" :key="item.name" class="rounded-md border p-3">
          <div class="flex items-center justify-between">
            <span class="font-medium">{{ item.name }}</span>
            <Badge :variant="badgeVariant(item.status)">{{ item.status }}</Badge>
          </div>
          <p class="mt-1 text-sm text-muted-foreground">{{ item.detail }}</p>
          <ul v-if="item.records?.length" class="mt-2 space-y-0.5">
            <li v-for="r in item.records" :key="r" class="break-all font-mono text-xs text-muted-foreground">
              {{ r }}
            </li>
          </ul>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
