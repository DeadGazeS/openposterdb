<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { selfApi } from '@/lib/api'
import type { SaveSettingsPayload } from '@/lib/settings'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import type { RenderSettings } from '@/lib/settings'
import { Button } from '@/components/ui/button'
import { BookOpen } from 'lucide-vue-next'
import { useRenderSettingsForm } from '@/composables/useRenderSettingsForm'

const auth = useAuthStore()
const router = useRouter()

const keyName = ref('')
const keyPrefix = ref('')
const settings = ref<RenderSettings | null>(null)
const settingsLoading = ref(true)
const initError = ref('')

// Shared load/save/reset pattern — mirrors SettingsView's wire-up via the
// same composable. The onLoad hook keeps the local `settings` ref in sync
// so RenderSettingsForm reflects the freshly-fetched data immediately.
const { loadSettings, saveSettings, resetSettings } = useRenderSettingsForm<SaveSettingsPayload>({
  api: {
    load: () => selfApi.getSettings(),
    save: (p) => selfApi.updateSettings(p as SaveSettingsPayload),
    reset: () => selfApi.resetSettings(),
  },
  onLoad: (data) => { settings.value = data },
})

onMounted(async () => {
  try {
    const [infoRes, settingsRes] = await Promise.all([
      selfApi.getInfo(),
      selfApi.getSettings(),
    ])

    if (!infoRes.ok || !settingsRes.ok) {
      auth.logoutApiKey()
      router.replace('/login')
      return
    }

    const info = await infoRes.json()
    keyName.value = info.name
    keyPrefix.value = info.key_prefix
    auth.apiKeyInfo = { name: info.name, key_prefix: info.key_prefix }

    settings.value = await settingsRes.json()
  } catch {
    initError.value = 'Failed to load settings'
  } finally {
    settingsLoading.value = false
  }
})

function handleLogout() {
  auth.logoutApiKey()
  router.push('/login')
}
</script>

<template>
  <main class="min-h-screen flex items-center justify-center">
    <div class="w-full max-w-md space-y-6">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold">Image Settings</h1>
          <p v-if="keyName" class="text-sm text-muted-foreground">
            {{ keyName }}
            <span class="font-mono">({{ keyPrefix }}...)</span>
          </p>
        </div>
        <Button v-if="auth.disablePublicPages" as-child variant="outline" size="sm">
          <router-link to="/docs">
            <BookOpen class="h-4 w-4" />
            API Docs
          </router-link>
        </Button>
        <Button variant="outline" size="sm" @click="handleLogout">Logout</Button>
      </div>

      <div v-if="settingsLoading" class="text-sm text-muted-foreground">Loading settings...</div>
      <div v-else-if="initError" class="text-sm text-destructive">{{ initError }}</div>

      <div v-else-if="settings" class="rounded-md border p-4">
        <RenderSettingsForm
          :settings="settings"
          uid="self"
          :load-settings="loadSettings"
          :save-settings="saveSettings"
          :reset-settings="resetSettings"
          :fetch-preview="selfApi.preview"
                                      />
      </div>
    </div>
  </main>
</template>
