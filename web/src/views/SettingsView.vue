<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Check, Loader2 } from 'lucide-vue-next'
import { useQuery } from '@tanstack/vue-query'
import { useSavedFlash } from '@/composables/useSavedFlash'
import { useRenderSettingsForm } from '@/composables/useRenderSettingsForm'
import { okOrThrow } from '@/lib/api-error'
import { adminApi } from '@/lib/api'
import type { RenderSettings, SaveSettingsPayload } from '@/lib/settings'
import { maskKey } from '@/lib/utils'
import { FREE_API_KEY } from '@/lib/constants'
import RefreshButton from '@/components/RefreshButton.vue'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import ApiKeysView from '@/views/ApiKeysView.vue'
import GeneralSettings from '@/components/GeneralSettings.vue'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const route = useRoute()
const router = useRouter()

// The active section is derived from the route — each section is its own page
// (Settings → API / Global Image Settings / General). Each has sub-tabs
// driven by `?tab=`; Backup is a tab of General.
const section = computed(() => {
  switch (route.name) {
    case 'settings-general':
      return 'general'
    case 'settings-image':
      return 'image'
    default:
      return 'api'
  }
})

// Tab state for the API route. The Settings page has 3 tabs (Free API Key /
// API Keys / Source API Keys); the underlying reka-ui Tabs is uncontrolled by
// default, which means the active tab resets to the default on every reload.
// Persist via the `?tab=` query param so reloads land on the same tab the
// user was last viewing (matches the back/forward-button expectation). The
// query param is always present in the URL — even for the default tab — so a
// hard reload, a shared link, or an open-in-new-tab action always lands on
// the right view.
type ApiTab = 'free-api-key' | 'api-keys' | 'source-api-keys'
const validApiTabs: ApiTab[] = ['free-api-key', 'api-keys', 'source-api-keys']
const apiTab = computed<ApiTab>({
  get() {
    const q = route.query.tab
    const v = Array.isArray(q) ? q[0] : q
    return (validApiTabs as string[]).includes(v ?? '') ? (v as ApiTab) : 'free-api-key'
  },
  set(v) {
    void router.replace({ query: { ...route.query, tab: v } })
  },
})

type SettingsResponse = RenderSettings & { free_api_key_enabled: boolean; free_api_key_locked: boolean }

// The server lists stored keys only as {id, masked}; plaintext is fetched
// per key on reveal/copy, and kept keys are sent back as "ref:<id>".
type ServiceKeyRef = { id: string; masked: string }
type ServiceKey = { locked: boolean; has_key: boolean; masked: string | null; keys?: ServiceKeyRef[] }
// A chip is either a stored key (id) or a newly typed one (value, unsaved).
type ServiceKeyChip = { id?: string; masked: string; value?: string }
function chipKey(c: ServiceKeyChip): string {
  return c.id ? `id:${c.id}` : `new:${c.value}`
}
type ServiceKeysResponse = { tmdb: ServiceKey; mdblist: ServiceKey; omdb: ServiceKey; fanart: ServiceKey; trakt: ServiceKey }

const freeApiKeyEnabled = ref(false)
const freeKeyError = ref('')

const serviceKeysSaving = ref<string | null>(null)
const serviceKeysError = ref('')

// Per-chip Copy feedback ('idle' | 'copied'). Keyed by `${svc}:${key}` so
// each chip toggles its own label briefly when clicked. Only revealed chips
// render the button, but the state stays a flat record for simplicity.
const serviceCopyState = ref<Record<string, 'idle' | 'copied'>>({})
async function copyServiceKey(svc: string, chip: ServiceKeyChip) {
  const key = await chipValue(svc, chip)
  if (!key) return
  const id = `${svc}:${chipKey(chip)}`
  forgetRevealed(id)
  try {
    await navigator.clipboard.writeText(key)
    serviceCopyState.value[id] = 'copied'
    setTimeout(() => {
      serviceCopyState.value = { ...serviceCopyState.value, [id]: 'idle' }
    }, 1500)
  } catch {
    const el = document.createElement('textarea')
    el.value = key
    el.style.position = 'fixed'
    el.style.opacity = '0'
    document.body.appendChild(el)
    el.select()
    try {
      document.execCommand('copy')
      serviceCopyState.value[id] = 'copied'
      setTimeout(() => {
        serviceCopyState.value = { ...serviceCopyState.value, [id]: 'idle' }
      }, 1500)
    } finally {
      document.body.removeChild(el)
    }
  }
}

