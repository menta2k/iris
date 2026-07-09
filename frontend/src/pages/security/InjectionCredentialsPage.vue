<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { injectionCredentialsService } from '@/services'
import { ApiError } from '@/services/http'
import type { InjectionCredential } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<InjectionCredential>({
  loader: () => injectionCredentialsService.list(),
})
const { toast } = useToast()

const MIN_PASSWORD = 12

function formatDate(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(undefined, { hour12: false })
}

// ---- Create / edit dialog ----
const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

interface FormState {
  username: string
  password: string
  label: string
  enabled: boolean
  allowedMailclasses: string[]
}
function emptyForm(): FormState {
  return { username: '', password: '', label: '', enabled: true, allowedMailclasses: [] }
}
const form = ref<FormState>(emptyForm())
const isEdit = computed(() => mode.value === 'edit')

const passwordError = computed(() => {
  if (isEdit.value) return '' // password not edited here
  const p = form.value.password
  if (!p) return ''
  return p.length < MIN_PASSWORD ? `Password must be at least ${MIN_PASSWORD} characters` : ''
})
const canSubmit = computed(() => {
  if (isEdit.value) return true
  return (
    !!form.value.username.trim() &&
    form.value.password.length >= MIN_PASSWORD
  )
})

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  dialogOpen.value = true
}
function openEdit(c: InjectionCredential) {
  mode.value = 'edit'
  editId.value = c.id
  form.value = {
    username: c.username,
    password: '',
    label: c.label,
    enabled: c.enabled,
    allowedMailclasses: [...c.allowedMailclasses],
  }
  dialogOpen.value = true
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await injectionCredentialsService.update(editId.value, {
        id: editId.value,
        label: form.value.label,
        enabled: form.value.enabled,
        allowedMailclasses: form.value.allowedMailclasses,
      })
      toast({ title: 'Credential updated', description: form.value.username, variant: 'success' })
    } else {
      await injectionCredentialsService.create({
        username: form.value.username.trim(),
        password: form.value.password,
        label: form.value.label,
        enabled: form.value.enabled,
        allowedMailclasses: form.value.allowedMailclasses,
      })
      toast({ title: 'Credential created', description: form.value.username, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save credential.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

// ---- Reset password dialog ----
const pwDialogOpen = ref(false)
const pwSaving = ref(false)
const pwTarget = ref<InjectionCredential | null>(null)
const newPassword = ref('')
const newPasswordError = computed(() =>
  newPassword.value && newPassword.value.length < MIN_PASSWORD
    ? `Password must be at least ${MIN_PASSWORD} characters`
    : '',
)
function openResetPassword(c: InjectionCredential) {
  pwTarget.value = c
  newPassword.value = ''
  pwDialogOpen.value = true
}
async function submitPassword() {
  if (!pwTarget.value || newPassword.value.length < MIN_PASSWORD) return
  pwSaving.value = true
  try {
    await injectionCredentialsService.setPassword(pwTarget.value.id, newPassword.value)
    toast({ title: 'Password reset', description: pwTarget.value.username, variant: 'success' })
    pwDialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to reset password.'
    toast({ title: 'Reset failed', description: msg, variant: 'destructive' })
  } finally {
    pwSaving.value = false
  }
}

// ---- Delete ----
const deletingId = ref<string | null>(null)
async function remove(c: InjectionCredential) {
  deletingId.value = c.id
  try {
    await injectionCredentialsService.remove(c.id)
    toast({ title: 'Credential removed', description: c.username, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete credential.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  } finally {
    deletingId.value = null
  }
}

// ---- Enable/disable toggle (inline) ----
async function toggleEnabled(c: InjectionCredential) {
  try {
    await injectionCredentialsService.update(c.id, {
      id: c.id,
      label: c.label,
      enabled: !c.enabled,
      allowedMailclasses: c.allowedMailclasses,
    })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to update credential.'
    toast({ title: 'Update failed', description: msg, variant: 'destructive' })
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Injection API"
      description="Credentials for the GreenArrow-compatible mail-injection listener. Each key authenticates injection requests by username + password; a key may optionally be restricted to specific mailclasses. Passwords are stored hashed and shown only once, when set."
    >
      <template #actions>
        <Button data-testid="create-credential" @click="openCreate">New Credential</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No injection credentials yet. Create one to let an application inject mail."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Username</TableHead>
                <TableHead>Label</TableHead>
                <TableHead>Mailclasses</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Last used</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="c in items" :key="c.id">
                <TableCell class="font-mono font-weight-medium">{{ c.username }}</TableCell>
                <TableCell class="text-medium-emphasis">{{ c.label || '—' }}</TableCell>
                <TableCell>
                  <template v-if="c.allowedMailclasses.length">
                    <Badge v-for="m in c.allowedMailclasses" :key="m" variant="secondary" class="mr-1">{{ m }}</Badge>
                  </template>
                  <span v-else class="text-caption text-medium-emphasis">Any</span>
                </TableCell>
                <TableCell>
                  <Badge :variant="c.enabled ? 'success' : 'secondary'">{{ c.enabled ? 'Enabled' : 'Disabled' }}</Badge>
                </TableCell>
                <TableCell class="text-caption text-no-wrap">{{ formatDate(c.lastUsedAt) }}</TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <Button variant="outline" size="sm" :data-testid="`toggle-${c.id}`" @click="toggleEnabled(c)">
                      {{ c.enabled ? 'Disable' : 'Enable' }}
                    </Button>
                    <Button variant="outline" size="sm" :data-testid="`reset-pw-${c.id}`" @click="openResetPassword(c)">
                      Reset password
                    </Button>
                    <Button variant="outline" size="sm" :data-testid="`edit-${c.id}`" @click="openEdit(c)">Edit</Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :disabled="deletingId === c.id"
                      :data-testid="`delete-${c.id}`"
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
    </DataState>

    <!-- Create / edit dialog -->
    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit Credential' : 'New Injection Credential' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="inj-username">Username</Label>
          <Input id="inj-username" v-model="form.username" :disabled="isEdit" placeholder="acme-app" class="font-mono" />
          <p v-if="isEdit" class="text-caption text-medium-emphasis">Username can't be changed after creation.</p>
        </div>
        <div v-if="!isEdit" class="d-flex flex-column ga-1">
          <Label for="inj-password">Password</Label>
          <Input id="inj-password" v-model="form.password" type="password" placeholder="at least 12 characters" />
          <p v-if="passwordError" class="text-caption text-error">{{ passwordError }}</p>
          <p v-else class="text-caption text-medium-emphasis">
            Shown only once. The client sends this as its API password.
          </p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="inj-label">Label</Label>
          <Input id="inj-label" v-model="form.label" placeholder="Example transactional app" />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label>Allowed mailclasses</Label>
          <v-combobox
            v-model="form.allowedMailclasses"
            multiple
            chips
            closable-chips
            clearable
            variant="outlined"
            density="compact"
            hide-details
            placeholder="Leave empty to allow any mailclass"
            data-testid="inj-mailclasses"
          />
          <p class="text-caption text-medium-emphasis">
            Type a mailclass and press Enter. Empty means this key may inject any mailclass.
          </p>
        </div>
        <div class="d-flex align-center ga-2">
          <v-switch v-model="form.enabled" color="primary" density="compact" hide-details inset />
          <span class="text-body-2">{{ form.enabled ? 'Enabled' : 'Disabled' }}</span>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !canSubmit">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>

    <!-- Reset password dialog -->
    <Dialog v-model:open="pwDialogOpen">
      <DialogHeader>
        <DialogTitle>Reset password</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submitPassword">
        <p class="text-body-2 text-medium-emphasis">
          Set a new password for <code class="font-mono">{{ pwTarget?.username }}</code>. The client
          must be updated to use it.
        </p>
        <div class="d-flex flex-column ga-1">
          <Label for="inj-newpw">New password</Label>
          <Input id="inj-newpw" v-model="newPassword" type="password" placeholder="at least 12 characters" />
          <p v-if="newPasswordError" class="text-caption text-error">{{ newPasswordError }}</p>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="pwDialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="pwSaving || newPassword.length < MIN_PASSWORD">
            {{ pwSaving ? 'Saving…' : 'Reset password' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
