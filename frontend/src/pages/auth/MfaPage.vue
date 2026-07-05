<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/composables/useAuth'
import { ApiError } from '@/services/http'
import { safeRedirect } from './redirect'

const route = useRoute()
const router = useRouter()
const { verifyMfa, enrollMfa, confirmMfa } = useAuth()

const enrolling = computed(() => route.query.mode === 'enroll')
const code = ref('')
const error = ref('')
const submitting = ref(false)
const secret = ref('')
const otpauthUri = ref('')

onMounted(async () => {
  if (!enrolling.value) return
  try {
    const res = await enrollMfa()
    secret.value = res.secret
    otpauthUri.value = res.otpauthUri
  } catch (err) {
    handleError(err)
  }
})

function handleError(err: unknown) {
  if (err instanceof ApiError && err.status === 401) {
    // No (or expired) partial session — restart from login.
    router.replace({ name: 'login' })
    return
  }
  error.value =
    err instanceof ApiError && (err.status === 400 || err.status === 422)
      ? 'That code is not valid. Try the current code from your authenticator.'
      : err instanceof Error
        ? err.message
        : 'Verification failed.'
}

async function onSubmit() {
  error.value = ''
  submitting.value = true
  try {
    if (enrolling.value) {
      await confirmMfa(code.value.trim())
    } else {
      await verifyMfa(code.value.trim())
    }
    router.replace(safeRedirect(route.query.redirect))
  } catch (err) {
    handleError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="d-flex align-center justify-center bg-background px-4" style="min-height: 100vh">
    <Card class="w-100" style="max-width: 384px">
      <CardHeader>
        <CardTitle>Two-factor authentication</CardTitle>
        <CardDescription>
          {{
            enrolling
              ? 'Set up an authenticator app to finish signing in.'
              : 'Enter the 6-digit code from your authenticator app.'
          }}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div v-if="enrolling && secret" class="mb-4 d-flex flex-column ga-2 rounded border bg-surface-light pa-3">
          <p class="text-caption text-medium-emphasis">
            Add this account to your authenticator app (Google Authenticator, 1Password,
            Authy…) using the setup key below, then enter the generated code.
          </p>
          <div class="d-flex flex-column ga-1">
            <Label class="text-caption">Setup key</Label>
            <code class="d-block text-break rounded bg-background px-2 py-1 text-caption">{{ secret }}</code>
          </div>
          <a
            :href="otpauthUri"
            class="text-caption text-primary"
          >
            Open in authenticator app
          </a>
        </div>

        <form class="d-flex flex-column ga-4" @submit.prevent="onSubmit">
          <div class="d-flex flex-column ga-1">
            <Label for="code">Authentication code</Label>
            <Input
              id="code"
              v-model="code"
              type="text"
              inputmode="numeric"
              autocomplete="one-time-code"
              placeholder="123456"
              :disabled="submitting"
            />
          </div>
          <p v-if="error" class="text-body-2 text-error" role="alert">{{ error }}</p>
          <Button type="submit" class="w-100" :disabled="submitting || code.length < 6">
            {{ submitting ? 'Verifying…' : enrolling ? 'Confirm & continue' : 'Verify' }}
          </Button>
          <RouterLink
            :to="{ name: 'login' }"
            class="d-block text-center text-caption text-medium-emphasis"
          >
            Back to sign in
          </RouterLink>
        </form>
      </CardContent>
    </Card>
  </div>
</template>