// Per-service key chips. Each chip is one key; the whole list is saved
// (comma-joined, matching the backend's multi-key format) whenever a chip is
// added or removed — there is no separate Save button.
const serviceKeyChips = ref<Record<string, ServiceKeyChip[]>>({
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
  queryFn: async () => okOrThrow<ServiceKeysResponse>(await adminApi.getServiceKeys(), 'Failed to fetch service keys'),
})

// Seed the chips from the server's real keys once loaded.
watch(
  serviceKeys,
  (keys) => {
    if (!keys) return
    for (const svc of ['tmdb', 'mdblist', 'omdb', 'fanart', 'trakt'] as const) {
      const k = keys[svc]
      if (pendingServiceSaves.has(svc)) continue // unsaved chips: keep them
      if (k && !k.locked) serviceKeyChips.value[svc] = (k.keys ?? []).map((r) => ({ id: r.id, masked: r.masked }))
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

// maskKey is imported from @/lib/utils — shared with ApiKeysView so both
// surfaces obfuscate identically. Mirror of the backend MaskKey at
// api-go/internal/services/servicekeys.go: first len/4 (max 4) chars + "..." +
// last len/4 chars, always hiding at least half of the key.

// Keys the user has chosen to reveal (per service+chip); everything else stays
// masked. Stored keys' plaintext is fetched from the server on first reveal.
const revealedKeys = ref<Set<string>>(new Set())
const revealedValues = ref<Record<string, string>>({})

function isRevealed(svc: string, chip: ServiceKeyChip): boolean {
  return revealedKeys.value.has(`${svc}:${chipKey(chip)}`)
}

async function chipValue(svc: string, chip: ServiceKeyChip): Promise<string | null> {
  if (chip.value) return chip.value
  const id = `${svc}:${chipKey(chip)}`
  if (revealedValues.value[id]) return revealedValues.value[id]!
  try {
    const res = await adminApi.revealServiceKey(svc, chip.id!)
    if (!res.ok) {
      serviceKeysError.value = 'Could not reveal the key'
      return null
    }
    const data = (await res.json()) as { key: string }
    revealedValues.value = { ...revealedValues.value, [id]: data.key }
    return data.key
  } catch {
    serviceKeysError.value = 'Could not reveal the key'
    return null
  }
}

function displayedKey(svc: string, chip: ServiceKeyChip): string {
  if (!isRevealed(svc, chip)) return chip.masked
  return chip.value ?? revealedValues.value[`${svc}:${chipKey(chip)}`] ?? chip.masked
}

async function toggleReveal(svc: string, chip: ServiceKeyChip) {
  const id = `${svc}:${chipKey(chip)}`
  const next = new Set(revealedKeys.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    if (!(await chipValue(svc, chip))) return
    next.add(id)
  }
  revealedKeys.value = next
  forgetRevealed(id)
}

// Drops a revealed plaintext once it isn't shown (it was kept until the view
// unmounted, removed chips included).
function forgetRevealed(id: string) {
  if (revealedKeys.value.has(id) || !(id in revealedValues.value)) return
  const rest = { ...revealedValues.value }
  delete rest[id]
  revealedValues.value = rest
}

function newChip(value: string): ServiceKeyChip {
  return { value, masked: maskKey(value) }
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
    if (!merged.some((c) => c.value === k)) merged.push(newChip(k))
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
      if (!merged.some((c) => c.value === k)) merged.push(newChip(k))
    }
    serviceKeyChips.value[svc] = merged
    serviceKeyDraft.value[svc] = ''
    void saveServiceKeys(svc)
  }
}

function removeServiceKey(svc: string, chip: ServiceKeyChip) {
  const id = `${svc}:${chipKey(chip)}`
  if (revealedKeys.value.has(id)) {
    const next = new Set(revealedKeys.value)
    next.delete(id)
    revealedKeys.value = next
  }
  forgetRevealed(id)
  serviceKeyChips.value[svc] = (serviceKeyChips.value[svc] ?? []).filter((c) => chipKey(c) !== chipKey(chip))
  void saveServiceKeys(svc)
}

// Services whose chips changed while another save was running: saved next,
// and not re-seeded from the server meanwhile (that dropped the new chips).
const pendingServiceSaves = new Set<string>()

async function saveServiceKeys(svc: string) {
  if (serviceKeysSaving.value) {
    pendingServiceSaves.add(svc)
    return
  }
  serviceKeysSaving.value = svc
  serviceKeysError.value = ''
  try {
    // Stored keys go back as references; only newly typed keys travel as text.
    const value = (serviceKeyChips.value[svc] ?? []).map((c) => (c.id ? `ref:${c.id}` : c.value)).join(',')
    const res = await adminApi.updateServiceKeys({ [svc]: value })
    if (res.ok) {
      await refetchServiceKeys()
    } else {
      const data = await res.json().catch(() => null)
      serviceKeysError.value = data?.error || 'Failed to save'
    }
  } catch {
    // Without this a network error left "saving" set and blocked every
    // later save until a reload.
    serviceKeysError.value = 'Failed to save'
  } finally {
    serviceKeysSaving.value = null
  }
  const next = pendingServiceSaves.values().next().value
  if (next !== undefined) {
    pendingServiceSaves.delete(next)
    await saveServiceKeys(next)
  }
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
  // ...which also means every automatic refetch re-applies the saved config.
  // A focus refetch would silently wipe unsaved edits (and re-fire all
  // previews), so only refetch on mount and on an explicit Refresh.
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
  queryFn: async () => okOrThrow<SettingsResponse>(await adminApi.getSettings(), 'Failed to fetch settings'),
})

