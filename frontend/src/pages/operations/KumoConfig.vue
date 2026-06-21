<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import ConfirmDialog from '@/components/confirm/ConfirmDialog.vue'
import { useToast } from '@/composables/useToast'
import { kumoConfigService } from '@/services'
import { ApiError, newConfirmationId } from '@/services/http'
import type { KumoConfigPreview } from '@/types'

const { toast } = useToast()

const preview = ref<KumoConfigPreview | null>(null)
const generating = ref(false)
const previewError = ref<string | null>(null)

const confirmOpen = ref(false)
const applying = ref(false)

async function generate() {
  generating.value = true
  previewError.value = null
  try {
    preview.value = await kumoConfigService.generate()
  } catch (err) {
    preview.value = null
    if (err instanceof ApiError && err.status === 0) {
      previewError.value = 'Cannot reach the backend. Is the API server running?'
    } else {
      previewError.value = err instanceof ApiError ? err.message : 'Failed to generate config.'
    }
  } finally {
    generating.value = false
  }
}

function requestApply() {
  confirmOpen.value = true
}

async function confirmApply() {
  applying.value = true
  try {
    const res = await kumoConfigService.apply({ confirmation_id: newConfirmationId() })
    toast({
      title: `Config applied — ${res.status}`,
      description: res.resultSummary || `Written to ${res.appliedPath}`,
      variant: 'success',
    })
    confirmOpen.value = false
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to apply config.'
    toast({ title: 'Apply failed', description: msg, variant: 'destructive' })
  } finally {
    applying.value = false
  }
}

const shortChecksum = (checksum: string) => (checksum ? checksum.slice(0, 12) : '—')
</script>

<template>
  <div>
    <PageHeader
      title="KumoMTA Configuration"
      description="Generate the KumoMTA Lua policy from current Iris config, preview it, then apply it to the live service."
    >
      <template #actions>
        <Button variant="outline" data-testid="generate-config" :disabled="generating" @click="generate">
          {{ generating ? 'Generating…' : 'Generate / Preview' }}
        </Button>
        <Button
          variant="destructive"
          data-testid="apply-config"
          :disabled="preview ? preview.valid === false : false"
          @click="requestApply"
        >
          Apply to KumoMTA
        </Button>
      </template>
    </PageHeader>

    <p class="mb-4 text-sm text-muted-foreground">
      Applying writes the actual KumoMTA configuration file and reloads the service. This affects
      live mail delivery — preview the generated policy first.
    </p>

    <div
      v-if="previewError"
      data-testid="config-error"
      class="mb-4 rounded-md border border-destructive/40 bg-destructive/5 px-4 py-6 text-center text-sm text-destructive"
    >
      {{ previewError }}
    </div>

    <div
      v-else-if="!preview"
      class="mb-4 rounded-md border border-dashed px-4 py-10 text-center text-sm text-muted-foreground"
    >
      Click “Generate / Preview” to render the current KumoMTA policy.
    </div>

    <template v-else>
      <div
        v-if="preview.valid === false"
        data-testid="config-lint-error"
        class="mb-4 rounded-md border border-destructive/40 bg-destructive/5 px-4 py-3 text-sm text-destructive"
      >
        <p class="font-medium">Generated policy failed Lua validation — apply is disabled.</p>
        <ul class="mt-1 list-inside list-disc font-mono text-xs">
          <li v-for="(issue, i) in preview.lintIssues ?? []" :key="i">{{ issue }}</li>
        </ul>
      </div>
      <div
        v-else-if="preview.valid"
        data-testid="config-lint-ok"
        class="mb-4 rounded-md border border-emerald-500/40 bg-emerald-500/5 px-4 py-2 text-sm text-emerald-700 dark:text-emerald-400"
      >
        Policy is valid Lua and ready to apply.
      </div>

      <div class="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <Card>
          <CardContent class="p-4">
            <div class="text-xs text-muted-foreground">VMTAs</div>
            <div class="text-2xl font-semibold tabular-nums">{{ preview.vmtaCount }}</div>
          </CardContent>
        </Card>
        <Card>
          <CardContent class="p-4">
            <div class="text-xs text-muted-foreground">Pools</div>
            <div class="text-2xl font-semibold tabular-nums">{{ preview.poolCount }}</div>
          </CardContent>
        </Card>
        <Card>
          <CardContent class="p-4">
            <div class="text-xs text-muted-foreground">Routes</div>
            <div class="text-2xl font-semibold tabular-nums">{{ preview.routeCount }}</div>
          </CardContent>
        </Card>
        <Card>
          <CardContent class="p-4">
            <div class="text-xs text-muted-foreground">DKIM</div>
            <div class="text-2xl font-semibold tabular-nums">{{ preview.dkimCount }}</div>
          </CardContent>
        </Card>
        <Card>
          <CardContent class="p-4">
            <div class="text-xs text-muted-foreground">Suppressions</div>
            <div class="text-2xl font-semibold tabular-nums">{{ preview.suppressionCount }}</div>
          </CardContent>
        </Card>
        <Card>
          <CardContent class="p-4">
            <div class="text-xs text-muted-foreground">Checksum</div>
            <div class="mt-1">
              <Badge variant="secondary" class="font-mono">{{ shortChecksum(preview.checksum) }}</Badge>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Generated KumoMTA Policy</CardTitle>
          <CardDescription>Lua policy that will be written when you apply.</CardDescription>
        </CardHeader>
        <CardContent>
          <pre
            data-testid="config-content"
            class="max-h-[28rem] overflow-auto rounded-md border bg-muted/40 p-4 font-mono text-xs leading-relaxed"
          ><code>{{ preview.content }}</code></pre>
        </CardContent>
      </Card>
    </template>

    <ConfirmDialog
      v-model:open="confirmOpen"
      title="Apply config to KumoMTA"
      description="This writes the generated configuration to KumoMTA and reloads the service, affecting live delivery. Type APPLY to confirm."
      confirm-label="Apply"
      confirm-text="APPLY"
      variant="destructive"
      :loading="applying"
      @confirm="confirmApply"
    />
  </div>
</template>
