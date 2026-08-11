<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { ChevronRight, ChevronUp, ChevronDown, Eye, Info, Loader2, Download, Trash2 } from 'lucide-vue-next'
import { titleIdFromCacheValue } from '@/lib/utils'
import RefreshButton from '@/components/RefreshButton.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface ImageMeta {
  cache_key: string
  release_date: string | null
  created_at: number
  updated_at: number
  last_accessed: number
}

interface ListResponse {
  items: ImageMeta[]
  total: number
  page: number
  page_size: number
}

const props = withDefaults(defineProps<{
  kind: 'poster' | 'logo' | 'backdrop' | 'episode'
  listFn: (page: number, pageSize: number, sortBy: string, sortDir: string) => Promise<Response>
  imageFn: (key: string) => Promise<Response>
  fetchFn: (idType: string, idValue: string) => Promise<Response>
  deleteFn: (idType: string, idValue: string, scope: 'title' | 'variant') => Promise<Response>
  clearAllFn: () => Promise<Response>
  // Set false when the parent view renders the Fetch/Clear/Refresh buttons
  // itself (e.g. inside its own header row).
  showToolbar?: boolean
}>(), {
  showToolbar: true,
})

const route = useRoute()
const router = useRouter()

const page = computed({
  get: () => {
    const p = Number(route.query.page)
    return p > 0 ? p : 1
  },
  set: (v: number) => {
    router.replace({ query: { ...route.query, page: v === 1 ? undefined : String(v) } })
  },
})
const pageSize = 50

// Sort state — mirrored from the URL so a refresh keeps the user's choice.
// Default is last_accessed DESC (the most-recently-served row first); the
// backend allowlist accepts it alongside release_date / created_at / updated_at.
type SortBy = 'release_date' | 'created_at' | 'updated_at' | 'last_accessed'
type SortDir = 'asc' | 'desc'
const SORT_BY_VALUES: readonly SortBy[] = ['release_date', 'created_at', 'updated_at', 'last_accessed']
const SORT_DIR_VALUES: readonly SortDir[] = ['asc', 'desc']

function readSortBy(raw: unknown): SortBy {
  return SORT_BY_VALUES.includes(raw as SortBy) ? (raw as SortBy) : 'last_accessed'
}
function readSortDir(raw: unknown): SortDir {
  return SORT_DIR_VALUES.includes(raw as SortDir) ? (raw as SortDir) : 'desc'
}

const sortBy = computed(() => readSortBy(route.query.sort))
const sortDir = computed(() => readSortDir(route.query.dir))

async function setSort(column: SortBy) {
  // Apply both changes in a single router.replace — two consecutive replaces
  // race and the second cancels the first (Vue Router queues navigations).
  const nextSort: SortBy = column
  const nextDir: SortDir = sortBy.value === column
    ? (sortDir.value === 'asc' ? 'desc' : 'asc')
    : 'desc'
  await router.replace({
    query: {
      ...route.query,
      sort: nextSort === 'last_accessed' ? undefined : nextSort,
      dir: nextDir === 'desc' ? undefined : nextDir,
    },
  })
  // Refetch safety net — the queryKey is reactive on route.query so useQuery
  // refetches once the route commits, but call refetch() too so the refresh
  // is deterministic regardless of router commit timing. Awaiting the route
  // commit first guarantees the refetch reads the new sortBy/sortDir values.
  await refetch()
}

const { data, isPending, refetch } = useQuery<ListResponse>({
  queryKey: computed(() => ['admin', props.kind + 's', page.value, sortBy.value, sortDir.value]),
  queryFn: async () => {
    const res = await props.listFn(page.value, pageSize, sortBy.value, sortDir.value)
    if (!res.ok) throw new Error(`Failed to fetch ${props.kind}s`)
    return res.json()
  },
})

// True only while a user-initiated Refresh click is in flight — the query's
// own isFetching also fires on initial load / page changes.
const userRefreshing = ref(false)

async function handleRefresh() {
  userRefreshing.value = true
  try {
    await refetch()
  } finally {
    userRefreshing.value = false
  }
}

const previewOpen = ref(false)
const previewKey = ref('')
const previewUrl = ref<string | null>(null)
const previewLoading = ref(false)

async function openPreview(cacheKey: string) {
  previewKey.value = cacheKey
  previewOpen.value = true
  previewLoading.value = true
  previewUrl.value = null

  try {
    const res = await props.imageFn(cacheKey)
    if (res.ok) {
      const blob = await res.blob()
      previewUrl.value = URL.createObjectURL(blob)
    }
  } finally {
    previewLoading.value = false
  }
}

function closePreview() {
  previewOpen.value = false
  if (previewUrl.value) {
    URL.revokeObjectURL(previewUrl.value)
    previewUrl.value = null
  }
}

onUnmounted(() => {
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
})

