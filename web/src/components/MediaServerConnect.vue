<script setup lang="ts">
import { ref, computed } from 'vue'
import { useCopyToClipboard } from '@/composables/useCopyToClipboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { ChevronRight, Copy, Check } from 'lucide-vue-next'

const props = defineProps<{
  apiKey: string
  keyPrefix: string
}>()

const JELLYFIN_MANIFEST_URL =
  'https://raw.githubusercontent.com/PNRxA/jellyfin-plugin-openposterdb/main/manifest.json'
const PLEX_BASE = 'https://plex.openposterdb.com'

const isLegacy = computed(() => !props.apiKey)
// `{api_key}` is left as a literal placeholder for legacy keys whose raw
// value was never persisted (see api-go/internal/services/apikeys.go —
// EncryptedKey is nil for rows created before that column shipped).
const effectiveKey = computed(() => props.apiKey || '{api_key}')

const baseUrl = ref(window.location.origin)
const altEpisodeIdsOpen = ref(false)

const { copyState, copy } = useCopyToClipboard()

const aioRows = computed(() => [
  { key: 'aio-poster', label: 'Poster', url: `${baseUrl.value}/${effectiveKey.value}/imdb/poster-default/{imdb_id}.jpg` },
  { key: 'aio-backdrop', label: 'Background', url: `${baseUrl.value}/${effectiveKey.value}/tmdb/backdrop-default/{type}-{tmdb_id}.jpg?imageSize=large` },
  { key: 'aio-logo', label: 'Logo', url: `${baseUrl.value}/${effectiveKey.value}/tmdb/logo-default/{type}-{tmdb_id}.png` },
  { key: 'aio-episode', label: 'Episode', url: `${baseUrl.value}/${effectiveKey.value}/imdb/episode-default/episode-{imdb_id}-S{season}E{episode}.jpg` },
])

const altEpisodeRows = computed(() => [
  { key: 'aio-episode-tmdb', label: 'TMDB', url: `${baseUrl.value}/${effectiveKey.value}/tmdb/episode-default/episode-{tmdb_id}-S{season}E{episode}.jpg` },
  { key: 'aio-episode-tvdb', label: 'TVDB', url: `${baseUrl.value}/${effectiveKey.value}/tvdb/episode-default/episode-{tvdb_id}-S{season}E{episode}.jpg` },
])

const plexRows = computed(() => [
  { key: 'plex-movies', label: 'Movies', url: `${PLEX_BASE}/${effectiveKey.value}/movie` },
  { key: 'plex-tv', label: 'TV', url: `${PLEX_BASE}/${effectiveKey.value}/tv` },
])

const plexExampleUrl = computed(() => `${PLEX_BASE}/${effectiveKey.value}/movie~badge_style=v&ratings_limit=5`)
</script>

