<script setup lang="ts">
import { ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { adminApi, type SaveSettingsPayload } from '@/lib/api'
import { FREE_API_KEY } from '@/lib/constants'
import RefreshButton from '@/components/RefreshButton.vue'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import ClearCacheButton from '@/components/ClearCacheButton.vue'
import type { RenderSettings } from '@/components/RenderSettingsForm.vue'

type SettingsResponse = RenderSettings & { free_api_key_enabled: boolean; free_api_key_locked: boolean }

type ServiceKey = { locked: boolean; has_key: boolean; masked: string | null }
type ServiceKeysResponse = { mdblist: ServiceKey; omdb: ServiceKey; fanart: ServiceKey; trakt: ServiceKey }

const freeApiKeyEnabled = ref(false)
const freeKeyLoading = ref(false)
const freeKeyError = ref('')
const cacheMessage = ref('')

const serviceKeysSaving = ref<string | null>(null)
const serviceKeysError = ref('')

const serviceKeyInputs = ref({
  mdblist: '',
  omdb: '',
  fanart: '',
  trakt: '',
})

const {
  data: serviceKeys,
  refetch: refetchServiceKeys,
} = useQuery<ServiceKeysResponse>({
  queryKey: ['service-keys'],
  queryFn: async () => {
    const res = await adminApi.getServiceKeys()
    if (!res.ok) throw new Error('Failed to fetch service keys')
    return res.json()
  },
})

async function saveServiceKey(service: string) {
  if (serviceKeysSaving.value) return
  serviceKeysSaving.value = service
  serviceKeysError.value = ''
  const value = serviceKeyInputs.value[service as keyof typeof serviceKeyInputs.value]
  const res = await adminApi.updateServiceKeys({ [service]: value || '' })
  if (res.ok) {
    serviceKeyInputs.value[service as keyof typeof serviceKeyInputs.value] = service === 'mdblist' && !value ? '' : ''
    await refetchServiceKeys()
  } else {
    const data = await res.json().catch(() => null)
    serviceKeysError.value = data?.error || 'Failed to save'
  }
  serviceKeysSaving.value = null
}

const serviceLabels: Record<string, string> = {
  mdblist: 'MDBList',
  omdb: 'OMDb',
  fanart: 'Fanart.tv',
  trakt: 'Trakt',
}

const {
  data: settings,
  isFetching,
  refetch,
} = useQuery<SettingsResponse>({
  queryKey: ['global-settings'],
  queryFn: async () => {
    const res = await adminApi.getSettings()
    if (!res.ok) throw new Error('Failed to fetch settings')
    return res.json()
  },
})

watch(settings, (s) => {
  if (s) freeApiKeyEnabled.value = s.free_api_key_enabled
}, { immediate: true })

async function loadSettings(): Promise<RenderSettings | null> {
  const res = await adminApi.getSettings()
  if (!res.ok) return null
  return res.json()
}

async function saveSettings(s: SaveSettingsPayload): Promise<string | null> {
  const res = await adminApi.updateSettings(s)
  if (res.ok) return null
  const data = await res.json().catch(() => null)
  return data?.error || 'Failed to save settings'
}

async function toggleFreeApiKey() {
  if (!settings.value) return
  freeKeyLoading.value = true
  freeKeyError.value = ''
  const newVal = !freeApiKeyEnabled.value
  // Forward the full settings unchanged (admin update is a full replace where every
  // omitted field is written back as its serde default). Spreading guards against
  // silently resetting any field not explicitly listed — e.g. backdrop position,
  // direction, and edge insets. The read-only free_api_key_locked field is ignored
  // by the backend.
  const res = await adminApi.updateSettings({
    ...settings.value,
    free_api_key_enabled: newVal,
  })
  if (res.ok) {
    freeApiKeyEnabled.value = newVal
  } else {
    const data = await res.json().catch(() => null)
    freeKeyError.value = data?.error || 'Failed to save'
  }
  freeKeyLoading.value = false
}
</script>

<template>
  <div class="space-y-8">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold">Settings</h1>
      <RefreshButton :fetching="isFetching" @refresh="refetch()" />
    </div>

    <div class="max-w-lg space-y-6">
      <div class="rounded-lg border p-6 space-y-4">
        <h2 class="text-lg font-semibold">Free API Key</h2>
        <p class="text-sm text-muted-foreground">
          When enabled, the key <code class="font-mono text-xs bg-muted px-1 py-0.5 rounded">{{ FREE_API_KEY }}</code>
          can be used for poster serving with global default settings.
          It does not grant access to self-service features.
        </p>
        <label class="flex items-center gap-3 cursor-pointer">
          <button
            type="button"
            role="switch"
            :aria-checked="freeApiKeyEnabled"
            :disabled="freeKeyLoading || !settings || settings?.free_api_key_locked"
            class="relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
            :class="freeApiKeyEnabled ? 'bg-primary' : 'bg-input'"
            @click="toggleFreeApiKey"
          >
            <span
              class="pointer-events-none block h-4 w-4 rounded-full bg-background shadow-lg ring-0 transition-transform"
              :class="freeApiKeyEnabled ? 'translate-x-4' : 'translate-x-0'"
            />
          </button>
          <span class="text-sm font-medium">{{ freeApiKeyEnabled ? 'Enabled' : 'Disabled' }}</span>
          <span v-if="freeKeyError" class="text-sm text-destructive">{{ freeKeyError }}</span>
        </label>
        <p v-if="settings?.free_api_key_locked" class="text-sm text-muted-foreground">
          Controlled by <code class="font-mono text-xs bg-muted px-1 py-0.5 rounded">FREE_KEY_ENABLED</code> environment variable.
        </p>
      </div>

      <div class="rounded-lg border p-6 space-y-4">
        <h2 class="text-lg font-semibold">External API Keys</h2>
        <p class="text-sm text-muted-foreground">
          API keys for external rating and image providers. Keys set via environment
          variables are locked and cannot be changed here. After saving, the running
          service is updated without a restart.
        </p>
        <p v-if="serviceKeysError" class="text-sm text-destructive">{{ serviceKeysError }}</p>

        <div v-for="service in (['mdblist', 'omdb', 'fanart', 'trakt'] as const)" :key="service" class="space-y-2">
          <label :for="`key-${service}`" class="text-sm font-medium flex items-center gap-2">
            {{ serviceLabels[service] }}
            <span v-if="serviceKeys?.[service]?.locked" class="text-xs text-muted-foreground bg-muted px-1.5 py-0.5 rounded">env</span>
          </label>
          <div v-if="serviceKeys?.[service]?.locked && serviceKeys?.[service]?.masked" class="text-sm text-muted-foreground font-mono">
            {{ serviceKeys?.[service]?.masked }}
          </div>
          <div v-else class="flex gap-2">
            <input
              :id="`key-${service}`"
              v-model="serviceKeyInputs[service]"
              type="text"
              class="flex-1 h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs"
              :placeholder="serviceKeys?.[service]?.has_key ? serviceKeys?.[service]?.masked ?? '' : 'Enter key...'"
            />
            <button
              type="button"
              class="inline-flex items-center justify-center rounded-md h-9 px-4 text-sm font-medium bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              :disabled="serviceKeysSaving !== null"
              @click="saveServiceKey(service)"
            >
              {{ serviceKeysSaving === service ? 'Saving...' : 'Save' }}
            </button>
          </div>
        </div>
      </div>

      <div class="rounded-lg border p-6 space-y-4">
        <h2 class="text-lg font-semibold">Cache</h2>
        <p class="text-sm text-muted-foreground">
          Clear all cached images (posters, logos, backdrops, episodes). They are
          regenerated on the next request, so the first load of each title is slower.
        </p>
        <ClearCacheButton @cleared="(m: string) => (cacheMessage = m)" />
        <p v-if="cacheMessage" class="text-sm text-muted-foreground">{{ cacheMessage }}</p>
      </div>

      <div class="rounded-lg border p-6 space-y-4">
        <h2 class="text-lg font-semibold">Global Image Settings</h2>
        <p class="text-sm text-muted-foreground">
          These defaults apply to all API keys unless overridden per-key.
        </p>

        <RenderSettingsForm
          v-if="settings"
          :settings="settings"
          uid="global"
          :load-settings="loadSettings"
          :save-settings="saveSettings"
          :fetch-preview="adminApi.previewPoster"
          :fetch-logo-preview="adminApi.previewLogo"
          :fetch-backdrop-preview="adminApi.previewBackdrop"
          :fetch-episode-preview="adminApi.previewEpisode"
        />
      </div>
    </div>
  </div>
</template>
