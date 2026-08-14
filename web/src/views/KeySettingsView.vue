<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { selfApi } from '@/lib/api'
import type { SaveSettingsPayload } from '@/lib/settings'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import type { RenderSettings } from '@/lib/settings'
import { Button } from '@/components/ui/button'
import { BookOpen, Check, Loader2 } from 'lucide-vue-next'
import RefreshButton from '@/components/RefreshButton.vue'
import { useRenderSettingsForm } from '@/composables/useRenderSettingsForm'

const auth = useAuthStore()
const router = useRouter()

const keyName = ref('')
const keyPrefix = ref('')
const settings = ref<RenderSettings | null>(null)
const settingsLoading = ref(true)
const initError = ref('')

// Centralised load that fills `keyName`/`keyPrefix`/`settings` from the
// self endpoints and stores any thrown / non-ok result in `initError`. Called
// from onMounted on first load and again from the retry button if the initial
// fetch fails.
async function loadInitial() {
  settingsLoading.value = true
  initError.value = ''
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
  } catch (e) {
    initError.value = e instanceof Error ? e.message : String(e) || 'Failed to load settings'
  } finally {
    settingsLoading.value = false
  }
}

onMounted(loadInitial)

// Shared load/save/reset pattern — mirrors SettingsView's wire-up via the
// same composable. The onLoad hook keeps the local `settings` ref in sync
// so RenderSettingsForm reflects the freshly-fetched data immediately.
const { loadSettings, saveSettings, resetSettings } = useRenderSettingsForm<SaveSettingsPayload>({
  api: {
    load: () => selfApi.getSettings(),
    save: (p) => selfApi.updateSettings(p),
    reset: () => selfApi.resetSettings(),
  },
  onLoad: (data) => { settings.value = data },
})

// Template ref to RenderSettingsForm so the page header's Save / Discard can
// drive the same form instance Global Image Settings uses, and so the form's
// dirty/saving state can show / hide those buttons and the success check.
const formRef = ref<{
  save: () => Promise<void>
  discard: () => void
  dirty: boolean
  saving: boolean
  showCheck: boolean
  error: string
} | null>(null)

const userRefreshing = ref(false)

async function handleRefresh() {
  userRefreshing.value = true
  try {
    await loadSettings()
  } finally {
    userRefreshing.value = false
  }
}

// Reset = clear all per-key overrides and fall back to whatever Global Image
// Settings are. Only meaningful when the stored per-key settings actually have
// overrides (is_default === false). On success the reset endpoint flips
// is_default to true and we re-load to refresh the form.
const resetting = ref(false)
async function handleReset() {
  if (resetting.value) return
  resetting.value = true
  try {
    const ok = await resetSettings()
    if (ok) await loadSettings()
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <div class="rounded-lg border p-6 space-y-4">
    <div class="flex items-center justify-between gap-3 flex-wrap">
      <h3 class="w-fit border border-t-0 border-l-0 rounded-tl-md rounded-br-md bg-muted px-4 py-2 text-sm font-bold uppercase tracking-widest">
        Individual Image Settings
      </h3>
      <div class="flex items-center gap-2">
        <span v-if="formRef?.showCheck" class="flex items-center gap-1.5 text-sm text-green-500">
          <Check class="size-4" />
          Saved
        </span>
        <span v-if="formRef?.error" class="text-sm text-destructive">{{ formRef.error }}</span>
        <Button
          v-if="settings && !settings.is_default"
          variant="outline"
          size="sm"
          data-testid="reset-to-defaults-button"
          :disabled="resetting"
          @click="handleReset"
        >
          <Loader2 v-if="resetting" class="size-4 animate-spin mr-1" />
          Reset to defaults
        </Button>
        <Button
          v-if="formRef?.dirty"
          variant="outline"
          size="sm"
          data-testid="discard-settings-button"
          @click="formRef.discard()"
        >
          Discard changes
        </Button>
        <Button
          v-if="formRef?.dirty"
          data-testid="save-settings-button"
          :disabled="formRef.saving"
          @click="formRef.save()"
        >
          <Loader2 v-if="formRef?.saving" class="size-4 animate-spin mr-1" />
          Save
        </Button>
        <RefreshButton :fetching="userRefreshing" @refresh="handleRefresh" />
        <Button v-if="auth.disablePublicPages" as-child variant="outline" size="sm">
          <router-link to="/docs">
            <BookOpen class="h-4 w-4" />
            API Docs
          </router-link>
        </Button>
      </div>
    </div>

    <div v-if="keyName || settings?.is_default" class="flex items-center gap-2 flex-wrap text-sm">
      <p v-if="keyName" class="text-muted-foreground">
        {{ keyName }} <span class="font-mono">({{ keyPrefix }}...)</span>
      </p>
      <span
        v-if="settings?.is_default"
        class="text-xs bg-secondary text-secondary-foreground px-2 py-0.5 rounded"
      >
        Using Global Settings
      </span>
    </div>

    <div v-if="settingsLoading" class="text-sm text-muted-foreground">Loading settings...</div>
    <div v-else-if="initError" class="flex items-center gap-3 flex-wrap">
      <span class="text-sm text-destructive">{{ initError }}</span>
      <Button variant="outline" size="sm" data-testid="retry-load-button" @click="loadInitial">
        <Loader2 v-if="settingsLoading" class="size-4 animate-spin mr-1" />
        Retry
      </Button>
    </div>

    <RenderSettingsForm
      v-else-if="settings"
      ref="formRef"
      :settings="settings"
      uid="self"
      :show-actions="false"
      :load-settings="loadSettings"
      :save-settings="saveSettings"
      :fetch-preview="selfApi.preview"
                                />
  </div>
</template>
