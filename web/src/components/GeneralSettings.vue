<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { Check } from 'lucide-vue-next'
import { adminApi } from '@/lib/api'
import { okOrThrow } from '@/lib/api-error'
import { useSavedFlash } from '@/composables/useSavedFlash'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import BackupSettings from '@/components/BackupSettings.vue'
import RefreshButton from '@/components/RefreshButton.vue'

// Settings → General. "Config" holds server-level options (for now the
// client IP / reverse proxy setting: which peers may set X-Forwarded-For —
// the address the per-IP rate limits are keyed on); "Backup" exports and
// restores the settings.

const route = useRoute()
const router = useRouter()

type GeneralTab = 'config' | 'backup'
const validTabs: GeneralTab[] = ['config', 'backup']
const tab = computed<GeneralTab>({
  get() {
    const q = route.query.tab
    const v = Array.isArray(q) ? q[0] : q
    return (validTabs as string[]).includes(v ?? '') ? (v as GeneralTab) : 'config'
  },
  set(v) {
    void router.replace({ query: { ...route.query, tab: v } })
  },
})

type ProxySettings = { trust_private: boolean; trust_cloudflare: boolean; custom: string[] }
type CloudflareStatus = { ranges: number; source: string; checked_at: number; error?: string }
type CloudflareCheck = CloudflareStatus & { checked: boolean; changed: boolean; next_check_at: number }
type NetworkResponse = {
  locked: boolean
  settings: ProxySettings
  cloudflare: CloudflareStatus
  detected: { client_ip: string; remote_addr: string; forwarded_for: string }
}

const { data: network, refetch } = useQuery<NetworkResponse>({
  queryKey: ['network-settings'],
  queryFn: async () => okOrThrow<NetworkResponse>(await adminApi.getNetworkSettings(), 'Failed to load settings'),
  // Editing form: don't let a focus refetch overwrite unsaved changes.
  refetchOnWindowFocus: false,
})

const trustPrivate = ref(true)
const trustCloudflare = ref(false)
const custom = ref('')
const saving = ref(false)
const error = ref('')
const { active: savedFlash, flash: flashSaved } = useSavedFlash()

function applyFromServer(s: ProxySettings) {
  trustPrivate.value = s.trust_private
  trustCloudflare.value = s.trust_cloudflare
  custom.value = s.custom.join(', ')
}

watch(network, (n) => n && applyFromServer(n.settings), { immediate: true })

// Ticking "Cloudflare" asks the server to check cloudflare.com for newer
// edge ranges (applied automatically if they changed). Debounced: the check
// runs once the box has stayed ticked for CLOUDFLARE_CHECK_DEBOUNCE_MS, so
// ticking/unticking in quick succession sends at most one request. The
// server adds its own cooldown (24 h after a successful check, 10 min after
// a failed one), so reloads and other tabs don't re-check either.
const CLOUDFLARE_CHECK_DEBOUNCE_MS = 1500
const cloudflareCheck = ref<CloudflareCheck | null>(null)
const cloudflareChecking = ref(false)
let cloudflareTimer: ReturnType<typeof setTimeout> | null = null

async function runCloudflareCheck() {
  cloudflareTimer = null
  if (!trustCloudflare.value) return
  cloudflareChecking.value = true
  try {
    const res = await adminApi.checkCloudflareRanges()
    if (res.ok) cloudflareCheck.value = (await res.json()) as CloudflareCheck
  } catch {
    // status line keeps the last known state
  } finally {
    cloudflareChecking.value = false
  }
}

function onCloudflareToggle(v: boolean) {
  trustCloudflare.value = v
  if (cloudflareTimer) clearTimeout(cloudflareTimer)
  cloudflareTimer = v ? setTimeout(runCloudflareCheck, CLOUDFLARE_CHECK_DEBOUNCE_MS) : null
}

onBeforeUnmount(() => {
  if (cloudflareTimer) clearTimeout(cloudflareTimer)
})

const cloudflareStatus = computed<CloudflareStatus | null>(() => cloudflareCheck.value ?? network.value?.cloudflare ?? null)

function formatTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString()
}

function customList(): string[] {
  return custom.value
    .split(/[\s,]+/)
    .map((v) => v.trim())
    .filter(Boolean)
}

const dirty = computed(() => {
  const s = network.value?.settings
  if (!s) return false
  return (
    s.trust_private !== trustPrivate.value ||
    s.trust_cloudflare !== trustCloudflare.value ||
    s.custom.join(',') !== customList().join(',')
  )
})

async function save() {
  saving.value = true
  error.value = ''
  try {
    const res = await adminApi.updateNetworkSettings({
      trust_private: trustPrivate.value,
      trust_cloudflare: trustCloudflare.value,
      custom: customList(),
    })
    if (!res.ok) {
      const data = await res.json().catch(() => null)
      error.value = data?.error || 'Failed to save'
      return
    }
    await refetch()
    flashSaved()
  } catch {
    error.value = 'Failed to save'
  } finally {
    saving.value = false
  }
}

// Only a user-initiated Refresh spins the button (not the initial load).
const refreshing = ref(false)
async function refresh() {
  refreshing.value = true
  try {
    await refetch()
    if (network.value) applyFromServer(network.value.settings)
  } finally {
    refreshing.value = false
  }
}

function discard() {
  if (network.value) applyFromServer(network.value.settings)
  error.value = ''
}
</script>

