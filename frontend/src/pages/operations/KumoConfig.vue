<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import ConfirmDialog from '@/components/confirm/ConfirmDialog.vue'
import { useToast } from '@/composables/useToast'
import { diffLines } from '@/composables/lineDiff'
import { formatDateTime } from '@/composables/useTimezone'
import { kumoConfigService } from '@/services'
import { ApiError, newConfirmationId } from '@/services/http'
import type { AppliedKumoConfig, KumoConfigPreview } from '@/types'

const { toast } = useToast()

const preview = ref<KumoConfigPreview | null>(null)
const generating = ref(false)
const previewError = ref<string | null>(null)

const confirmOpen = ref(false)
const applying = ref(false)

// Preview (pending policy) vs Diff (pending compared to the running policy).
const viewMode = ref<'preview' | 'diff'>('preview')
const applied = ref<AppliedKumoConfig | null>(null)
const loadingApplied = ref(false)
const appliedError = ref<string | null>(null)

// Diff of running (old) → pending (new). Empty until both sides are loaded.
const diff = computed(() =>
  preview.value && applied.value
    ? diffLines(applied.value.content, preview.value.content)
    : null,
)
const identical = computed(() => diff.value !== null && diff.value.added === 0 && diff.value.removed === 0)

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

// Load the running policy (once) so it can be diffed against the pending one.
async function loadApplied() {
  if (applied.value || loadingApplied.value) return
  loadingApplied.value = true
  appliedError.value = null
  try {
    applied.value = await kumoConfigService.applied()
  } catch (err) {
    appliedError.value = err instanceof ApiError ? err.message : 'Failed to load running config.'
  } finally {
    loadingApplied.value = false
  }
}

