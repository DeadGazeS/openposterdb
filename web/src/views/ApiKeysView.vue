<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { useSavedFlash } from '@/composables/useSavedFlash'
import { parseApiError, okOrThrow } from '@/lib/api-error'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { keysApi, adminApi } from '@/lib/api'
import type { SaveSettingsPayload } from '@/lib/settings'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import type { RenderSettings } from '@/lib/settings'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Settings, Plus, Loader2, Check } from 'lucide-vue-next'

interface ApiKey {
  id: number
  name: string
  key_prefix: string
  created_at: string
  last_used_at: string | null
}

const queryClient = useQueryClient()

const { data: keys = ref([]) } = useQuery<ApiKey[]>({
  queryKey: ['api-keys'],
  queryFn: async () => okOrThrow<ApiKey[]>(await keysApi.list(), 'Failed to fetch keys'),
  initialData: [],
})

const newKeyName = ref('')
const newKeyValue = ref<string | null>(null)
const error = ref('')
const loading = ref(false)
const { active: showCreateCheck, flash: flashCreated } = useSavedFlash()

// Per-row prefix reveal state — mirrors the Source API Keys reveal UX in
// SettingsView.vue. Each click toggles the prefix between a masked placeholder
// and the actual `key_prefix` value (the server only stores the prefix, not
// the full key — that's shown once at creation time via `newKeyValue`).
const revealedKeyIds = ref<Set<number>>(new Set())
function toggleKeyReveal(id: number) {
  const next = new Set(revealedKeyIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  revealedKeyIds.value = next
}

// Per-key settings state
const expandedKey = ref<number | null>(null)
const keySettings = reactive<Record<number, RenderSettings>>({})
const settingsLoading = reactive<Record<number, boolean>>({})

async function toggleSettings(id: number) {
  if (expandedKey.value === id) {
    expandedKey.value = null
    return
  }
  expandedKey.value = id
  if (!keySettings[id]) {
    await fetchSettings(id)
  }
}

async function fetchSettings(id: number): Promise<RenderSettings | null> {
  const isInitialLoad = !keySettings[id]
  if (isInitialLoad) settingsLoading[id] = true
  try {
    const res = await keysApi.getSettings(id)
    if (res.ok) {
      const data: RenderSettings = await res.json()
      keySettings[id] = data
      return data
    }
  } catch {
    // handled by caller
  } finally {
    if (isInitialLoad) settingsLoading[id] = false
  }
  return null
}

function makeLoadSettings(id: number) {
  return () => fetchSettings(id)
}

function makeSaveSettings(id: number) {
  return async (s: SaveSettingsPayload): Promise<string | null> => {
    const res = await keysApi.updateSettings(id, s)
    if (res.ok) return null
    const data = await res.json().catch(() => null)
    return data?.error || 'Failed to save'
  }
}

function makeResetSettings(id: number) {
  return async (): Promise<boolean> => {
    const res = await keysApi.deleteSettings(id)
    return res.ok
  }
}

async function createKey() {
  if (loading.value || !newKeyName.value.trim()) return
  error.value = ''
  loading.value = true
  showCreateCheck.value = false
  try {
    const res = await keysApi.create(newKeyName.value.trim())
    if (res.ok) {
      const data = await res.json()
      newKeyValue.value = data.key
      newKeyName.value = ''
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
      flashCreated()
    } else {
      error.value = await parseApiError(res, 'Failed to create key')
    }
  } catch {
    error.value = 'Failed to create key'
  } finally {
    loading.value = false
  }
}

async function deleteKey(id: number) {
  if (!confirm('Delete this API key? Any services using it will stop working.')) return
  error.value = ''
  try {
    const res = await keysApi.delete(id)
    if (res.ok) {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    } else {
      const data = await res.json().catch(() => null)
      error.value = data?.error || 'Failed to delete key'
    }
  } catch {
    error.value = 'Failed to delete key'
  }
}

// --- Header-driven actions (the settings page header owns Save/Discard/Refresh) ---

const expandedFormRef = ref<{
  save: () => Promise<void>
  discard: () => void
  dirty: boolean
} | null>(null)

function saveExpanded() {
  return expandedFormRef.value?.save()
}

function discardExpanded() {
  expandedFormRef.value?.discard()
}

// Refresh pulls the last saved per-key config and overrides unsaved edits:
// replacing the settings object re-triggers the form's props.settings watcher.
async function refreshExpanded() {
  if (expandedKey.value == null) return
  await fetchSettings(expandedKey.value)
}

const expandedDirty = computed(() => expandedFormRef.value?.dirty ?? false)

defineExpose({ saveExpanded, discardExpanded, refreshExpanded, expandedDirty })
</script>

<template>
  <div class="space-y-4">
    <!-- Create new key -->
    <div class="space-y-3">
      <h4 class="text-sm font-semibold">Create new key</h4>
      <form class="flex gap-2" @submit.prevent="createKey">
        <Input
          v-model="newKeyName"
          type="text"
          placeholder="Key name (e.g. jellyfin-prod)"
          required
          class="flex-1"
        />
        <Button type="submit" :disabled="loading">
          <span class="relative size-4">
            <Transition
              enter-active-class="transition duration-200 ease-out"
              enter-from-class="opacity-0 scale-50"
              enter-to-class="opacity-100 scale-100"
              leave-active-class="transition duration-150 ease-in"
              leave-from-class="opacity-100 scale-100"
              leave-to-class="opacity-0 scale-50"
            >
              <Check v-if="showCreateCheck" class="absolute inset-0 size-4 text-green-500" />
              <Loader2 v-else-if="loading" class="absolute inset-0 size-4 animate-spin" />
              <Plus v-else class="absolute inset-0 size-4" />
            </Transition>
          </span>
          Create
        </Button>
      </form>
      <p v-if="error" class="text-sm text-destructive">{{ error }}</p>

      <!-- Show newly created key -->
      <div v-if="newKeyValue" class="rounded-md border border-yellow-500 bg-yellow-50 dark:bg-yellow-950 p-4 space-y-2">
        <p class="text-sm font-medium">Copy your API key now. It won't be shown again.</p>
        <code class="block text-sm bg-background border rounded px-3 py-2 break-all select-all">{{ newKeyValue }}</code>
        <Button variant="outline" size="sm" @click="newKeyValue = null">Dismiss</Button>
      </div>
    </div>

    <!-- Key list -->
    <div class="space-y-3">
      <h4 class="text-sm font-semibold">Existing keys</h4>
      <p v-if="keys.length === 0" class="text-sm text-muted-foreground">No API keys yet.</p>
      <div v-for="key in keys" :key="key.id" class="rounded-md border">
        <div class="flex items-center justify-between p-3">
          <div class="space-y-1">
            <p class="font-medium text-sm">{{ key.name }}</p>
            <p class="text-xs text-muted-foreground">
              <button
                type="button"
                class="font-mono hover:underline"
                :title="revealedKeyIds.has(key.id) ? 'Hide key' : 'Reveal key'"
                @click="toggleKeyReveal(key.id)"
              >{{ revealedKeyIds.has(key.id) ? `${key.key_prefix}...` : '••••••••' }}</button>
              &middot; Created {{ key.created_at }}
              <template v-if="key.last_used_at"> &middot; Last used {{ key.last_used_at }}</template>
            </p>
          </div>
          <div class="flex items-center gap-2">
            <Button variant="outline" size="sm" @click="toggleSettings(key.id)">
              <Settings class="h-4 w-4" />
            </Button>
            <Button variant="destructive" size="sm" @click="deleteKey(key.id)">Delete</Button>
          </div>
        </div>

        <!-- Inline settings panel -->
        <div v-if="expandedKey === key.id" class="border-t px-3 py-4 bg-muted/30">
          <div v-if="settingsLoading[key.id]" class="text-sm text-muted-foreground">Loading settings...</div>
          <RenderSettingsForm
            v-else-if="keySettings[key.id]"
            ref="expandedFormRef"
            :settings="keySettings[key.id]!"
            :uid="String(key.id)"
            :show-actions="false"
            :load-settings="makeLoadSettings(key.id)"
            :save-settings="makeSaveSettings(key.id)"
            :reset-settings="makeResetSettings(key.id)"
            :fetch-preview="adminApi.preview"
                                              />
        </div>
      </div>
    </div>
  </div>
</template>