const fetchModalOpen = ref(false)
const fetchIdType = ref('imdb')
const fetchIdValue = ref('')
const fetchLoading = ref(false)
const fetchError = ref('')

async function fetchImage() {
  const idValue = fetchIdValue.value.trim()
  if (!idValue) return

  fetchLoading.value = true
  fetchError.value = ''

  try {
    const res = await props.fetchFn(fetchIdType.value, idValue)
    if (!res.ok) {
      const text = await res.text()
      try { fetchError.value = JSON.parse(text).error || text } catch { fetchError.value = text || `Error ${res.status}` }
      return
    }
    const fetchedKey = `${fetchIdType.value}/${idValue}`
    // Use the image bytes from the fetch response directly for preview,
    // since the cache key used by imageFn may not match the full variant key.
    const blob = await res.blob()
    fetchIdValue.value = ''
    fetchModalOpen.value = false
    previewKey.value = fetchedKey
    previewOpen.value = true
    previewLoading.value = false
    previewUrl.value = URL.createObjectURL(blob)
    refetch()
  } catch (e) {
    fetchError.value = e instanceof Error ? e.message : 'Fetch failed'
  } finally {
    fetchLoading.value = false
  }
}

function openFetchModal() {
  fetchError.value = ''
  fetchModalOpen.value = true
}

// --- Purge: this exact variant (the row) or every variant of the title ---
const deleteOpen = ref(false)
const deleteTarget = ref<{ idType: string; cacheValue: string; titleId: string } | null>(null)
const deleteLoading = ref<'' | 'title' | 'variant'>('')
const deleteError = ref('')

function openDelete(cacheKey: string) {
  const { idType, idValue } = parseKey(cacheKey)
  deleteTarget.value = { idType, cacheValue: idValue, titleId: titleIdFromCacheValue(idValue) }
  deleteError.value = ''
  deleteOpen.value = true
}

async function confirmDelete(scope: 'title' | 'variant') {
  const target = deleteTarget.value
  if (!target || deleteLoading.value) return

  deleteLoading.value = scope
  deleteError.value = ''

  try {
    const idValue = scope === 'variant' ? target.cacheValue : target.titleId
    const res = await props.deleteFn(target.idType, idValue, scope)
    if (!res.ok) {
      const text = await res.text()
      try { deleteError.value = JSON.parse(text).error || text } catch { deleteError.value = text || `Error ${res.status}` }
      return
    }
    deleteOpen.value = false
    deleteTarget.value = null
    refetch()
  } catch (e) {
    deleteError.value = e instanceof Error ? e.message : 'Purge failed'
  } finally {
    deleteLoading.value = ''
  }
}

// --- Clear every cached image of this kind ---
const clearAllOpen = ref(false)
const clearAllLoading = ref(false)
const clearAllError = ref('')
const clearAllMessage = ref('')

function openClearAll() {
  clearAllError.value = ''
  clearAllMessage.value = ''
  clearAllOpen.value = true
}

async function confirmClearAll() {
  clearAllLoading.value = true
  clearAllError.value = ''

  try {
    const res = await props.clearAllFn()
    if (!res.ok) {
      const text = await res.text()
      try { clearAllError.value = JSON.parse(text).error || text } catch { clearAllError.value = text || `Error ${res.status}` }
      return
    }
    const body = await res.json().catch(() => ({}))
    const n = body.meta_deleted ?? 0
    clearAllMessage.value = `Cleared ${n} cached ${n === 1 ? kindLabel.value : kindLabelPlural.value}.`
    clearAllOpen.value = false
    page.value = 1
    refetch()
  } catch (e) {
    clearAllError.value = e instanceof Error ? e.message : 'Clear failed'
  } finally {
    clearAllLoading.value = false
  }
}

// Exposed so a parent view can drive the same actions from its own header
// row (with :show-toolbar="false"). userRefreshing unwraps to a reactive
// boolean when read through the component's public instance.
defineExpose({ openFetchModal, openClearAll, handleRefresh, userRefreshing })

function parseKey(cacheKey: string) {
  const idx = cacheKey.indexOf('/')
  if (idx < 0) return { idType: cacheKey, idValue: '' }
  return { idType: cacheKey.slice(0, idx), idValue: cacheKey.slice(idx + 1) }
}

// Extract the per-title group key from a cache_key by stripping the
// id-specific prefix. The remainder (variant + settings/ratings suffix) is
// what uniquely identifies the title + render — two rows with the same
// remainder but different idType prefixes are the same artwork.
function groupSuffixFromIdValue(idValue: string): string {
  // TMDB: movie-N / series-N / episode-N or episode-N-S<season>E<episode>
  // IMDb: ttNNNNN, optionally :S:E
  // TVDB: N, optionally :S:E
  const match = idValue.match(
    /^(?:movie-\d+|series-\d+|episode-\d+(?:-S\d+E\d+)?|tt\d+(?:[:\-]\d+)*|\d+(?:[:\-]\d+)*)/,
  )
  return match ? idValue.slice(match[0].length) : idValue
}

