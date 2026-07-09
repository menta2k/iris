<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import PaginationControls from '@/components/common/PaginationControls.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent } from '@/components/ui/card'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { classificationsService } from '@/services'
import { ApiError } from '@/services/http'
import type { SubjectClassification, SubjectMatchType } from '@/types'
import { simulate, compileRule, DEFAULT_SIMILARITY_THRESHOLD, type RuleMatch } from '@/utils/subjectMatch'

const { items, loading, error, notImplemented, load } = useAsyncList<SubjectClassification>({
  loader: () => classificationsService.list(),
})
const { toast } = useToast()

// Colour-code the Source column: manual rules (human-authored) → indigo,
// AI-generated labels → green. Unknown sources fall back to neutral.
const SOURCE_VARIANT: Record<string, 'default' | 'success'> = {
  manual: 'default',
  ai: 'success',
}
function sourceVariant(source: string) {
  return SOURCE_VARIANT[(source || '').toLowerCase()] ?? 'secondary'
}
function sourceLabel(source: string) {
  const s = (source || '').toLowerCase()
  if (s === 'ai') return 'AI'
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : 'Unknown'
}

const MATCH_TYPE_ITEMS: Array<{ title: string; value: SubjectMatchType }> = [
  { title: 'Similarity (trigram)', value: 'similarity' },
  { title: 'Regex', value: 'regex' },
]
function matchTypeOf(c: SubjectClassification): SubjectMatchType {
  return c.matchType === 'regex' ? 'regex' : 'similarity'
}
function isRegex(c: SubjectClassification) {
  return matchTypeOf(c) === 'regex'
}

// Client-side pagination: the endpoint returns the full list, so slice it here.
const pageSize = ref(25)
const pageNumber = ref(1)
const totalPages = computed(() => Math.max(1, Math.ceil(items.value.length / pageSize.value)))
const pagedItems = computed(() => {
  const start = (pageNumber.value - 1) * pageSize.value
  return items.value.slice(start, start + pageSize.value)
})
const hasPrev = computed(() => pageNumber.value > 1)
const hasNext = computed(() => pageNumber.value < totalPages.value)

// Keep the current page in range when the list shrinks (delete) or reloads.
watch(totalPages, (n) => {
  if (pageNumber.value > n) pageNumber.value = n
})

function goPrev() {
  if (hasPrev.value) pageNumber.value -= 1
}
function goNext() {
  if (hasNext.value) pageNumber.value += 1
}
function setPageSize(size: number) {
  pageSize.value = size
  pageNumber.value = 1
}

const dialogOpen = ref(false)
const saving = ref(false)
const deletingId = ref<string | null>(null)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

interface FormState {
  subject: string
  label: string
  matchType: SubjectMatchType
  priority: number
}

function emptyForm(): FormState {
  return { subject: '', label: '', matchType: 'similarity', priority: 0 }
}

const form = ref<FormState>(emptyForm())
const isEdit = computed(() => mode.value === 'edit')
const formIsRegex = computed(() => form.value.matchType === 'regex')
// For a regex rule, validate that the pattern compiles as a JS RegExp so the
// operator gets immediate feedback (the backend re-validates authoritatively).
const patternError = computed(() => {
  if (!formIsRegex.value) return ''
  const p = form.value.subject.trim()
  if (!p) return ''
  try {
    // Strip a leading inline flag group like (?i) which JS can't parse inline.
    new RegExp(p.replace(/^\(\?[ims]+\)/, ''))
    return ''
  } catch (e) {
    return e instanceof Error ? e.message : 'Invalid regular expression'
  }
})
const canSubmit = computed(
  () => !!form.value.subject.trim() && !!form.value.label.trim() && !patternError.value,
)

// Inline pattern tester (regex rules only): try the pattern being authored
// against a sample subject so the operator can confirm it before saving.
const regexTest = ref('')
const regexTestMatches = computed<boolean | null>(() => {
  if (!formIsRegex.value || !form.value.subject.trim() || !regexTest.value) return null
  if (patternError.value) return null
  const re = compileRule(form.value.subject.trim())
  return re ? re.test(regexTest.value) : null
})

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  regexTest.value = ''
  dialogOpen.value = true
}

