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
  <div class="flex min-h-screen items-center justify-center bg-background px-4">
    <Card class="w-full max-w-sm">
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
        <div v-if="enrolling && secret" class="mb-4 space-y-2 rounded-md border bg-muted/40 p-3">
          <p class="text-xs text-muted-foreground">
            Add this account to your authenticator app (Google Authenticator, 1Password,
            Authy…) using the setup key below, then enter the generated code.
          </p>
          <div class="space-y-1">
            <Label class="text-xs">Setup key</Label>
            <code class="block break-all rounded bg-background px-2 py-1 text-xs">{{ secret }}</code>
          </div>
          <a
            :href="otpauthUri"
            class="text-xs text-primary underline-offset-4 hover:underline"
          >
            Open in authenticator app
          </a>
        </div>

        <form class="space-y-4" @submit.prevent="onSubmit">
          <div class="space-y-1.5">
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
          <p v-if="error" class="text-sm text-destructive" role="alert">{{ error }}</p>
          <Button type="submit" class="w-full" :disabled="submitting || code.length < 6">
            {{ submitting ? 'Verifying…' : enrolling ? 'Confirm & continue' : 'Verify' }}
          </Button>
          <RouterLink
            :to="{ name: 'login' }"
            class="block text-center text-xs text-muted-foreground underline-offset-4 hover:underline"
          >
            Back to sign in
          </RouterLink>
        </form>
      </CardContent>
    </Card>
  </div>
</template>