<template>
  <div class="rounded-lg border p-6 space-y-4">
    <div class="flex items-center justify-between">
      <h3 class="w-fit border border-t-0 border-l-0 rounded-tl-md rounded-br-md bg-muted px-4 py-2 text-sm font-bold uppercase tracking-widest">
        General
      </h3>
      <!-- Same place and order as the API / Image pages. Backup has nothing to save. -->
      <div v-if="tab === 'config'" class="flex items-center gap-2">
        <Button v-if="dirty" variant="outline" size="sm" :disabled="saving" @click="discard">Discard changes</Button>
        <span v-if="savedFlash" class="flex items-center gap-1.5 text-sm text-green-500">
          <Check class="size-4" />
          Saved
        </span>
        <Button size="sm" data-testid="network-save" :disabled="saving || !dirty || network?.locked" @click="save">Save</Button>
        <RefreshButton :fetching="refreshing" @refresh="refresh" />
      </div>
    </div>
    <p class="text-sm text-muted-foreground">Server-level configuration and backups.</p>

    <Tabs v-model="tab">
      <TabsList class="h-auto flex-wrap">
        <TabsTrigger value="config">Config</TabsTrigger>
        <TabsTrigger value="backup">Backup</TabsTrigger>
      </TabsList>

      <TabsContent value="config" class="mt-3">
        <div class="rounded-md border p-4 space-y-4" data-testid="network-settings">
          <h4 class="text-sm font-semibold flex items-center gap-2">
            Client IP &amp; reverse proxy
            <span v-if="network?.locked" class="text-xs font-normal text-muted-foreground bg-muted px-1.5 py-0.5 rounded">env</span>
          </h4>
          <p class="text-sm text-muted-foreground">
            Rate limits are counted per visitor IP. If OpenPosterDB runs behind a reverse proxy (nginx, Caddy) or
            Cloudflare, it only sees the proxy's address and has to read the visitor's IP from the
            <code>X-Forwarded-For</code> header. Tick only the proxies that really sit in front of this server: a trusted
            proxy can tell OpenPosterDB any IP.
          </p>
          <p v-if="network?.locked" class="text-sm text-muted-foreground">
            Set by the <code>TRUSTED_PROXIES</code> environment variable — change it there.
          </p>

          <div class="space-y-3">
            <label class="flex items-start gap-2 cursor-pointer">
              <Checkbox
                data-testid="trust-private"
                :model-value="trustPrivate"
                :disabled="network?.locked"
                @update:model-value="(v: unknown) => (trustPrivate = !!v)"
              />
              <span class="text-sm">
                Reverse proxy on this host or Docker network
                <span class="block text-xs text-muted-foreground">Trusts loopback and private addresses (127.0.0.1, 10.x, 172.16–31.x, 192.168.x).</span>
              </span>
            </label>
            <label class="flex items-start gap-2 cursor-pointer">
              <Checkbox
                data-testid="trust-cloudflare"
                :model-value="trustCloudflare"
                :disabled="network?.locked"
                @update:model-value="(v: unknown) => onCloudflareToggle(!!v)"
              />
              <span class="text-sm">
                Cloudflare
                <span class="block text-xs text-muted-foreground">Trusts Cloudflare's edge servers (their published IP ranges, checked for updates when you tick this).</span>
                <span v-if="trustCloudflare && (cloudflareStatus || cloudflareChecking)" class="block text-xs text-muted-foreground" data-testid="cloudflare-status">
                  <template v-if="cloudflareChecking">Checking cloudflare.com for updated ranges…</template>
                  <template v-else-if="cloudflareStatus">
                    {{ cloudflareStatus.ranges }} ranges
                    <template v-if="cloudflareStatus.checked_at"> · checked {{ formatTime(cloudflareStatus.checked_at) }}</template>
                    <template v-else> · built-in list</template>
                    <template v-if="cloudflareCheck?.changed"> · updated just now</template>
                    <template v-else-if="cloudflareCheck?.checked && !cloudflareStatus.error"> · up to date</template>
                    <span v-if="cloudflareStatus.error" class="text-destructive"> · last check failed, using the current list</span>
                  </template>
                </span>
              </span>
            </label>
            <div class="space-y-1">
              <Label for="trusted-custom" class="text-sm">Other proxies (optional)</Label>
              <Input
                id="trusted-custom"
                v-model="custom"
                data-testid="trust-custom"
                :disabled="network?.locked"
                placeholder="IPs or ranges, comma-separated — e.g. 203.0.113.10, 198.51.100.0/24"
                class="font-mono"
              />
            </div>
          </div>
          <p v-if="error" class="text-sm text-destructive">{{ error }}</p>

          <div v-if="network" class="rounded-md bg-muted/50 p-3 text-sm space-y-1" data-testid="network-detected">
            <p>
              OpenPosterDB sees your IP as <code class="font-semibold">{{ network.detected.client_ip }}</code>
              <span class="ml-1 text-muted-foreground">(with the saved settings)</span>
            </p>
            <p class="text-xs text-muted-foreground">
              Connection from {{ network.detected.remote_addr }}<template v-if="network.detected.forwarded_for">
                · X-Forwarded-For: {{ network.detected.forwarded_for }}</template>
            </p>
            <p class="text-xs text-muted-foreground">
              If this shows a proxy or Cloudflare address instead of your own, tick the matching option above.
            </p>
          </div>
        </div>
      </TabsContent>

      <TabsContent value="backup" class="mt-3">
        <BackupSettings />
      </TabsContent>
    </Tabs>
  </div>
</template>