function openEdit(c: SubjectClassification) {
  mode.value = 'edit'
  editId.value = c.id
  form.value = { subject: c.subject, label: c.label, matchType: matchTypeOf(c), priority: c.priority ?? 0 }
  regexTest.value = ''
  dialogOpen.value = true
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  const payload = {
    subject: form.value.subject.trim(),
    label: form.value.label,
    matchType: form.value.matchType,
    priority: Number(form.value.priority) || 0,
  }
  try {
    if (isEdit.value && editId.value) {
      await classificationsService.update(editId.value, { id: editId.value, ...payload })
      toast({ title: 'Classification updated', description: form.value.label, variant: 'success' })
    } else {
      await classificationsService.create(payload)
      toast({ title: 'Classification created', description: form.value.label, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save classification.'
    toast({
      title: isEdit.value ? 'Update failed' : 'Create failed',
      description: msg,
      variant: 'destructive',
    })
  } finally {
    saving.value = false
  }
}

async function remove(c: SubjectClassification) {
  deletingId.value = c.id
  try {
    await classificationsService.remove(c.id)
    toast({ title: 'Classification removed', description: c.label, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete classification.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  } finally {
    deletingId.value = null
  }
}

// ---- Match simulator ----
// Preview which rule would classify a given subject, mirroring the server's
// matcher (priority-ordered, first match wins). Regex + ordering are exact;
// similarity is an approximation of the database's pg_trgm score.
const simSubject = ref('')
const simResult = computed(() =>
  simSubject.value.trim() ? simulate(simSubject.value, items.value) : null,
)
const simThreshold = DEFAULT_SIMILARITY_THRESHOLD
function reasonText(m: RuleMatch): string {
  if (m.reason.kind === 'regex') return 'regex'
  if (m.reason.kind === 'similarity') return `~${(m.reason.score * 100).toFixed(0)}% similar`
  return 'invalid'
}
</script>

<template>
  <div>
    <PageHeader
      title="Subject Classifications"
      description="Rules mapping email subjects to a short label. A rule matches either by trigram similarity of the subject or by a regular expression. Rules are evaluated highest-priority first and the first match wins. AI-generated similarity rules are added automatically. Enable the feature under Global Settings."
    >
      <template #actions>
        <Button data-testid="create-classification" @click="openCreate">New Rule</Button>
      </template>
    </PageHeader>

    <!-- Match simulator: preview which rule classifies a given subject. -->
    <Card class="mb-4">
      <CardContent class="pa-4">
        <div class="d-flex align-center ga-2 mb-3">
          <v-icon icon="mdi-flask-outline" size="small" class="text-medium-emphasis" />
          <span class="text-subtitle-2">Simulate a match</span>
          <span class="text-caption text-medium-emphasis">
            Enter a subject to see which rule would classify it.
          </span>
        </div>
        <v-text-field
          v-model="simSubject"
          label="Test subject"
          placeholder="e.g. [SECURITY] New sign-in to your account"
          prepend-inner-icon="mdi-email-search-outline"
          variant="outlined"
          density="compact"
          hide-details
          clearable
          data-testid="sim-subject"
        />

        <div v-if="simResult" class="mt-3">
          <!-- Winner banner -->
          <v-alert
            v-if="simResult.winner"
            type="success"
            variant="tonal"
            density="compact"
            data-testid="sim-winner"
          >
            <div class="d-flex align-center flex-wrap ga-2">
              <span>Classified as</span>
              <Badge variant="success">{{ simResult.winner.rule.label }}</Badge>
              <span class="text-medium-emphasis">
                via
                <Badge :variant="simResult.winner.rule.matchType === 'regex' ? 'warning' : 'secondary'">
                  {{ simResult.winner.rule.matchType === 'regex' ? 'Regex' : 'Similarity' }}
                </Badge>
                rule
                <code class="font-mono">{{ simResult.winner.rule.subject }}</code>
                (priority {{ simResult.winner.rule.priority }}, {{ reasonText(simResult.winner) }})
              </span>
            </div>
          </v-alert>
          <v-alert v-else type="info" variant="tonal" density="compact" data-testid="sim-nomatch">
            No rule matches — this subject would fall through to the AI classifier (or stay
            unlabeled if AI is off).
          </v-alert>

          <!-- All matching rules, in evaluation order -->
          <div v-if="simResult.matches.length > 1" class="mt-3">
            <div class="text-caption text-medium-emphasis mb-1">
              All matching rules (evaluation order — winner first):
            </div>
            <div
              v-for="(m, i) in simResult.matches"
              :key="m.rule.id"
              class="d-flex align-center ga-2 py-1 text-body-2"
              :class="{ 'text-medium-emphasis': i !== 0 }"
            >
              <v-icon
                :icon="i === 0 ? 'mdi-trophy-outline' : 'mdi-circle-small'"
                size="small"
                :color="i === 0 ? 'success' : undefined"
              />
              <span class="tabular-nums" style="min-width: 24px">{{ m.rule.priority }}</span>
              <Badge :variant="m.reason.kind === 'regex' ? 'warning' : 'secondary'">
                {{ m.reason.kind === 'regex' ? 'Regex' : 'Similarity' }}
              </Badge>
              <span class="font-weight-medium">{{ m.rule.label }}</span>
              <code class="font-mono text-truncate" style="max-width: 320px">{{ m.rule.subject }}</code>
              <span class="text-caption text-medium-emphasis">{{ reasonText(m) }}</span>
            </div>
          </div>

          <p class="text-caption text-medium-emphasis mt-2">
            Similarity scores are an approximation of the server's pg_trgm match (threshold
            {{ simThreshold }}); regex matching and priority order are exact.
          </p>
        </div>
      </CardContent>
    </Card>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No classification rules yet. They appear here as you add rules or the AI labels new subjects."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="text-right" style="width: 90px">Priority</TableHead>
                <TableHead>Subject / Pattern</TableHead>
                <TableHead>Label</TableHead>
                <TableHead>Match</TableHead>
                <TableHead>Source</TableHead>
                <TableHead class="text-right">Hits</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="c in pagedItems" :key="c.id">
                <TableCell class="text-right tabular-nums text-medium-emphasis">{{ c.priority }}</TableCell>
                <TableCell
                  :class="isRegex(c) ? 'text-truncate font-mono text-body-2' : 'text-truncate'"
                  style="max-width: 420px"
                  :title="c.subject"
                >{{ c.subject }}</TableCell>
                <TableCell class="font-weight-medium">{{ c.label }}</TableCell>
                <TableCell>
                  <Badge :variant="isRegex(c) ? 'warning' : 'secondary'">{{ isRegex(c) ? 'Regex' : 'Similarity' }}</Badge>
                </TableCell>
                <TableCell><Badge :variant="sourceVariant(c.source)">{{ sourceLabel(c.source) }}</Badge></TableCell>
                <TableCell class="text-right tabular-nums">{{ c.hitCount }}</TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <Button
                      variant="outline"
                      size="sm"
                      :data-testid="`edit-classification-${c.id}`"
                      @click="openEdit(c)"
                    >
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :disabled="deletingId === c.id"
                      :data-testid="`delete-classification-${c.id}`"
                      @click="remove(c)"
                    >
                      {{ deletingId === c.id ? 'Removing…' : 'Remove' }}
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <PaginationControls
        :page-number="pageNumber"
        :page-size="pageSize"
        :has-prev="hasPrev"
        :has-next="hasNext"
        @prev="goPrev"
        @next="goNext"
        @page-size-change="setPageSize"
      />
    </DataState>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit Classification' : 'Create Classification' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex ga-4">
          <div class="d-flex flex-column ga-1 flex-grow-1">
            <Label>Match type</Label>
            <v-select
              v-model="form.matchType"
              :items="MATCH_TYPE_ITEMS"
              item-title="title"
              item-value="value"
              variant="outlined"
              density="compact"
              hide-details
              data-testid="cls-match-type"
            />
          </div>
          <div class="d-flex flex-column ga-1" style="width: 120px">
            <Label for="cls-priority">Priority</Label>
            <v-text-field
              id="cls-priority"
              v-model.number="form.priority"
              type="number"
              variant="outlined"
              density="compact"
              hide-details
              data-testid="cls-priority"
            />
          </div>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="cls-subject">{{ formIsRegex ? 'Pattern' : 'Subject' }}</Label>
          <Input
            id="cls-subject"
            v-model="form.subject"
            :class="formIsRegex ? 'font-mono' : ''"
            :placeholder="formIsRegex ? '(?i)^invoice #\\d+' : 'Your order has shipped'"
          />
          <p v-if="patternError" class="text-caption text-error">{{ patternError }}</p>
          <p v-else class="text-caption text-medium-emphasis">
            <template v-if="formIsRegex">
              A regular expression tested against the raw subject. Use <code>(?i)</code> for
              case-insensitive matching.
            </template>
            <template v-else>
              A representative subject. It is normalized (digits/prefixes stripped) into a matching
              key, so similar subjects match this rule.
            </template>
          </p>
        </div>

        <!-- Inline tester: try the pattern against a sample subject before saving. -->
        <div v-if="formIsRegex" class="d-flex flex-column ga-1">
          <Label for="cls-regex-test">Test this pattern</Label>
          <v-text-field
            id="cls-regex-test"
            v-model="regexTest"
            placeholder="Paste a subject to test the pattern"
            variant="outlined"
            density="compact"
            hide-details
            clearable
            data-testid="cls-regex-test"
          >
            <template #append-inner>
              <v-chip
                v-if="regexTestMatches !== null"
                :color="regexTestMatches ? 'success' : 'error'"
                size="small"
                variant="tonal"
                data-testid="cls-regex-test-result"
              >
                <v-icon start :icon="regexTestMatches ? 'mdi-check' : 'mdi-close'" />
                {{ regexTestMatches ? 'Matches' : 'No match' }}
              </v-chip>
            </template>
          </v-text-field>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="cls-label">Label</Label>
          <Input id="cls-label" v-model="form.label" placeholder="shipping update" />
          <p class="text-caption text-medium-emphasis">One or two words. Longer input is truncated.</p>
        </div>
        <p class="text-caption text-medium-emphasis">
          Higher priority is evaluated first; the first matching rule wins.
        </p>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !canSubmit">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
