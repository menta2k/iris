<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useToast } from '@/composables/useToast'
import { toolsService } from '@/services'
import { ApiError } from '@/services/http'
import type { RblCheckReply, RblIpResult } from '@/types'

const { toast } = useToast()

const running = ref(false)
const report = ref<RblCheckReply | null>(null)

function listingFor(ip: RblIpResult, zone: string) {
  return (ip.listings ?? []).find((l) => l.zone === zone)
}

async function run() {
  running.value = true
  report.value = null
  try {
    report.value = await toolsService.rblCheck()
  } catch (err) {
    toast({
      title: 'RBL check failed',
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
      title="RBL Check"
      description="Check this deployment's listener and VMTA egress IPs against DNS blocklists (DNSBLs). Results are only reliable from a non-public DNS resolver."
    >
      <template #actions>
        <Button :disabled="running" data-testid="run-rbl" @click="run">
          {{ running ? 'Checking…' : 'Run check' }}
        </Button>
      </template>
    </PageHeader>

    <Card v-if="report">
      <CardHeader>
        <CardTitle>Results</CardTitle>
        <CardDescription>
          Checked {{ report.results?.length ?? 0 }} IP(s) against {{ (report.zones ?? []).length }}
          blocklist(s) at {{ report.checkedAt }}.
          <span v-if="report.skipped?.length">Skipped (non-IPv4): {{ report.skipped.join(', ') }}.</span>
        </CardDescription>
      </CardHeader>
      <CardContent class="pa-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>IP</TableHead>
              <TableHead>Source</TableHead>
              <TableHead>Overall</TableHead>
              <TableHead v-for="z in report.zones ?? []" :key="z" class="text-caption">{{ z }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="ip in report.results ?? []" :key="ip.ip">
              <TableCell class="font-mono">{{ ip.ip }}</TableCell>
              <TableCell class="text-medium-emphasis">{{ ip.source }}</TableCell>
              <TableCell>
                <Badge :variant="ip.listed ? 'destructive' : 'success'">
                  {{ ip.listed ? 'Listed' : 'Clean' }}
                </Badge>
              </TableCell>
              <TableCell v-for="z in report.zones ?? []" :key="z">
                <Badge
                  :variant="listingFor(ip, z)?.listed ? 'destructive' : 'secondary'"
                  :title="listingFor(ip, z)?.reason || ''"
                >
                  {{ listingFor(ip, z)?.listed ? 'listed' : 'ok' }}
                </Badge>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>

    <p v-else class="text-body-2 text-medium-emphasis">
      Click "Run check" to test all configured IPs against the blocklists.
    </p>
  </div>
</template>