watch(settings, (s) => {
  if (s) freeApiKeyEnabled.value = s.free_api_key_enabled
}, { immediate: true })

// Shared load/save/reset pattern — same composable the key view uses. The
// onSave hook merges the transient free_api_key_enabled flag into the
// payload before POST so this view doesn't need its own saveSettings shape.
const { loadSettings, saveSettings } = useRenderSettingsForm<SaveSettingsPayload>({
  api: {
    load: () => adminApi.getSettings(),
    save: (p) => adminApi.updateSettings(p),
  },
  onSave: (s) => ({ ...s, free_api_key_enabled: freeApiKeyEnabled.value }),
  saveErrorMessage: 'Failed to save settings',
})

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
  try {
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
  } catch {
    freeKeyError.value = 'Failed to save'
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
    await apiKeysRef.value?.saveExpanded()
  } else {
    await formRef.value?.save()
  }
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

</script>

<template>
  <div class="space-y-4">
    <!-- General (server config: client IP / reverse proxy) -->
    <GeneralSettings v-if="section === 'general'" />

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

      <Tabs v-model="apiTab">
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
              <div v-if="serviceKeys?.[service]?.locked" class="text-sm text-muted-foreground">
                {{ serviceKeys?.[service]?.masked }}
              </div>
              <div v-else class="flex flex-wrap items-center gap-1.5 rounded-md border border-input bg-transparent px-2 py-1.5 min-h-9">
                <span
                  v-for="chip in serviceKeyChips[service]"
                  :key="chipKey(chip)"
                  class="inline-flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-xs"
                >
                  <button
                    type="button"
                    class="hover:underline"
                    :title="isRevealed(service, chip) ? 'Hide key' : 'Reveal key'"
                    @click="toggleReveal(service, chip)"
                  >{{ displayedKey(service, chip) }}</button>
                  <button
                    v-if="isRevealed(service, chip)"
                    type="button"
                    class="hover:text-foreground"
                    :title="serviceCopyState[`${service}:${chipKey(chip)}`] === 'copied' ? 'Copied' : 'Copy to clipboard'"
                    @click="copyServiceKey(service, chip)"
                  >{{ serviceCopyState[`${service}:${chipKey(chip)}`] === 'copied' ? 'Copied' : 'Copy' }}</button>
                  <button
                    type="button"
                    class="hover:text-destructive disabled:opacity-50"
                    :aria-label="`Remove ${serviceLabels[service]} key`"
                    :disabled="serviceKeysSaving !== null"
                    @click="removeServiceKey(service, chip)"
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
          <span v-if="formRef?.error" class="text-sm text-destructive">{{ formRef.error }}</span>
          <Button size="sm" data-testid="save-settings-button" @click="handleSave">
            <Loader2 v-if="formRef?.saving" class="size-4 animate-spin mr-1" />
            Save
          </Button>
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
