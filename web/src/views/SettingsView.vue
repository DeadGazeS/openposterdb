<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Check, Loader2, Download, Upload } from 'lucide-vue-next'
import { useQuery } from '@tanstack/vue-query'
import { useSavedFlash } from '@/composables/useSavedFlash'
import { parseApiError } from '@/lib/api-error'
import { adminApi } from '@/lib/api'
import type { SaveSettingsPayload } from '@/lib/settings'
import { FREE_API_KEY } from '@/lib/constants'
import RefreshButton from '@/components/RefreshButton.vue'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import ApiKeysView from '@/views/ApiKeysView.vue'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
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
import type { RenderSettings } from '@/lib/settings'

const route = useRoute()

// The active section is derived from the route — each section is its own page
// (Settings → API / Global Image / Backup).
const section = computed(() => {
  switch (route.name) {
    case 'settings-backup':
      return 'backup'
    case 'settings-image':
      return 'image'
    default:
      return 'api'
  }
})

type SettingsResponse = RenderSettings & { free_api_key_enabled: boolean; free_api_key_locked: boolean }

type ServiceKey = { locked: boolean; has_key: boolean; masked: string | null; keys?: string[] }
type ServiceKeysResponse = { tmdb: ServiceKey; mdblist: ServiceKey; omdb: ServiceKey; fanart: ServiceKey; trakt: ServiceKey }

const freeApiKeyEnabled = ref(false)
const freeKeyError = ref('')

const serviceKeysSaving = ref<string | null>(null)
const serviceKeysError = ref('')

