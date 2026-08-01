<script setup lang="ts">
import { ref, watch } from 'vue'
import { Check, Loader2, Download, Upload } from 'lucide-vue-next'
import { useQuery } from '@tanstack/vue-query'
import { adminApi, type SaveSettingsPayload } from '@/lib/api'
import { FREE_API_KEY } from '@/lib/constants'
import RefreshButton from '@/components/RefreshButton.vue'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import ClearCacheButton from '@/components/ClearCacheButton.vue'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import type { RenderSettings } from '@/components/RenderSettingsForm.vue'

type SettingsResponse = RenderSettings & { free_api_key_enabled: boolean; free_api_key_locked: boolean }

type ServiceKey = { locked: boolean; has_key: boolean; masked: string | null }
type ServiceKeysResponse = { tmdb: ServiceKey; mdblist: ServiceKey; omdb: ServiceKey; fanart: ServiceKey; trakt: ServiceKey }

const freeApiKeyEnabled = ref(false)
const freeKeyLoading = ref(false)
const freeKeyError = ref('')
const cacheMessage = ref('')

const serviceKeysSaving = ref<string | null>(null)
const serviceKeysError = ref('')

const serviceKeyInputs = ref({
  tmdb: '',
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
  tmdb: 'TMDB',
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
  const res = await adminApi.updateSettings({
    ...s,
    free_api_key_enabled: freeApiKeyEnabled.value,
  })
  if (res.ok) return null
  const data = await res.json().catch(() => null)
  return data?.error || 'Failed to save settings'
}

function toggleFreeApiKey() {
  if (!settings.value || settings.value.free_api_key_locked) return
  freeApiKeyEnabled.value = !freeApiKeyEnabled.value
}

const formRef = ref<{
  save: () => Promise<void>
  discard: () => void
  dirty: boolean
  saving: boolean
  showCheck: boolean
  error: string
} | null>(null)

// --- Backup & Restore ---
const exportDialogOpen = ref(false)
const exportServiceKeys = ref(true)
const exportAPIKeys = ref(true)
const exportLoading = ref(false)
const exportError = ref('')

const importBusy = ref(false)
const importError = ref('')
const importFileInput = ref<HTMLInputElement | null>(null)
type ImportResult = {
  restored_settings?: number
  restored_keys?: number
  regenerated_keys?: { name: string; key: string; key_prefix: string }[]
}
const importResult = ref<ImportResult | null>(null)

async function runExport() {
  exportLoading.value = true
  exportError.value = ''
  try {
    const res = await adminApi.exportSettings(exportServiceKeys.value, exportAPIKeys.value)
    if (!res.ok) {
      const data = await res.json().catch(() => null)
      exportError.value = data?.error || 'Export failed'
      return
    }
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `openposterdb-settings-${new Date().toISOString().slice(0, 10)}.json`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    exportDialogOpen.value = false
  } catch {
    exportError.value = 'Export failed'
  } finally {
    exportLoading.value = false
  }
}

async function onImportFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  importBusy.value = true
  importError.value = ''
  importResult.value = null
  try {
    const payload = JSON.parse(await file.text())
    const res = await adminApi.importSettings(payload)
    const data = await res.json().catch(() => null)
    if (!res.ok) {
      importError.value = data?.error || 'Import failed'
      return
    }
    importResult.value = data
    await refetch()
  } catch {
    importError.value = 'Import failed — not a valid settings file'
  } finally {
    importBusy.value = false
  }
}
</script>

