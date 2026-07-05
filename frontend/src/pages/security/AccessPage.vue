<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useToast } from '@/composables/useToast'
import { identityAuditService } from '@/services'
import { ApiError } from '@/services/http'

const rolePermissions = [
  { role: 'admin', perms: ['Full read/write across all modules', 'User & access management', 'Service control'] },
  { role: 'operator', perms: ['Outbound & operations write', 'Domain safety & inbound write', 'Read-only security'] },
  { role: 'viewer', perms: ['Read-only across all modules'] },
]

const { toast } = useToast()
// MFA enrollment for the signed-in user (TOTP).
const enrollment = ref<{ secret: string; otpauthUri: string } | null>(null)
const code = ref('')
const busy = ref(false)
const enrolled = ref(false)

async function startEnroll() {
  busy.value = true
  try {
    enrollment.value = await identityAuditService.enrollMfa()
    enrolled.value = false
  } catch (err) {
    toast({ title: 'Could not start enrollment', description: msg(err), variant: 'destructive' })
  } finally {
    busy.value = false
  }
}

async function confirmEnroll() {
  if (code.value.length !== 6) return
  busy.value = true
  try {
    await identityAuditService.confirmMfa(code.value)
    enrolled.value = true
    enrollment.value = null
    code.value = ''
    toast({ title: 'MFA enabled', description: 'Your authenticator is now required to sign in.', variant: 'success' })
  } catch (err) {
    toast({ title: 'Invalid code', description: msg(err), variant: 'destructive' })
  } finally {
    busy.value = false
  }
}

async function disable() {
  busy.value = true
  try {
    await identityAuditService.disableMfa()
    enrolled.value = false
    enrollment.value = null
    toast({ title: 'MFA disabled', variant: 'success' })
  } catch (err) {
    toast({ title: 'Could not disable MFA', description: msg(err), variant: 'destructive' })
  } finally {
    busy.value = false
  }
}

function msg(err: unknown) {
  return err instanceof ApiError ? err.message : 'Unexpected error.'
}
</script>

<template>
  <div>
    <PageHeader
      title="MFA & Permissions"
      description="Role-based access control and multi-factor settings."
    />

    <Card class="mb-4">
      <CardHeader>
        <CardTitle>Multi-Factor Authentication</CardTitle>
        <CardDescription>
          Enroll a TOTP authenticator (Google Authenticator, 1Password, etc.) for your account.
        </CardDescription>
      </CardHeader>
      <CardContent class="d-flex flex-column ga-4">
        <div v-if="enrolled" class="d-flex align-center ga-3">
          <Badge variant="secondary">Enabled</Badge>
          <Button variant="outline" size="sm" :disabled="busy" @click="disable">Disable MFA</Button>
        </div>

        <template v-else-if="enrollment">
          <p class="text-body-2 text-medium-emphasis">
            Add this account to your authenticator app, then enter the 6-digit code to confirm.
          </p>
          <div class="d-flex flex-column ga-1">
            <Label>Secret</Label>
            <div class="font-mono text-body-2 text-break rounded border bg-surface-light px-3 py-2">
              {{ enrollment.secret }}
            </div>
          </div>
          <div class="d-flex flex-column ga-1">
            <Label>otpauth URI</Label>
            <div class="font-mono text-caption text-break text-medium-emphasis">{{ enrollment.otpauthUri }}</div>
          </div>
          <div class="d-flex align-end ga-2">
            <div class="d-flex flex-column ga-1">
              <Label for="mfa-code">Code</Label>
              <Input id="mfa-code" v-model="code" inputmode="numeric" maxlength="6" placeholder="123456" style="width: 128px" />
            </div>
            <Button :disabled="busy || code.length !== 6" @click="confirmEnroll">Confirm</Button>
          </div>
        </template>

        <div v-else>
          <Button :disabled="busy" data-testid="enroll-mfa" @click="startEnroll">
            {{ busy ? 'Starting…' : 'Set up MFA' }}
          </Button>
        </div>
      </CardContent>
    </Card>

    <v-row dense>
      <v-col v-for="r in rolePermissions" :key="r.role" cols="12" md="4">
        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="d-flex align-center ga-2 text-subtitle-1">
              <Badge variant="secondary">{{ r.role }}</Badge>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul class="d-flex flex-column ga-1 text-body-2 text-medium-emphasis">
              <li v-for="p in r.perms" :key="p" class="d-flex ga-2">
                <span class="text-primary">•</span>{{ p }}
              </li>
            </ul>
          </CardContent>
        </Card>
      </v-col>
    </v-row>
  </div>
</template>