// Per-service key chips. Each chip is one key; the whole list is saved
// (comma-joined, matching the backend's multi-key format) whenever a chip is
// added or removed — there is no separate Save button.
const serviceKeyChips = ref<Record<string, string[]>>({
  tmdb: [],
  mdblist: [],
  omdb: [],
  fanart: [],
  trakt: [],
})
const serviceKeyDraft = ref<Record<string, string>>({
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

// Seed the chips from the server's real keys once loaded.
watch(
  serviceKeys,
  (keys) => {
    if (!keys) return
    for (const svc of ['tmdb', 'mdblist', 'omdb', 'fanart', 'trakt'] as const) {
      const k = keys[svc]
      if (k && !k.locked) serviceKeyChips.value[svc] = [...(k.keys ?? [])]
    }
  },
  { immediate: true },
)

function splitServiceKeys(s: string): string[] {
  return s
    .split(/[\s,]+/)
    .map((k) => k.trim())
    .filter(Boolean)
}

// maskKey mirrors the backend MaskKey: first len/4 (max 4) chars + "..." +
// last len/4 chars, always hiding at least half of the key.
function maskKey(key: string): string {
  let n = Math.floor(key.length / 4)
  if (n > 4) n = 4
  if (n === 0) return '****'
  return key.slice(0, n) + '...' + key.slice(-n)
}

// Keys the user has chosen to reveal (per service+key); everything else stays
// masked until clicked.
const revealedKeys = ref<Set<string>>(new Set())

function isRevealed(svc: string, key: string): boolean {
  return revealedKeys.value.has(`${svc}:${key}`)
}

function toggleReveal(svc: string, key: string) {
  const id = `${svc}:${key}`
  const next = new Set(revealedKeys.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  revealedKeys.value = next
}

// Add the draft's keys as chips (Enter / comma / space all work), then
// auto-save the resulting list.
function commitServiceKeyDraft(svc: string) {
  const draft = serviceKeyDraft.value[svc]
  if (!draft) return
  const add = splitServiceKeys(draft)
  if (add.length === 0) return
  const merged = [...(serviceKeyChips.value[svc] ?? [])]
  for (const k of add) {
    if (!merged.includes(k)) merged.push(k)
  }
  serviceKeyChips.value[svc] = merged
  serviceKeyDraft.value[svc] = ''
  void saveServiceKeys(svc)
}

// Split on comma/space while typing, so "key1, key2" becomes two chips.
function onServiceKeyDraftInput(svc: string) {
  const draft = serviceKeyDraft.value[svc]
  if (draft && /[\s,]/.test(draft)) {
    const add = splitServiceKeys(draft)
    const merged = [...(serviceKeyChips.value[svc] ?? [])]
    for (const k of add) {
      if (!merged.includes(k)) merged.push(k)
    }
    serviceKeyChips.value[svc] = merged
    serviceKeyDraft.value[svc] = ''
    void saveServiceKeys(svc)
  }
}

function removeServiceKey(svc: string, key: string) {
  serviceKeyChips.value[svc] = (serviceKeyChips.value[svc] ?? []).filter((k) => k !== key)
  void saveServiceKeys(svc)
}

async function saveServiceKeys(svc: string) {
  if (serviceKeysSaving.value) return
  serviceKeysSaving.value = svc
  serviceKeysError.value = ''
  const value = (serviceKeyChips.value[svc] ?? []).join(',')
  const res = await adminApi.updateServiceKeys({ [svc]: value })
  if (res.ok) {
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
  refetch,
} = useQuery<SettingsResponse>({
  queryKey: ['global-settings'],
  // Disable structural sharing so a Refresh always delivers a fresh object
  // reference. With sharing enabled, refetching identical server data keeps
  // the same reference, the form's settings watcher never fires, and unsaved
  // edits survive Refresh. A fresh reference re-applies the saved config
  // (same path as discard, plus pulling the latest save).
  structuralSharing: false,
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

// The API Keys card's exposed per-key actions, driven from the page header so
// Save / Discard / Refresh cover both the global settings and the expanded
// per-key settings form.
const apiKeysRef = ref<{
  saveExpanded: () => Promise<void> | undefined
  discardExpanded: () => void
  refreshExpanded: () => Promise<void>
  expandedDirty: boolean
} | null>(null)

const anyDirty = computed(() => {
  if (section.value === 'image') return !!formRef.value?.dirty
  return !!apiKeysRef.value?.expandedDirty || freeKeyDirty.value
})

// The free-key toggle is a global setting saved via a minimal partial PUT
// (the backend keeps omitted fields) — the full settings form only lives on
// the Image page.
const freeKeyDirty = computed(
  () => !!settings.value && freeApiKeyEnabled.value !== settings.value.free_api_key_enabled,
)

const { active: apiSavedCheck, flash: showApiSaved } = useSavedFlash()

async function saveFreeApiKeyToggle(): Promise<void> {
  if (!settings.value) return
  const res = await adminApi.updateSettings({
    image_source: settings.value.image_source,
    free_api_key_enabled: freeApiKeyEnabled.value,
  })
  if (res.ok) {
    await refetch()
    showApiSaved()
  } else {
    const data = await res.json().catch(() => null)
    freeKeyError.value = data?.error || 'Failed to save'
  }
}

// True only while a user-initiated Refresh click is in flight. The query's
// own isFetching also fires on initial load / background refetches, which
// would make the Refresh button spin without the user asking for it.
const userRefreshing = ref(false)

async function handleSave() {
  if (section.value === 'api') {
    // Minimal partial PUT: the backend keeps omitted fields, so only the
    // toggle (and the key the API is tied to) is sent.
    await saveFreeApiKeyToggle()
  } else {
    await formRef.value?.save()
  }
  await apiKeysRef.value?.saveExpanded()
}

function handleDiscard() {
  if (section.value === 'api') {
    if (settings.value) freeApiKeyEnabled.value = settings.value.free_api_key_enabled
    apiKeysRef.value?.discardExpanded()
  } else {
    formRef.value?.discard()
  }
}

async function handleRefresh() {
  userRefreshing.value = true
  try {
    await refetch()
    await apiKeysRef.value?.refreshExpanded()
  } finally {
    userRefreshing.value = false
  }
}

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
  <div class="space-y-4">
    <!-- API -->
    <div v-if="section === 'api'" class="rounded-lg border p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="w-fit border border-t-0 border-l-0 rounded-tl-md rounded-br-md bg-muted px-4 py-2 text-sm font-bold uppercase tracking-widest">
          API
        </h3>
        <div class="flex items-center gap-2">
          <Button
            v-if="anyDirty"
            variant="outline"
            size="sm"
            data-testid="discard-settings-button"
            @click="handleDiscard"
        >
            Discard changes
          </Button>
          <span v-if="apiSavedCheck" class="flex items-center gap-1.5 text-sm text-green-500">
            <Check class="size-4" />
            Saved
          </span>
          <Button size="sm" data-testid="save-settings-button" @click="handleSave">
            Save
          </Button>
          <RefreshButton :fetching="userRefreshing" @refresh="handleRefresh" />
        </div>
      </div>
      <p class="text-sm text-muted-foreground">
        Free API key, poster-serving API keys, and keys for external rating and image providers.
      </p>

      <Tabs default-value="free-api-key" :unmount-on-hide="false">
        <TabsList class="h-auto flex-wrap">
          <TabsTrigger value="free-api-key">Free API Key</TabsTrigger>
          <TabsTrigger value="api-keys">API Keys</TabsTrigger>
          <TabsTrigger value="source-api-keys">Source API Keys</TabsTrigger>
        </TabsList>

        <!-- Free API Key -->
        <TabsContent value="free-api-key" class="mt-3">
          <div class="rounded-md border p-4 space-y-3">
            <p class="text-sm font-semibold">Free API Key</p>
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
                :disabled="!settings || settings?.free_api_key_locked"
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
        </TabsContent>

        <!-- API Keys -->
        <TabsContent value="api-keys" class="mt-3">
          <div class="rounded-md border p-4 space-y-3">
            <p class="text-sm font-semibold">API Keys</p>
            <ApiKeysView ref="apiKeysRef" />
          </div>
        </TabsContent>

        <!-- Source API Keys -->
        <TabsContent value="source-api-keys" class="mt-3">
          <div class="rounded-md border p-4 space-y-3">
            <p class="text-sm font-semibold">Source API Keys</p>
            <p class="text-sm text-muted-foreground">
              Keys for external rating and image providers. Keys set via environment
              variables are locked and cannot be changed here. Each key is its own entry —
              press Enter (or type a comma/space) to add one, use the × to remove it.
              Changes are saved automatically.
            </p>
            <p v-if="serviceKeysError" class="text-sm text-destructive">{{ serviceKeysError }}</p>

            <div v-for="service in (['tmdb', 'mdblist', 'omdb', 'fanart', 'trakt'] as const)" :key="service" class="space-y-2">
              <label :for="`key-${service}`" class="text-sm font-medium flex items-center gap-2">
                {{ serviceLabels[service] }}
                <span v-if="serviceKeys?.[service]?.locked" class="text-xs text-muted-foreground bg-muted px-1.5 py-0.5 rounded">env</span>
                <span v-if="serviceKeysSaving === service" class="text-xs text-muted-foreground">Saving…</span>
              </label>
              <div v-if="serviceKeys?.[service]?.locked" class="text-sm text-muted-foreground font-mono">
                {{ serviceKeys?.[service]?.masked }}
              </div>
              <div v-else class="flex flex-wrap items-center gap-1.5 rounded-md border border-input bg-transparent px-2 py-1.5 min-h-9">
                <span
                  v-for="key in serviceKeyChips[service]"
                  :key="key"
                  class="inline-flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-xs font-mono"
                >
                  <button
                    type="button"
                    class="hover:underline"
                    :title="isRevealed(service, key) ? 'Hide key' : 'Reveal key'"
                    @click="toggleReveal(service, key)"
                  >{{ isRevealed(service, key) ? key : maskKey(key) }}</button>
                  <button
                    type="button"
                    class="hover:text-destructive disabled:opacity-50"
                    :aria-label="`Remove ${serviceLabels[service]} key`"
                    :disabled="serviceKeysSaving !== null"
                    @click="removeServiceKey(service, key)"
                  >×</button>
                </span>
                <input
                  :id="`key-${service}`"
                  v-model="serviceKeyDraft[service]"
                  type="text"
                  class="flex-1 min-w-40 bg-transparent text-sm outline-none"
                  :placeholder="(serviceKeyChips[service] ?? []).length ? 'Add another key…' : 'Enter key…'"
                  @keydown.enter.prevent="commitServiceKeyDraft(service)"
                  @input="onServiceKeyDraftInput(service)"
                />
              </div>
            </div>
          </div>
        </TabsContent>
      </Tabs>
    </div>

    <!-- Backup -->
    <div v-if="section === 'backup'" class="rounded-lg border p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="w-fit border border-t-0 border-l-0 rounded-tl-md rounded-br-md bg-muted px-4 py-2 text-sm font-bold uppercase tracking-widest">
          Backup &amp; Restore
        </h3>
        <RefreshButton :fetching="userRefreshing" @refresh="handleRefresh" />
      </div>
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

    <!-- Image -->
    <div v-if="section === 'image'" class="rounded-lg border p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="w-fit border border-t-0 border-l-0 rounded-tl-md rounded-br-md bg-muted px-4 py-2 text-sm font-bold uppercase tracking-widest">
          Global Image Settings
        </h3>
        <div class="flex items-center gap-2">
          <Button
            v-if="formRef?.dirty"
            variant="outline"
            size="sm"
            data-testid="discard-settings-button"
            @click="handleDiscard"
        >
            Discard changes
          </Button>
          <span v-if="formRef?.showCheck" class="flex items-center gap-1.5 text-sm text-green-500">
            <Check class="size-4" />
            Saved
          </span>
          <Button size="sm" data-testid="save-settings-button" @click="handleSave">
            <Loader2 v-if="formRef?.saving" class="size-4 animate-spin mr-1" />
            Save
          </Button>
          <span v-if="formRef?.error" class="text-sm text-destructive">{{ formRef.error }}</span>
          <RefreshButton :fetching="userRefreshing" @refresh="handleRefresh" />
        </div>
      </div>
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
        :fetch-preview="adminApi.preview"
                              />
    </div>
  </div>
</template>
