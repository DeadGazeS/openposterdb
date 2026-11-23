<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { commonAuthMessage } from '@/lib/auth-result'

const auth = useAuthStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const error = ref('')
const loading = ref(false)

async function handleSetup() {
  error.value = ''
  if (password.value !== confirmPassword.value) {
    error.value = 'Passwords do not match'
    return
  }
  if (password.value.length < 8) {
    error.value = 'Password must be at least 8 characters'
    return
  }
  loading.value = true
  try {
    const result = await auth.setup(username.value, password.value)
    if (result.ok) {
      router.push('/admin')
    } else if (result.reason === 'invalid') {
      // The backend's validation text (fixed strings, no user input echoed).
      error.value = result.message ?? 'Invalid username or password.'
    } else if (result.reason === 'forbidden') {
      error.value = 'Setup is already complete: an admin account exists. Sign in instead.'
    } else {
      error.value = commonAuthMessage(result) ?? 'Setup failed'
    }
  } catch {
    error.value = 'Setup failed'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="min-h-screen flex items-center justify-center">
    <div class="w-full max-w-sm space-y-6">
      <div class="text-center">
        <h1 class="text-2xl font-bold">OpenPosterDB Setup</h1>
        <p class="text-muted-foreground">Create your admin account</p>
      </div>

      <form class="space-y-4" @submit.prevent="handleSetup">
        <div>
          <Label for="username" class="mb-1">Username</Label>
          <Input
            id="username"
            v-model="username"
            type="text"
            autocomplete="username"
            required
          />
        </div>
        <div>
          <Label for="password" class="mb-1">Password</Label>
          <Input
            id="password"
            v-model="password"
            type="password"
            autocomplete="new-password"
            required
          />
        </div>
        <div>
          <Label for="confirm-password" class="mb-1">Confirm Password</Label>
          <Input
            id="confirm-password"
            v-model="confirmPassword"
            type="password"
            autocomplete="new-password"
            required
          />
        </div>
        <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
        <Button type="submit" class="w-full" :disabled="loading">
          {{ loading ? 'Creating account...' : 'Create admin account' }}
        </Button>
      </form>
    </div>
  </main>
</template>