function setView(mode: 'preview' | 'diff') {
  viewMode.value = mode
  if (mode === 'diff') loadApplied()
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
    // The running policy just changed — drop the cache so the diff refreshes.
    applied.value = null
    if (viewMode.value === 'diff') loadApplied()
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

    <p class="mb-4 text-body-2 text-medium-emphasis">
      Applying writes the actual KumoMTA configuration file and reloads the service. This affects
      live mail delivery — preview the generated policy first.
    </p>

    <v-alert
      v-if="previewError"
      data-testid="config-error"
      type="error"
      variant="tonal"
      density="comfortable"
      class="mb-4 text-center text-body-2"
    >
      {{ previewError }}
    </v-alert>

    <div
      v-else-if="!preview"
      class="mb-4 rounded border border-dashed px-4 py-10 text-center text-body-2 text-medium-emphasis"
    >
      Click “Generate / Preview” to render the current KumoMTA policy.
    </div>

    <template v-else>
      <v-alert
        v-if="preview.valid === false"
        data-testid="config-lint-error"
        type="error"
        variant="tonal"
        density="comfortable"
        class="mb-4 text-body-2"
      >
        <p class="font-weight-medium">Generated policy failed Lua validation — apply is disabled.</p>
        <ul class="mt-1 font-mono text-caption" style="list-style: disc inside">
          <li v-for="(issue, i) in preview.lintIssues ?? []" :key="i">{{ issue }}</li>
        </ul>
      </v-alert>
      <v-alert
        v-else-if="preview.valid"
        data-testid="config-lint-ok"
        type="success"
        variant="tonal"
        density="comfortable"
        class="mb-4 text-body-2"
      >
        Policy is valid Lua and ready to apply.
      </v-alert>

      <v-row dense class="mb-4">
        <v-col cols="6" sm="4" lg="2">
          <Card>
            <CardContent class="pa-4">
              <div class="text-caption text-medium-emphasis">VMTAs</div>
              <div class="text-h5 font-weight-bold tabular-nums">{{ preview.vmtaCount }}</div>
            </CardContent>
          </Card>
        </v-col>
        <v-col cols="6" sm="4" lg="2">
          <Card>
            <CardContent class="pa-4">
              <div class="text-caption text-medium-emphasis">Pools</div>
              <div class="text-h5 font-weight-bold tabular-nums">{{ preview.poolCount }}</div>
            </CardContent>
          </Card>
        </v-col>
        <v-col cols="6" sm="4" lg="2">
          <Card>
            <CardContent class="pa-4">
              <div class="text-caption text-medium-emphasis">Routes</div>
              <div class="text-h5 font-weight-bold tabular-nums">{{ preview.routeCount }}</div>
            </CardContent>
          </Card>
        </v-col>
        <v-col cols="6" sm="4" lg="2">
          <Card>
            <CardContent class="pa-4">
              <div class="text-caption text-medium-emphasis">DKIM</div>
              <div class="text-h5 font-weight-bold tabular-nums">{{ preview.dkimCount }}</div>
            </CardContent>
          </Card>
        </v-col>
        <v-col cols="6" sm="4" lg="2">
          <Card>
            <CardContent class="pa-4">
              <div class="text-caption text-medium-emphasis">Suppressions</div>
              <div class="text-h5 font-weight-bold tabular-nums">{{ preview.suppressionCount }}</div>
            </CardContent>
          </Card>
        </v-col>
        <v-col cols="6" sm="4" lg="2">
          <Card>
            <CardContent class="pa-4">
              <div class="text-caption text-medium-emphasis">Checksum</div>
              <div class="mt-1">
                <Badge variant="secondary" class="font-mono">{{ shortChecksum(preview.checksum) }}</Badge>
              </div>
            </CardContent>
          </Card>
        </v-col>
      </v-row>

      <Card>
        <CardHeader class="d-flex flex-row align-start justify-space-between ga-4">
          <div>
            <CardTitle>Generated KumoMTA Policy</CardTitle>
            <CardDescription>
              {{
                viewMode === 'diff'
                  ? 'Changes between the running policy and the pending one.'
                  : 'Lua policy that will be written when you apply.'
              }}
            </CardDescription>
          </div>
          <v-btn-toggle
            v-model="viewMode"
            density="compact"
            variant="outlined"
            divided
            mandatory
            @update:model-value="setView($event)"
          >
            <v-btn value="preview" size="small">Preview</v-btn>
            <v-btn value="diff" size="small" data-testid="config-diff-toggle">Diff vs running</v-btn>
          </v-btn-toggle>
        </CardHeader>
        <CardContent>
          <!-- Preview: the full pending policy. -->
          <pre
            v-if="viewMode === 'preview'"
            data-testid="config-content"
            class="overflow-auto rounded border bg-surface-light pa-4 font-mono text-caption"
            style="max-height: 28rem"
          ><code>{{ preview.content }}</code></pre>

          <!-- Diff: running (old) vs pending (new). -->
          <template v-else>
            <p v-if="loadingApplied" class="py-6 text-center text-body-2 text-medium-emphasis">
              Loading running config…
            </p>
            <v-alert
              v-else-if="appliedError"
              type="error"
              variant="tonal"
              density="comfortable"
              class="text-body-2"
            >
              {{ appliedError }}
            </v-alert>
            <v-alert
              v-else-if="applied?.neverApplied"
              type="info"
              variant="tonal"
              density="comfortable"
              class="text-body-2"
            >
              No policy has been applied yet — there is no running config to diff against. Apply
              this policy to make it the running configuration.
            </v-alert>
            <v-alert
              v-else-if="identical"
              type="success"
              variant="tonal"
              density="comfortable"
              data-testid="config-diff-identical"
              class="text-body-2"
            >
              No differences — the pending policy matches the running one.
            </v-alert>
            <template v-else-if="diff">
              <div class="mb-2 d-flex align-center ga-2 text-caption">
                <Badge variant="success" class="font-mono">+{{ diff.added }}</Badge>
                <Badge variant="destructive" class="font-mono">−{{ diff.removed }}</Badge>
                <span class="text-medium-emphasis">
                  vs applied {{ applied?.appliedAt ? formatDateTime(applied.appliedAt) : '' }}
                </span>
              </div>
              <div
                data-testid="config-diff"
                class="overflow-auto rounded border font-mono text-caption diff-view"
                style="max-height: 28rem"
              >
                <div
                  v-for="(line, i) in diff.lines"
                  :key="i"
                  class="diff-line"
                  :class="`diff-line--${line.type}`"
                >
                  <span class="diff-gutter">{{ line.oldNumber ?? '' }}</span>
                  <span class="diff-gutter">{{ line.newNumber ?? '' }}</span>
                  <span class="diff-sign">{{ line.type === 'add' ? '+' : line.type === 'del' ? '-' : ' ' }}</span>
                  <span class="diff-text">{{ line.text }}</span>
                </div>
              </div>
            </template>
          </template>
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

<style scoped lang="scss">
.diff-view {
  background: rgb(var(--v-theme-surface-light));
  line-height: 1.5;
}
.diff-line {
  display: flex;
  white-space: pre;
  padding-inline-end: 8px;
}
.diff-gutter {
  flex: 0 0 auto;
  width: 40px;
  padding-inline: 8px 4px;
  text-align: right;
  color: rgba(var(--v-theme-on-surface), 0.4);
  user-select: none;
}
.diff-sign {
  flex: 0 0 auto;
  width: 16px;
  text-align: center;
  user-select: none;
}
.diff-text {
  flex: 1 1 auto;
  white-space: pre-wrap;
  word-break: break-word;
}
.diff-line--add {
  background: rgba(var(--v-theme-success), 0.14);
}
.diff-line--add .diff-sign {
  color: rgb(var(--v-theme-success));
}
.diff-line--del {
  background: rgba(var(--v-theme-error), 0.14);
}
.diff-line--del .diff-sign {
  color: rgb(var(--v-theme-error));
}
</style>
