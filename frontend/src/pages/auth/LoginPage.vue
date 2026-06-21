<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
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
const { login } = useAuth()

const email = ref('')
const password = ref('')
const error = ref('')
const submitting = ref(false)

async function onSubmit() {
  error.value = ''
  submitting.value = true
  try {
    const redirect = safeRedirect(route.query.redirect)
    const status = await login(email.value.trim(), password.value)
    if (status === 'authenticated') {
      router.replace(redirect)
    } else if (status === 'mfa_enrollment_required') {
      router.replace({ name: 'mfa', query: { mode: 'enroll', redirect } })
    } else {
      // mfa_required (or any other non-terminal status) → verify a code.
      router.replace({ name: 'mfa', query: { mode: 'verify', redirect } })
    }
  } catch (err) {
    error.value =
      err instanceof ApiError && err.status === 401
        ? 'Invalid email or password.'
        : err instanceof Error
          ? err.message
          : 'Sign in failed.'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-background px-4">
    <Card class="w-full max-w-sm">
      <CardHeader>
        <div class="mb-1 flex items-center gap-2">
          <div
            class="flex h-7 w-7 items-center justify-center rounded bg-primary text-sm font-bold text-primary-foreground"
          >
            I
          </div>
          <span class="text-sm font-semibold">Iris</span>
        </div>
        <CardTitle>Sign in</CardTitle>
        <CardDescription>KumoMTA operator console</CardDescription>
      </CardHeader>
      <CardContent>
        <form class="space-y-4" @submit.prevent="onSubmit">
          <div class="space-y-1.5">
            <Label for="email">Email</Label>
            <Input
              id="email"
              v-model="email"
              type="email"
              placeholder="you@example.com"
              :disabled="submitting"
            />
          </div>
          <div class="space-y-1.5">
            <Label for="password">Password</Label>
            <Input
              id="password"
              v-model="password"
              type="password"
              placeholder="••••••••"
              :disabled="submitting"
            />
          </div>
          <p v-if="error" class="text-sm text-destructive" role="alert">{{ error }}</p>
          <Button
            type="submit"
            class="w-full"
            :disabled="submitting || !email || !password"
          >
            {{ submitting ? 'Signing in…' : 'Sign in' }}
          </Button>
        </form>
      </CardContent>
    </Card>
  </div>
</template>