<template>
  <div class="space-y-8">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold">Settings</h1>
      <div class="flex items-center gap-2">
        <Button
          v-if="formRef?.dirty"
          variant="outline"
          size="sm"
          data-testid="discard-settings-button"
          @click="formRef?.discard()"
        >
          Discard changes
        </Button>
        <span v-if="formRef?.showCheck" class="flex items-center gap-1.5 text-sm text-green-500">
          <Check class="size-4" />
          Saved
        </span>
        <Button size="sm" data-testid="save-settings-button" @click="formRef?.save()">
          <Loader2 v-if="formRef?.saving" class="size-4 animate-spin mr-1" />
          Save
        </Button>
        <span v-if="formRef?.error" class="text-sm text-destructive">{{ formRef.error }}</span>
        <RefreshButton :fetching="isFetching" @refresh="refetch()" />
      </div>
    </div>

    <div class="max-w-3xl space-y-6">
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

        <div v-for="service in (['tmdb', 'mdblist', 'omdb', 'fanart', 'trakt'] as const)" :key="service" class="space-y-2">
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
          ref="formRef"
          :settings="settings"
          uid="global"
          :show-actions="false"
          :load-settings="loadSettings"
          :save-settings="saveSettings"
          :fetch-preview="adminApi.previewPoster"
          :fetch-logo-preview="adminApi.previewLogo"
          :fetch-backdrop-preview="adminApi.previewBackdrop"
          :fetch-episode-preview="adminApi.previewEpisode"
        />
      </div>

      <div class="rounded-lg border p-6 space-y-4">
        <h2 class="text-lg font-semibold">Backup &amp; Restore</h2>
        <p class="text-sm text-muted-foreground">
          Export the current settings to a file, or restore them from a previous export.
          API keys can optionally be included.
        </p>

        <div class="flex flex-wrap items-center gap-3">
          <Dialog v-model:open="exportDialogOpen">
            <DialogTrigger as-child>
              <Button variant="outline" size="sm">
                <Download class="size-4 mr-1" />
                Export settings
              </Button>
            </DialogTrigger>
            <DialogContent class="sm:max-w-md">
              <DialogHeader>
                <DialogTitle>Export settings</DialogTitle>
                <DialogDescription>
                  Choose what to include in the export file.
                </DialogDescription>
              </DialogHeader>
              <div class="space-y-3 py-2">
                <label class="flex items-center gap-2 cursor-pointer">
                  <Checkbox
                    :model-value="exportServiceKeys"
                    @update:model-value="(v: unknown) => (exportServiceKeys = !!v)"
                  />
                  <span class="text-sm">
                    External API keys
                    <span class="text-muted-foreground text-xs">(TMDB, MDBList, OMDb, Fanart.tv, Trakt)</span>
                  </span>
                </label>
                <label class="flex items-center gap-2 cursor-pointer">
                  <Checkbox
                    :model-value="exportAPIKeys"
                    @update:model-value="(v: unknown) => (exportAPIKeys = !!v)"
                  />
                  <span class="text-sm">
                    API keys
                    <span class="text-muted-foreground text-xs">
                      (poster-serving keys + their settings — existing key values can't be
                      recovered, so missing keys are recreated on import)
                    </span>
                  </span>
                </label>
                <p v-if="exportError" class="text-sm text-destructive">{{ exportError }}</p>
              </div>
              <DialogFooter>
                <DialogClose as-child>
                  <Button variant="outline" size="sm">Cancel</Button>
                </DialogClose>
                <Button size="sm" :disabled="exportLoading" @click="runExport">
                  <Loader2 v-if="exportLoading" class="size-4 animate-spin mr-1" />
                  Export
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" :disabled="importBusy" @click="importFileInput?.click()">
            <Upload class="size-4 mr-1" />
            {{ importBusy ? 'Importing...' : 'Import settings' }}
          </Button>
          <input ref="importFileInput" type="file" accept="application/json,.json" class="hidden" @change="onImportFile" />
        </div>

        <p v-if="importError" class="text-sm text-destructive">{{ importError }}</p>
        <div v-if="importResult" class="space-y-2 text-sm">
          <p class="text-muted-foreground">
            Import complete: {{ importResult.restored_settings ?? 0 }} settings and
            {{ importResult.restored_keys ?? 0 }} service keys restored.
          </p>
          <template v-if="importResult.regenerated_keys?.length">
            <p class="font-medium">Newly created API keys (values are shown once — save them now):</p>
            <div v-for="k in importResult.regenerated_keys" :key="k.name" class="rounded border bg-muted px-3 py-2 font-mono text-xs">
              <div class="font-semibold">{{ k.name }}</div>
              <div>{{ k.key }}</div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