<template>
  <div class="space-y-6 text-sm">
    <div class="space-y-1.5">
      <Label for="connect-base-url">Base URL</Label>
      <Input id="connect-base-url" v-model="baseUrl" data-testid="connect-base-url" />
      <p class="text-xs text-muted-foreground">
        Auto-detected from this page's URL — edit if your public URL differs (e.g. behind a reverse proxy).
      </p>
    </div>

    <div v-if="isLegacy" data-testid="connect-legacy-banner" class="rounded-md border border-yellow-500 bg-yellow-50 dark:bg-yellow-950 p-3 text-sm">
      Legacy key — the raw value isn't available (it was never stored for keys created before this feature shipped). <code>{api_key}</code> below is left as a placeholder; recreate this key to get a fully working copy-paste URL.
    </div>

    <div class="space-y-2">
      <h5 class="font-semibold">AIOMetadata</h5>
      <p class="text-xs text-muted-foreground">
        Set these URL templates in your AIOMetadata configuration. <code>{type}</code> is <code>movie</code> or <code>series</code>.
      </p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-28">Image type</TableHead>
            <TableHead>URL template</TableHead>
            <TableHead class="w-10"><span class="sr-only">Copy</span></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in aioRows" :key="row.key">
            <TableCell class="font-medium">{{ row.label }}</TableCell>
            <TableCell class="font-mono text-xs break-all">{{ row.url }}</TableCell>
            <TableCell>
              <Button variant="ghost" size="sm" :title="`Copy ${row.label} URL`" @click="copy(row.url, row.key)">
                <Check v-if="copyState[row.key] === 'copied'" class="h-4 w-4" />
                <Copy v-else class="h-4 w-4" />
              </Button>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>

      <Collapsible v-model:open="altEpisodeIdsOpen">
        <CollapsibleTrigger as-child>
          <button type="button" class="flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors">
            <ChevronRight class="h-3.5 w-3.5 shrink-0 transition-transform duration-200" :class="{ 'rotate-90': altEpisodeIdsOpen }" />
            Alternate episode ID formats (TMDB / TVDB)
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent class="pt-2">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="w-28">ID type</TableHead>
                <TableHead>Episode URL template</TableHead>
                <TableHead class="w-10"><span class="sr-only">Copy</span></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="row in altEpisodeRows" :key="row.key">
                <TableCell class="font-medium">{{ row.label }}</TableCell>
                <TableCell class="font-mono text-xs break-all">{{ row.url }}</TableCell>
                <TableCell>
                  <Button variant="ghost" size="sm" :title="`Copy ${row.label} URL`" @click="copy(row.url, row.key)">
                    <Check v-if="copyState[row.key] === 'copied'" class="h-4 w-4" />
                    <Copy v-else class="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CollapsibleContent>
      </Collapsible>
    </div>

    <div class="space-y-2">
      <h5 class="font-semibold">Jellyfin</h5>
      <p class="text-xs text-muted-foreground">
        Dedicated <a href="https://github.com/PNRxA/jellyfin-plugin-openposterdb" target="_blank" rel="noopener" class="underline">Jellyfin plugin</a> — install via repository manifest, then set Base URL and API key.
      </p>
      <ol class="list-decimal list-inside space-y-2 text-xs text-muted-foreground">
        <li>
          In Jellyfin, go to <strong>Dashboard → Plugins → Repositories</strong> and add the manifest URL:
          <div class="mt-1 flex items-center gap-1">
            <code class="flex-1 block bg-muted px-2 py-1 rounded break-all select-all">{{ JELLYFIN_MANIFEST_URL }}</code>
            <Button variant="ghost" size="sm" title="Copy manifest URL" @click="copy(JELLYFIN_MANIFEST_URL, 'jellyfin-manifest')">
              <Check v-if="copyState['jellyfin-manifest'] === 'copied'" class="h-4 w-4" />
              <Copy v-else class="h-4 w-4" />
            </Button>
          </div>
        </li>
        <li>Install "OpenPosterDB" from the <strong>Catalog</strong>, then restart Jellyfin.</li>
        <li>
          Open the plugin's settings and set:
          <div class="mt-1 grid grid-cols-1 sm:grid-cols-2 gap-2">
            <div class="flex items-center gap-1">
              <span class="w-16 shrink-0">Base URL</span>
              <code class="flex-1 block bg-muted px-2 py-1 rounded break-all select-all">{{ baseUrl }}</code>
              <Button variant="ghost" size="sm" title="Copy Base URL" @click="copy(baseUrl, 'jellyfin-base-url')">
                <Check v-if="copyState['jellyfin-base-url'] === 'copied'" class="h-4 w-4" />
                <Copy v-else class="h-4 w-4" />
              </Button>
            </div>
            <div class="flex items-center gap-1">
              <span class="w-16 shrink-0">API key</span>
              <code class="flex-1 block bg-muted px-2 py-1 rounded break-all select-all">{{ effectiveKey }}</code>
              <Button variant="ghost" size="sm" :disabled="isLegacy" title="Copy API key" @click="copy(props.apiKey, 'jellyfin-api-key')">
                <Check v-if="copyState['jellyfin-api-key'] === 'copied'" class="h-4 w-4" />
                <Copy v-else class="h-4 w-4" />
              </Button>
            </div>
          </div>
        </li>
      </ol>
    </div>

    <div class="space-y-2">
      <h5 class="font-semibold">Plex</h5>
      <p class="text-xs text-muted-foreground">
        Hosted Custom Metadata Provider (PMS 1.43+) at Plex's own fixed domain — <strong>not</strong> your instance's base URL above.
      </p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-20">Library</TableHead>
            <TableHead>Provider URL</TableHead>
            <TableHead class="w-10"><span class="sr-only">Copy</span></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in plexRows" :key="row.key">
            <TableCell class="font-medium">{{ row.label }}</TableCell>
            <TableCell class="font-mono text-xs break-all">{{ row.url }}</TableCell>
            <TableCell>
              <Button variant="ghost" size="sm" :title="`Copy ${row.label} provider URL`" @click="copy(row.url, row.key)">
                <Check v-if="copyState[row.key] === 'copied'" class="h-4 w-4" />
                <Copy v-else class="h-4 w-4" />
              </Button>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <ol class="list-decimal list-inside space-y-1 text-xs text-muted-foreground">
        <li>In Plex, go to <strong>Settings → Manage → Metadata Agents → Add Provider</strong> and add the Movies URL above; add the TV URL as a second provider.</li>
        <li><strong>Add Agent</strong> — pair the OpenPosterDB provider with Plex Movie (and Plex TV Series), then drag it above the default art source.</li>
        <li>Assign the agent to each library under <strong>Library → Edit → Advanced → Agent</strong>, then <strong>Refresh Metadata</strong>.</li>
      </ol>
      <p class="text-xs text-muted-foreground">
        To pin a poster style, append options to the mount segment with <code>~key=val&amp;key2=val2</code> — note this differs from the <code>?key=val</code> query strings used everywhere else. Example:
      </p>
      <div class="flex items-center gap-1">
        <code class="flex-1 block bg-muted px-2 py-1 rounded break-all select-all text-xs">{{ plexExampleUrl }}</code>
        <Button variant="ghost" size="sm" title="Copy example URL" @click="copy(plexExampleUrl, 'plex-example')">
          <Check v-if="copyState['plex-example'] === 'copied'" class="h-4 w-4" />
          <Copy v-else class="h-4 w-4" />
        </Button>
      </div>
      <p class="text-xs text-muted-foreground">
        See the <a href="docs/api.md" class="underline">API reference</a> for all available options.
      </p>
    </div>
  </div>
</template>