// Build a stable group id for an item by combining release_date + the
// suffix-after-id-value. Items with the same group id represent the same
// title rendered under different id types.
function groupKeyFor(item: ImageMeta): string {
  const { idValue } = parseKey(item.cache_key)
  return `${item.release_date ?? ''}::${groupSuffixFromIdValue(idValue)}`
}

// Pick the first row by the ACTIVE sort column. Reads `sortBy`/`sortDir` at
// call time so the `groups` computed re-runs whenever the user changes the
// sort header — variants within an expanded group stay in the active order,
// and the collapsed row is always "first in the user's chosen sort".
// Mirrors the backend ORDER BY exactly: `<col> <dir>, cache_key <dir>`. The
// direction-following cache_key tiebreaker means ASC/DESC flips reorder even
// when every row ties on the sort column (same-second timestamps, or
// last_accessed = 0). release_date nulls follow SQLite semantics (first in
// ASC, last in DESC) so the collapsed row matches the backend's group order.
// Caveat: re-fetching bumps updated_at regardless of which column the user
// is sorting by, so an inactive `updated_at` sort can still reshuffle the
// preferred on every refetch. That's the spec.
function compareByActiveSort(a: ImageMeta, b: ImageMeta): number {
  const field = sortBy.value
  const dir = sortDir.value
  let primary = 0
  if (field === 'release_date') {
    const av = a.release_date
    const bv = b.release_date
    if (av === null && bv === null) primary = 0
    else if (av === null) primary = -1 // null sorts first in ASC
    else if (bv === null) primary = 1
    else primary = av < bv ? -1 : av > bv ? 1 : 0
  } else {
    // All other sortable fields are numbers (Unix epoch seconds).
    const av = (a as unknown as Record<string, number>)[field]!
    const bv = (b as unknown as Record<string, number>)[field]!
    primary = av === bv ? 0 : av < bv ? -1 : 1
  }
  const tie = a.cache_key === b.cache_key ? 0 : a.cache_key < b.cache_key ? -1 : 1
  const cmp = primary || tie
  return dir === 'desc' ? -cmp : cmp
}

function sortByActive(items: ImageMeta[]): ImageMeta[] {
  return [...items].sort(compareByActiveSort)
}

interface Group {
  key: string
  preferred: ImageMeta
  others: ImageMeta[]
}

const groups = computed<Group[]>(() => {
  if (!data.value) return []
  const map = new Map<string, ImageMeta[]>()
  for (const item of data.value.items) {
    const k = groupKeyFor(item)
    const list = map.get(k)
    if (list) list.push(item)
    else map.set(k, [item])
  }
  const result = Array.from(map.entries()).map(([key, items]) => {
    const sorted = sortByActive(items)
    return { key, preferred: sorted[0]!, others: sorted.slice(1) }
  })
  return result
})

