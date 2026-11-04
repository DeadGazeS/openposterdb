<script setup lang="ts">
import { ref } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { Loader2, Download, Upload } from 'lucide-vue-next'
import { adminApi } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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

// Settings → General → Backup: export the settings to a file, or restore
// them from a previous export.

const queryClient = useQueryClient()

const exportDialogOpen = ref(false)
const exportServiceKeys = ref(true)
const exportAPIKeys = ref(true)
const exportPassphrase = ref('')
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
    const res = await adminApi.exportSettings(
      exportServiceKeys.value,
      exportAPIKeys.value,
      exportPassphrase.value || undefined,
    )
    if (!res.ok) {
      const data = await res.json().catch(() => null)
      exportError.value = data?.error || 'Export failed'
      return
    }
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const suffix = exportPassphrase.value ? '.enc' : ''
    a.download = `openposterdb-settings-${new Date().toISOString().slice(0, 10)}${suffix}.json`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    exportDialogOpen.value = false
    exportPassphrase.value = ''
  } catch {
    exportError.value = 'Export failed'
  } finally {
    exportLoading.value = false
  }
}

// After a restore, the other settings pages must not show stale data.
async function afterImport(data: ImportResult) {
  importResult.value = data
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['global-settings'] }),
    queryClient.invalidateQueries({ queryKey: ['service-keys'] }),
    queryClient.invalidateQueries({ queryKey: ['network-settings'] }),
  ])
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
    const parsed = JSON.parse(await file.text())
    let passphrase: string | undefined
    if (parsed?.kind === 'openposterdb/settings-encrypted') {
      passphrase = window.prompt('This file is encrypted. Enter the passphrase:') || undefined
      if (!passphrase) {
        importError.value = 'Import cancelled — passphrase required for encrypted export'
        return
      }
    }
    const res = await adminApi.importSettings(parsed, passphrase)
    const data = await res.json().catch(() => null)
    if (!res.ok) {
      importError.value = data?.error || 'Import failed'
      return
    }
    await afterImport(data)
  } catch {
    importError.value = 'Import failed — not a valid settings file'
  } finally {
    importBusy.value = false
  }
}
</script>

<template>
  <div class="rounded-md border p-4 space-y-4" data-testid="backup-settings">
    <h4 class="text-sm font-semibold">Backup &amp; Restore</h4>
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
                  (poster-serving keys + their settings + encrypted_key — on import the
                  raw key is recovered when SECRETS_KEY matches; auth via hash works
                  regardless, missing keys are still recreated)
                </span>
              </span>
            </label>
            <div class="space-y-1">
              <Label for="export-passphrase" class="text-sm">
                {{ exportServiceKeys ? 'Passphrase (required with external API keys)' : 'Encrypt export (optional)' }}
              </Label>
              <Input
                id="export-passphrase"
                v-model="exportPassphrase"
                type="password"
                placeholder="Passphrase — leave blank for plain JSON"
                autocomplete="new-password"
                class="font-mono"
              />
              <p class="text-xs text-muted-foreground">
                When set, the export file is encrypted with a passphrase-derived
                key (PBKDF2 + AES-256-GCM). The passphrase is required to import.
                External API keys are only exported encrypted.
              </p>
            </div>
            <p v-if="exportError" class="text-sm text-destructive">{{ exportError }}</p>
          </div>
          <DialogFooter>
            <DialogClose as-child>
              <Button variant="outline" size="sm">Cancel</Button>
            </DialogClose>
            <Button size="sm" :disabled="exportLoading || (exportServiceKeys && !exportPassphrase)" @click="runExport">
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
</template>
