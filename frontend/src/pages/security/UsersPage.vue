<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
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
import { StatusBadge, Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { identityAuditService } from '@/services'
import { ApiError } from '@/services/http'
import type { User } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<User>({
  loader: () => identityAuditService.listUsers(),
})
const { toast } = useToast()

const BUILTIN_ROLES = ['owner', 'operator', 'security_admin', 'viewer'] as const
const USER_STATUSES = ['invited', 'active', 'disabled', 'locked']

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref<{
  email: string
  display_name: string
  mfa_required: boolean
  roles: string[]
  status: string
}>({
  email: '',
  display_name: '',
  mfa_required: true,
  roles: ['operator'],
  status: 'invited',
})

const isEdit = computed(() => mode.value === 'edit')

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = {
    email: '',
    display_name: '',
    mfa_required: true,
    roles: ['operator'],
    status: 'invited',
  }
  dialogOpen.value = true
}

function openEdit(u: User) {
  mode.value = 'edit'
  editId.value = u.id
  form.value = {
    email: u.email,
    display_name: u.displayName,
    mfa_required: u.mfaRequired,
    roles: [...(u.roles ?? [])],
    status: (u.status || 'active').toLowerCase(),
  }
  dialogOpen.value = true
}

function toggleRole(role: string) {
  const idx = form.value.roles.indexOf(role)
  if (idx === -1) form.value.roles.push(role)
  else form.value.roles.splice(idx, 1)
}

async function submit() {
  if (!isEdit.value && !form.value.email) return
  if (form.value.roles.length === 0) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await identityAuditService.updateUser(editId.value, {
        display_name: form.value.display_name,
        status: form.value.status,
        mfa_required: form.value.mfa_required,
        roles: [...form.value.roles],
      })
      toast({ title: 'User updated', description: form.value.email, variant: 'success' })
    } else {
      await identityAuditService.createUser({
        email: form.value.email,
        display_name: form.value.display_name,
        mfa_required: form.value.mfa_required,
        roles: [...form.value.roles],
      })
      toast({ title: 'User created', description: form.value.email, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save user.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

// --- Reset password (admin) ---
const MIN_PASSWORD_LENGTH = 12

const resetOpen = ref(false)
const resetSaving = ref(false)
const resetId = ref<string | null>(null)
const resetEmail = ref('')
const resetPw = ref('')
const resetConfirm = ref('')

const resetValid = computed(
  () =>
    resetPw.value.length >= MIN_PASSWORD_LENGTH && resetPw.value === resetConfirm.value,
)

function openReset(u: User) {
  resetId.value = u.id
  resetEmail.value = u.email
  resetPw.value = ''
  resetConfirm.value = ''
  resetOpen.value = true
}

// 16 chars from an unambiguous alphabet; enough entropy to hand off as a
// temporary password, easy to read aloud.
function generatePassword() {
  const alphabet = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  let out = ''
  for (let i = 0; i < 16; i++) out += alphabet[bytes[i] % alphabet.length]
  resetPw.value = out
  resetConfirm.value = out
}

async function submitReset() {
  if (!resetId.value || !resetValid.value) return
  resetSaving.value = true
  try {
    await identityAuditService.resetPassword(resetId.value, { password: resetPw.value })
    toast({ title: 'Password reset', description: resetEmail.value, variant: 'success' })
    resetOpen.value = false
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to reset password.'
    toast({ title: 'Reset failed', description: msg, variant: 'destructive' })
  } finally {
    resetSaving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Users" description="Administrators and operators with access to Iris.">
      <template #actions>
        <Button data-testid="create-user" @click="openCreate">New User</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No users found."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Roles</TableHead>
                <TableHead>MFA</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="u in items" :key="u.id">
                <TableCell class="font-medium">{{ u.displayName }}</TableCell>
                <TableCell>{{ u.email }}</TableCell>
                <TableCell>
                  <div class="flex flex-wrap gap-1">
                    <Badge v-for="r in u.roles" :key="r" variant="secondary">{{ r }}</Badge>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge :variant="u.mfaRequired ? 'success' : 'destructive'">
                    {{ u.mfaRequired ? 'Required' : 'Optional' }}
                  </Badge>
                </TableCell>
                <TableCell><StatusBadge :status="u.status" /></TableCell>
                <TableCell class="text-right">
                  <div class="flex justify-end gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      :data-testid="`edit-user-${u.id}`"
                      @click="openEdit(u)"
                    >
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :data-testid="`reset-user-${u.id}`"
                      @click="openReset(u)"
                    >
                      Reset password
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit User' : 'Create User' }}</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="user-email">Email</Label>
          <Input
            id="user-email"
            v-model="form.email"
            type="email"
            placeholder="ops@example.com"
            :disabled="isEdit"
          />
          <p v-if="isEdit" class="text-xs text-muted-foreground">Email is immutable.</p>
        </div>
        <div class="space-y-1.5">
          <Label for="user-name">Display Name</Label>
          <Input id="user-name" v-model="form.display_name" placeholder="Ops Team" />
        </div>
        <div v-if="isEdit" class="space-y-1.5">
          <Label for="user-status">Status</Label>
          <Select id="user-status" v-model="form.status">
            <option v-for="s in USER_STATUSES" :key="s" :value="s">{{ s }}</option>
          </Select>
        </div>
        <div class="space-y-1.5">
          <Label>Roles</Label>
          <div class="flex flex-wrap gap-3">
            <label v-for="r in BUILTIN_ROLES" :key="r" class="flex items-center gap-1.5 text-sm">
              <input
                type="checkbox"
                :value="r"
                :checked="form.roles.includes(r)"
                @change="toggleRole(r)"
              />
              {{ r }}
            </label>
          </div>
        </div>
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" v-model="form.mfa_required" />
          Require MFA
        </label>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || (!isEdit && !form.email) || form.roles.length === 0">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>

    <Dialog v-model:open="resetOpen">
      <DialogHeader>
        <DialogTitle>Reset password</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submitReset">
        <p class="text-sm text-muted-foreground">
          Set a new password for
          <span class="font-medium text-foreground">{{ resetEmail }}</span>. They can use it to
          sign in immediately.
        </p>
        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <Label for="reset-pw">New password</Label>
            <button
              type="button"
              class="text-xs text-primary hover:underline"
              @click="generatePassword"
            >
              Generate
            </button>
          </div>
          <Input
            id="reset-pw"
            v-model="resetPw"
            type="password"
            autocomplete="new-password"
            placeholder="At least 12 characters"
          />
        </div>
        <div class="space-y-1.5">
          <Label for="reset-confirm">Confirm password</Label>
          <Input
            id="reset-confirm"
            v-model="resetConfirm"
            type="password"
            autocomplete="new-password"
          />
          <p v-if="resetConfirm && resetConfirm !== resetPw" class="text-xs text-destructive">
            Passwords do not match.
          </p>
          <p v-else-if="resetPw.length > 0 && resetPw.length < MIN_PASSWORD_LENGTH" class="text-xs text-muted-foreground">
            At least {{ MIN_PASSWORD_LENGTH }} characters.
          </p>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="resetOpen = false">Cancel</Button>
          <Button type="submit" :disabled="resetSaving || !resetValid">
            {{ resetSaving ? 'Saving…' : 'Reset' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