// Track which groups are expanded. Default collapsed.
const expandedGroups = ref<Set<string>>(new Set())
function toggleGroup(key: string) {
  const next = new Set(expandedGroups.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedGroups.value = next
}

// Release dates arrive as ISO "YYYY-MM-DD" strings; display them in the
// user's dd.mm.yyyy format (zero-padded, locale-independent). Empty/missing
// renders "—"; a malformed value passes through untouched rather than
// rendering garbage.
function formatReleaseDate(date: string | null): string {
  if (!date) return '—'
  const m = date.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  return m ? `${m[3]}.${m[2]}.${m[1]}` : date
}

function relativeTime(epoch: number) {
  if (!epoch) return '—'
  const diff = Date.now() / 1000 - epoch
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

// --- Short ID (the bare idValue without the variant + settings suffix) ---
//
// The cache_key is `{idType}/{idValue}{variant}@{ratings}{settings}` (see
// api-go/internal/image/serve.go cacheVariant + cachesuffix.go). The ID Value
// column shows just the bare idValue (e.g. `tt9813792` for an IMDb key) so the
// column stays narrow; the full key is broken down in the more-info dialog.
function shortIdFromCacheKey(cacheKey: string): string {
  const slash = cacheKey.indexOf('/')
  const after = slash >= 0 ? cacheKey.slice(slash + 1) : cacheKey
  const us = after.indexOf('_')
  const at = after.indexOf('@')
  let stop = -1
  if (us >= 0 && at >= 0) stop = Math.min(us, at)
  else if (us >= 0) stop = us
  else if (at >= 0) stop = at
  return stop >= 0 ? after.slice(0, stop) : after
}

// --- Cache key breakdown for the more-info dialog ---
//
// Mirrors the suffix order in api-go/internal/services/cachesuffix.go
// SettingsCacheSuffixWithRatings. Each setting key uses longest-prefix matching
// so e.g. `.ly45615199` matches `ly` not `l`, and `.stb` matches `s` not `sh`.
// Rating chars come from api-go/internal/services/ratings.go RatingSource.

interface CacheKeyBreakdown {
  idType: string
  idValue: string
  variant: { raw: string; kind: string; source: string; lang: string }
  ratings: { raw: string; providers: Array<{ char: string; name: string }> }
  settings: Array<{ key: string; value: string; label: string; explanation: string }>
  raw: string
}

// Char → display name for rating providers (mirror of RatingSource.Label).
// Mirrors api-go/internal/services/ratings.go.
const RATING_NAMES: Record<string, string> = {
  i: 'IMDb',
  c: 'Rotten Tomatoes Critics',
  a: 'Rotten Tomatoes Audience',
  m: 'Metacritic',
  t: 'TMDB',
  k: 'Trakt',
  e: 'MyAnimeList',
  l: 'Letterboxd',
  d: 'MDBList',
  r: 'Roger Ebert',
}

const VARIANT_KIND_LABELS: Record<string, string> = {
  l: 'logo',
  p: 'poster',
  b: 'backdrop',
  e: 'episode',
}

const VARIANT_SOURCE_LABELS: Record<string, string> = {
  f: 'fanart',
  t: 'tmdb',
}

// Labels + plain-English explanations for every setting key the api-go
// cachesuffix.go emits. `bz` is badge size (per api-go ScaleCacheSuffix), not
// "border" as an early draft had it.
const SETTINGS_LABELS: Record<string, { label: string; explanation: string }> = {
  s:    { label: 'Badge style',     explanation: 'how the badge is composed (lr / rl / tb / bt)' },
  l:    { label: 'Label style',     explanation: 'icon / text / official / high-res' },
  d:    { label: 'Badge direction', explanation: 'horizontal / vertical' },
  ly:   { label: 'Layout hash',     explanation: '8-char SHA-256 of the layout positions / order' },
  ls:   { label: 'Logo size',       explanation: '% of default logo size' },
  ts:   { label: 'Text size',       explanation: '% of default font size' },
  bz:   { label: 'Badge size',      explanation: '% of default badge size' },
  bw:   { label: 'Badge width',     explanation: '% of default badge width' },
  bh:   { label: 'Badge height',    explanation: '% of default badge height' },
  sh:   { label: 'Badge shape',     explanation: 'rounded (r) or pill (p)' },
  ba:   { label: 'Badge alpha',     explanation: 'background opacity' },
  zm:   { label: 'Image size',      explanation: 'default (medium) when no override' },
  col:  { label: 'Colors hash',     explanation: '8-char SHA-256 of custom color overrides' },
  blur: { label: 'Blur background', explanation: 'episode backdrop blur flag' },
  eh:   { label: 'Edge inset X',    explanation: 'backdrop horizontal edge inset (px)' },
  ev:   { label: 'Edge inset Y',    explanation: 'backdrop vertical edge inset (px)' },
}

// Setting keys ordered longest-first so the prefix match is unambiguous
// (e.g. `ly` beats `l`, `bw` beats `b`, `stb` falls through to `s`).
const SETTING_KEYS_LONGEST_FIRST: readonly string[] = [
  'col', 'ly', 'ls', 'bw', 'bh', 'bz', 'ba',
  'blur', 'ts', 'sh', 'zm', 'eh', 'ev',
  's', 'l', 'd',
]

function parseVariant(token: string): { kind: string; source: string; lang: string } {
  // token format examples (cacheVariant + TmdbPosterVariant):
  //   ''         → poster TMDB en default (TmdbPosterVariant returns '')
  //   '_f_en'    → poster fanart + lang
  //   '_f_tl'    → poster fanart + textless
  //   '_l_f_en'  → logo fanart + lang
  //   '_b_f'     → backdrop fanart (no lang)
  //   '_t_tl'    → poster TMDB + textless
  if (!token) return { kind: 'poster', source: 'tmdb', lang: 'en' }
  const rest = token.startsWith('_') ? token.slice(1) : token
  const parts = rest.split('_')
  const first = parts[0] ?? ''
  if (first === 'l' || first === 'b') {
    // Logo / backdrop: _<kind><source>[_lang]
    const source = parts[1] ?? ''
    const lang = parts[2] ?? ''
    return {
      kind: VARIANT_KIND_LABELS[first] ?? first,
      source: VARIANT_SOURCE_LABELS[source] ?? source,
      lang,
    }
  }
  // Poster (or other): _<source>[_lang] — first char is the source.
  const source = first
  const lang = parts[1] ?? ''
  return {
    kind: 'poster',
    source: VARIANT_SOURCE_LABELS[source] ?? source,
    lang,
  }
}

function parseCacheKey(cacheKey: string): CacheKeyBreakdown {
  // {idType}/{idValue}{variant}@{ratings}{settings}
  const slash = cacheKey.indexOf('/')
  const idType = slash >= 0 ? cacheKey.slice(0, slash) : ''
  const after = slash >= 0 ? cacheKey.slice(slash + 1) : cacheKey
  // Find variant boundary: the first `_` (if any) before `@`.
  const at = after.indexOf('@')
  const beforeAt = at >= 0 ? after.slice(0, at) : after
  let variantToken = ''
  let idValue = beforeAt
  const firstUnderscore = beforeAt.indexOf('_')
  if (firstUnderscore >= 0) {
    variantToken = beforeAt.slice(firstUnderscore)
    idValue = beforeAt.slice(0, firstUnderscore)
  }
  // Ratings: chars between `@` and the first `.` (or end).
  let ratingsRaw = ''
  let settingsRaw = ''
  if (at >= 0) {
    const afterAt = after.slice(at + 1)
    const dot = afterAt.indexOf('.')
    if (dot < 0) {
      ratingsRaw = afterAt
    } else {
      ratingsRaw = afterAt.slice(0, dot)
      settingsRaw = afterAt.slice(dot + 1)
    }
  }
  const variant = parseVariant(variantToken)
  const providers = ratingsRaw
    .split('')
    .filter((c) => c in RATING_NAMES)
    .map((c) => ({ char: c, name: RATING_NAMES[c]! }))
  const settings: CacheKeyBreakdown['settings'] = []
  if (settingsRaw) {
    for (const token of settingsRaw.split('.')) {
      if (!token) continue
      const matchedKey = SETTING_KEYS_LONGEST_FIRST.find((k) => token.startsWith(k))
      if (!matchedKey) continue
      const value = token.slice(matchedKey.length)
      const meta = SETTINGS_LABELS[matchedKey]
      settings.push({
        key: matchedKey,
        value,
        label: meta?.label ?? matchedKey,
        explanation: meta?.explanation ?? '',
      })
    }
  }
  return {
    idType,
    idValue,
    variant: { raw: variantToken, ...variant },
    ratings: { raw: ratingsRaw, providers },
    settings,
    raw: cacheKey,
  }
}

// --- More-info dialog state ---
const moreInfoOpen = ref(false)
const moreInfoKey = ref('')
function openMoreInfo(cacheKey: string) {
  moreInfoKey.value = cacheKey
  moreInfoOpen.value = true
}
function closeMoreInfo() {
  moreInfoOpen.value = false
}
const breakdown = computed<CacheKeyBreakdown | null>(() =>
  moreInfoKey.value ? parseCacheKey(moreInfoKey.value) : null,
)

const totalPages = computed(() => data.value ? Math.ceil(data.value.total / data.value.page_size) : 0)

const kindLabel = computed(() => props.kind)
const kindLabelPlural = computed(() => props.kind + 's')

function prevPage() {
  if (page.value > 1) page.value--
}

function nextPage() {
  if (page.value < totalPages.value) page.value++
}

const previewSizeClass = computed(() => {
  if (props.kind === 'backdrop') return 'max-w-2xl'
  if (props.kind === 'episode') return 'max-w-xl'
  return 'max-w-md'
})

const skeletonClass = computed(() => {
  if (props.kind === 'poster') return 'h-[400px] w-[270px] max-w-full rounded-md'
  if (props.kind === 'logo') return 'h-[200px] w-[400px] max-w-full rounded-md'
  return 'h-[270px] w-[480px] max-w-full rounded-md'
})
</script>

<template>
  <div class="space-y-4">
    <div class="rounded-lg border p-6 space-y-4">
      <div v-if="showToolbar" class="flex items-center justify-between">
        <h3 class="w-fit border border-t-0 border-l-0 rounded-tl-md rounded-br-md bg-muted px-4 py-2 text-sm font-bold uppercase tracking-widest">
          {{ kindLabelPlural }}
        </h3>
        <div class="flex items-center gap-2">
          <Button variant="outline" size="sm" @click="openFetchModal">
            <Download class="size-4 mr-1" />
            Fetch
          </Button>
          <Button variant="outline" size="sm" class="text-destructive hover:text-destructive" @click="openClearAll">
            <Trash2 class="size-4 mr-1" />
            Clear {{ kindLabelPlural }}
          </Button>
          <RefreshButton :fetching="userRefreshing" @refresh="handleRefresh" />
        </div>
      </div>
      <p v-if="clearAllMessage" class="text-sm text-muted-foreground text-right">{{ clearAllMessage }}</p>
      <div v-if="isPending" class="space-y-3">
        <Skeleton v-for="i in 5" :key="i" class="h-10 w-full" />
      </div>
      <template v-else-if="data">
        <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-10"></TableHead>
            <TableHead class="w-24">ID Type</TableHead>
            <TableHead class="w-40">ID Value</TableHead>
            <TableHead class="w-32">
              <button
                type="button"
                class="inline-flex items-center px-2 py-1 -mx-2 -my-1 rounded hover:bg-muted cursor-pointer"
                :class="sortBy === 'release_date' ? 'text-zinc-500 dark:text-zinc-400' : 'text-foreground'"
                @click="setSort('release_date')"
                :aria-sort="sortBy === 'release_date' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'"
              >
                <span :class="sortBy === 'release_date' ? 'underline' : ''">Release Date</span>
                <span class="inline-block w-3 ml-1 shrink-0">
                  <ChevronUp v-if="sortBy === 'release_date' && sortDir === 'asc'" class="size-3" />
                  <ChevronDown v-else-if="sortBy === 'release_date' && sortDir === 'desc'" class="size-3" />
                </span>
              </button>
            </TableHead>
            <TableHead class="w-32">
              <button
                type="button"
                class="inline-flex items-center px-2 py-1 -mx-2 -my-1 rounded hover:bg-muted cursor-pointer"
                :class="sortBy === 'last_accessed' ? 'text-zinc-500 dark:text-zinc-400' : 'text-foreground'"
                @click="setSort('last_accessed')"
                :aria-sort="sortBy === 'last_accessed' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'"
              >
                <span :class="sortBy === 'last_accessed' ? 'underline' : ''">Last Accessed</span>
                <span class="inline-block w-3 ml-1 shrink-0">
                  <ChevronUp v-if="sortBy === 'last_accessed' && sortDir === 'asc'" class="size-3" />
                  <ChevronDown v-else-if="sortBy === 'last_accessed' && sortDir === 'desc'" class="size-3" />
                </span>
              </button>
            </TableHead>
            <TableHead class="w-32">
              <button
                type="button"
                class="inline-flex items-center px-2 py-1 -mx-2 -my-1 rounded hover:bg-muted cursor-pointer"
                :class="sortBy === 'updated_at' ? 'text-zinc-500 dark:text-zinc-400' : 'text-foreground'"
                @click="setSort('updated_at')"
                :aria-sort="sortBy === 'updated_at' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'"
              >
                <span :class="sortBy === 'updated_at' ? 'underline' : ''">Last Updated</span>
                <span class="inline-block w-3 ml-1 shrink-0">
                  <ChevronUp v-if="sortBy === 'updated_at' && sortDir === 'asc'" class="size-3" />
                  <ChevronDown v-else-if="sortBy === 'updated_at' && sortDir === 'desc'" class="size-3" />
                </span>
              </button>
            </TableHead>
            <TableHead class="w-28">
              <button
                type="button"
                class="inline-flex items-center px-2 py-1 -mx-2 -my-1 rounded hover:bg-muted cursor-pointer"
                :class="sortBy === 'created_at' ? 'text-zinc-500 dark:text-zinc-400' : 'text-foreground'"
                @click="setSort('created_at')"
                :aria-sort="sortBy === 'created_at' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'"
              >
                <span :class="sortBy === 'created_at' ? 'underline' : ''">Created</span>
                <span class="inline-block w-3 ml-1 shrink-0">
                  <ChevronUp v-if="sortBy === 'created_at' && sortDir === 'asc'" class="size-3" />
                  <ChevronDown v-else-if="sortBy === 'created_at' && sortDir === 'desc'" class="size-3" />
                </span>
              </button>
            </TableHead>
            <TableHead class="w-10 text-right"><span class="sr-only">Actions</span></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-if="groups.length === 0">
            <TableCell colspan="8" class="text-center text-muted-foreground">No {{ kindLabelPlural }} cached yet.</TableCell>
          </TableRow>
          <template v-for="group in groups" :key="group.key">
            <TableRow class="cursor-pointer" @click="openPreview(group.preferred.cache_key)">
              <TableCell>
                <button
                  v-if="group.others.length"
                  type="button"
                  class="inline-flex items-center -m-1 p-1 rounded hover:bg-muted"
                  :title="`${group.others.length} more id${group.others.length === 1 ? '' : 's'} for this title (click to ${expandedGroups.has(group.key) ? 'collapse' : 'expand'})`"
                  :aria-label="expandedGroups.has(group.key) ? 'Collapse variants' : 'Expand variants'"
                  @click.stop="toggleGroup(group.key)"
                >
                  <ChevronRight
                    :class="[
                      'size-4 transition-transform text-muted-foreground',
                      expandedGroups.has(group.key) ? 'rotate-90' : '',
                    ]"
                  />
                </button>
              </TableCell>
              <TableCell>{{ parseKey(group.preferred.cache_key).idType }}</TableCell>
              <TableCell>{{ shortIdFromCacheKey(group.preferred.cache_key) }}</TableCell>
              <TableCell>{{ formatReleaseDate(group.preferred.release_date) }}</TableCell>
              <TableCell>{{ relativeTime(group.preferred.last_accessed) }}</TableCell>
              <TableCell>{{ relativeTime(group.preferred.updated_at) }}</TableCell>
              <TableCell>{{ relativeTime(group.preferred.created_at) }}</TableCell>
              <TableCell class="text-right">
                <div class="inline-flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    class="size-8 text-muted-foreground hover:text-foreground"
                    aria-label="Show cache_key breakdown"
                    title="Show cache_key breakdown"
                    @click.stop="openMoreInfo(group.preferred.cache_key)"
                  >
                    <Info class="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    class="size-8 text-muted-foreground hover:text-destructive"
                    :aria-label="`Purge ${kindLabel}`"
                    :title="`Purge cached ${kindLabel}`"
                    @click.stop="openDelete(group.preferred.cache_key)"
                  >
                    <Trash2 class="size-4" />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
            <TableRow
              v-for="(item, idx) in group.others"
              v-show="expandedGroups.has(group.key)"
              :key="item.cache_key"
              :class="[
                'cursor-pointer',
                idx === group.others.length - 1 ? 'border-b-2 border-foreground' : '',
              ]"
              @click="openPreview(item.cache_key)"
            >
              <TableCell></TableCell>
              <TableCell>{{ parseKey(item.cache_key).idType }}</TableCell>
              <TableCell>{{ shortIdFromCacheKey(item.cache_key) }}</TableCell>
              <TableCell>{{ formatReleaseDate(item.release_date) }}</TableCell>
              <TableCell>{{ relativeTime(item.last_accessed) }}</TableCell>
              <TableCell>{{ relativeTime(item.updated_at) }}</TableCell>
              <TableCell>{{ relativeTime(item.created_at) }}</TableCell>
              <TableCell class="text-right">
                <div class="inline-flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    class="size-8 text-muted-foreground hover:text-foreground"
                    aria-label="Show cache_key breakdown"
                    title="Show cache_key breakdown"
                    @click.stop="openMoreInfo(item.cache_key)"
                  >
                    <Info class="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    class="size-8 text-muted-foreground hover:text-destructive"
                    :aria-label="`Purge ${kindLabel}`"
                    :title="`Purge cached ${kindLabel}`"
                    @click.stop="openDelete(item.cache_key)"
                  >
                    <Trash2 class="size-4" />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          </template>
        </TableBody>
      </Table>
      <div class="flex items-center justify-between">
        <p class="text-sm text-muted-foreground">
          {{ data.total }} {{ data.total === 1 ? kindLabel : kindLabelPlural }} total
        </p>
        <div class="flex items-center gap-2">
          <Button variant="outline" size="sm" :disabled="page <= 1" @click="prevPage">Previous</Button>
          <span class="text-sm">Page {{ page }} of {{ totalPages }}</span>
          <Button variant="outline" size="sm" :disabled="page >= totalPages" @click="nextPage">Next</Button>
        </div>
      </div>
      </template>
    </div>

    <Dialog :open="fetchModalOpen" @update:open="(v: boolean) => { if (!v) fetchModalOpen = false }">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>Fetch {{ kindLabel.charAt(0).toUpperCase() + kindLabel.slice(1) }}</DialogTitle>
        </DialogHeader>
        <form class="space-y-4" @submit.prevent="fetchImage">
          <div class="space-y-2">
            <Label>ID Type</Label>
            <Select v-model="fetchIdType">
              <SelectTrigger data-testid="fetch-id-type-select">
                <SelectValue placeholder="Select ID type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="imdb">IMDb</SelectItem>
                <SelectItem value="tmdb">TMDb</SelectItem>
                <SelectItem value="tvdb">TVDB</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="space-y-2">
            <Label>ID Value</Label>
            <Input v-model="fetchIdValue" placeholder="e.g. tt1234567" />
          </div>
          <p v-if="fetchError" class="text-sm text-destructive">{{ fetchError }}</p>
          <div class="flex justify-end">
            <Button type="submit" :disabled="fetchLoading || !fetchIdValue.trim()">
              <Loader2 v-if="fetchLoading" class="size-4 animate-spin mr-1" />
              Fetch
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>

    <Dialog :open="clearAllOpen" @update:open="(v: boolean) => { if (!v) clearAllOpen = false }">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>Clear {{ kindLabelPlural }}</DialogTitle>
        </DialogHeader>
        <div class="space-y-4">
          <p class="text-sm text-muted-foreground">
            Remove <strong>all</strong> cached {{ kindLabelPlural }}<span v-if="data"> ({{ data.total }} title{{ data.total === 1 ? '' : 's' }})</span>?
            Other image types are unaffected. They regenerate on the next request.
          </p>
          <p v-if="clearAllError" class="text-sm text-destructive">{{ clearAllError }}</p>
          <div class="flex justify-end gap-2">
            <Button variant="outline" :disabled="clearAllLoading" @click="clearAllOpen = false">Cancel</Button>
            <Button variant="destructive" :disabled="clearAllLoading" @click="confirmClearAll">
              <Loader2 v-if="clearAllLoading" class="size-4 animate-spin mr-1" />
              Clear {{ kindLabelPlural }}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>

    <Dialog :open="deleteOpen" @update:open="(v: boolean) => { if (!v) { deleteOpen = false; deleteTarget = null } }">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>Purge {{ kindLabel }}</DialogTitle>
        </DialogHeader>
        <div class="space-y-4">
          <p class="text-sm text-muted-foreground">
            Remove just this one cached {{ kindLabel }} variant, or every variant of
            this title? Anything removed is regenerated on the next request.
          </p>
          <div class="space-y-1 rounded-md border bg-muted/40 p-2 text-xs font-mono break-all">
            <p><span class="text-muted-foreground">this variant: </span>{{ deleteTarget?.idType }}/{{ deleteTarget?.cacheValue }}</p>
            <p><span class="text-muted-foreground">entire title: </span>{{ deleteTarget?.idType }}/{{ deleteTarget?.titleId }}</p>
          </div>
          <p v-if="deleteError" class="text-sm text-destructive">{{ deleteError }}</p>
          <div class="flex flex-wrap justify-end gap-2">
            <Button variant="outline" :disabled="!!deleteLoading" @click="deleteOpen = false">Cancel</Button>
            <Button variant="outline" class="text-destructive hover:text-destructive" :disabled="!!deleteLoading" @click="confirmDelete('variant')">
              <Loader2 v-if="deleteLoading === 'variant'" class="size-4 animate-spin mr-1" />
              This variant
            </Button>
            <Button variant="destructive" :disabled="!!deleteLoading" @click="confirmDelete('title')">
              <Loader2 v-if="deleteLoading === 'title'" class="size-4 animate-spin mr-1" />
              Entire title
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>

    <Dialog :open="previewOpen" @update:open="(v: boolean) => { if (!v) closePreview() }">
      <DialogContent :class="previewSizeClass">
        <DialogHeader>
          <DialogTitle class="min-w-0 break-all pr-8 font-mono text-sm">{{ previewKey }}</DialogTitle>
        </DialogHeader>
        <div class="flex items-center justify-center min-h-[200px] min-w-0">
          <Skeleton v-if="previewLoading" :class="skeletonClass" />
          <img
            v-else-if="previewUrl"
            :src="previewUrl"
            :alt="previewKey"
            class="max-h-[70vh] max-w-full rounded-md object-contain"
          />
          <p v-else class="text-sm text-muted-foreground">Failed to load {{ kindLabel }}</p>
        </div>
      </DialogContent>
    </Dialog>

    <Dialog :open="moreInfoOpen" @update:open="(v: boolean) => { if (!v) closeMoreInfo() }">
      <DialogContent class="max-w-lg">
        <DialogHeader>
          <DialogTitle class="font-mono text-sm break-all">{{ moreInfoKey }}</DialogTitle>
        </DialogHeader>
        <div v-if="breakdown" class="space-y-3 text-sm">
          <dl class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1">
            <dt class="text-muted-foreground">ID Type</dt><dd class="font-mono">{{ breakdown.idType }}</dd>
            <dt class="text-muted-foreground">ID Value</dt><dd class="font-mono">{{ breakdown.idValue }}</dd>
            <dt class="text-muted-foreground">Kind</dt><dd>{{ breakdown.variant.kind }}</dd>
            <dt class="text-muted-foreground">Source</dt><dd>{{ breakdown.variant.source }}</dd>
            <dt class="text-muted-foreground">Language</dt><dd>{{ breakdown.variant.lang || '—' }}</dd>
          </dl>
          <div>
            <p class="text-muted-foreground mb-1">Ratings included</p>
            <div v-if="breakdown.ratings.providers.length" class="flex flex-wrap gap-1">
              <span
                v-for="r in breakdown.ratings.providers"
                :key="r.char"
                class="px-2 py-0.5 rounded bg-muted text-xs"
              >{{ r.name }} ({{ r.char }})</span>
            </div>
            <p v-else class="text-xs text-muted-foreground">none</p>
          </div>
          <div>
            <p class="text-muted-foreground mb-1">Render settings</p>
            <dl v-if="breakdown.settings.length" class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1">
              <template v-for="s in breakdown.settings" :key="s.key">
                <dt class="text-muted-foreground">{{ s.label }}</dt>
                <dd>
                  <span class="font-mono">{{ s.value || '—' }}</span>
                  <span class="text-muted-foreground text-xs ml-2">{{ s.explanation }}</span>
                </dd>
              </template>
            </dl>
            <p v-else class="text-xs text-muted-foreground">none</p>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  </div>
</template>
