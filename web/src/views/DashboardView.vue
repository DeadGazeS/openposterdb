<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { adminApi } from '@/lib/api'
import { okOrThrow } from '@/lib/api-error'
import RefreshButton from '@/components/RefreshButton.vue'
import ClearCacheButton from '@/components/ClearCacheButton.vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Button } from '@/components/ui/button'
import { ArrowLeft, ArrowRight, Eye, EyeOff, Info, LayoutGrid } from 'lucide-vue-next'
import {
  DASHBOARD_LAYOUT_PREF,
  defaultLayout,
  moveBy,
  moveTo,
  parseLayout,
  serializeLayout,
  sortedAZ,
  toggleHidden,
  type DashboardColumns,
  type DashboardLayout,
} from '@/lib/dashboardLayout'

interface Stats {
  total_images: number
  total_api_keys: number
  mem_cache_entries: number
  id_cache_entries: number
  ratings_cache_entries: number
  image_mem_cache_mb: number
  // null with EXTERNAL_CACHE_ONLY (no disk cache) or when measuring failed
  image_disk_cache_mb: number | null
  image_disk_cache_files: number | null
  uptime_seconds: number
  database_mb: number | null
  image_requests: number
  // null until the first image request since the restart
  mem_cache_hit_rate: number | null
}

const { data: stats, isPending, refetch } = useQuery<Stats>({
  queryKey: ['admin', 'stats'],
  queryFn: async () => okOrThrow<Stats>(await adminApi.getStats(), 'Failed to fetch stats'),
})

// True only while a user-initiated Refresh click is in flight — the query's
// own isFetching also fires on initial load / background refetches.
const userRefreshing = ref(false)

async function handleRefresh() {
  userRefreshing.value = true
  try {
    await refetch()
  } finally {
    userRefreshing.value = false
  }
}

// Disk = rendered images kept across restarts; the "Memory" tiles and the
// request counters are in-process and start over after every restart.
// Shown A–Z by default; each admin can reorder, hide tiles and pick the
// column count (Customize), stored in their server-side prefs.
const cards = [
  {
    key: 'total_images',
    label: 'Total Images',
    hint: 'Rendered posters, logos, backdrops and episode images recorded in the database.',
  },
  {
    key: 'image_disk_cache_mb',
    label: 'Image Disk Cache (MB)',
    hint: 'Space the rendered images take up on disk. Kept across restarts.',
  },
  {
    key: 'image_disk_cache_files',
    label: 'Disk Cache Files',
    hint: 'Image files in the disk cache. If this is far off Total Images, the database and the files are out of sync (e.g. files deleted by hand).',
  },
  {
    key: 'total_api_keys',
    label: 'API Keys',
    hint: 'Poster-serving API keys you have created.',
  },
  {
    key: 'database_mb',
    label: 'Database (MB)',
    hint: 'Size of the SQLite database (settings, API keys, image records).',
  },
  {
    key: 'uptime_seconds',
    label: 'Uptime',
    hint: 'How long the server has been running. The memory caches and request counters start over at every restart.',
  },
  {
    key: 'image_requests',
    label: 'Image Requests',
    hint: 'Poster, logo, backdrop and episode requests since the last restart that reached the cache.',
  },
  {
    key: 'mem_cache_hit_rate',
    label: 'Memory Cache Hit Rate',
    hint: 'Share of those requests answered straight from RAM. If it stays low under steady traffic, a larger IMAGE_MEM_CACHE_MB may help.',
  },
  {
    key: 'mem_cache_entries',
    label: 'Memory Cache Entries',
    hint: 'Images held in RAM for fast responses. Empty after a restart; fills as posters are requested.',
  },
  {
    key: 'image_mem_cache_mb',
    label: 'Image Memory Cache (MB)',
    hint: 'RAM used by those images, capped by IMAGE_MEM_CACHE_MB (default 512 MB).',
  },
  {
    key: 'id_cache_entries',
    label: 'ID Cache Entries',
    hint: 'Remembered ID lookups (e.g. IMDb → TMDB), so they are not fetched again. In memory; empty after a restart.',
  },
  {
    key: 'ratings_cache_entries',
    label: 'Ratings Cache Entries',
    hint: 'Fetched ratings per title, refreshed in the background after 30 minutes. In memory; empty after a restart.',
  },
] as const

type CardKey = (typeof cards)[number]['key']

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d} d ${h} h`
  if (h > 0) return `${h} h ${m} min`
  return m > 0 ? `${m} min` : '< 1 min'
}

function formatStatValue(key: CardKey, value: number | null | undefined): string {
  if (value == null) return '—'
  switch (key) {
    case 'image_mem_cache_mb':
    case 'image_disk_cache_mb':
    case 'database_mb':
      return value.toFixed(2)
    case 'uptime_seconds':
      return formatUptime(value)
    case 'mem_cache_hit_rate':
      return `${value.toFixed(1)} %`
    default:
      return String(value)
  }
}

type StatCard = (typeof cards)[number]
const cardByKey = new Map<string, StatCard>(cards.map((c) => [c.key, c]))

// --- Layout (order / hidden / columns) ---
const layout = ref<DashboardLayout>(defaultLayout(cards))
const editing = ref(false)
const layoutError = ref('')
let layoutBeforeEdit: DashboardLayout | null = null

onMounted(async () => {
  try {
    const res = await adminApi.getPrefs()
    if (res.ok) {
      const prefs = (await res.json()) as Record<string, string>
      // Slow prefs response: don't overwrite edits the user already started.
      if (!editing.value) layout.value = parseLayout(prefs[DASHBOARD_LAYOUT_PREF], cards)
    }
  } catch {
    // prefs unavailable: keep the default layout
  }
})

// While editing, hidden tiles stay in the grid (dimmed) so they can be shown again.
const visibleCards = computed(() =>
  layout.value.order
    .filter((k) => editing.value || !layout.value.hidden.includes(k))
    .map((k) => cardByKey.get(k))
    .filter((c): c is StatCard => !!c),
)

// Static class names so Tailwind keeps them.
const columnClass: Record<DashboardColumns, string> = {
  2: 'lg:grid-cols-2',
  3: 'lg:grid-cols-3',
  4: 'lg:grid-cols-4',
}

function isHidden(key: string) {
  return layout.value.hidden.includes(key)
}

function startEditing() {
  // Plain copy: structuredClone can't clone the reactive proxy.
  const l = layout.value
  layoutBeforeEdit = { order: [...l.order], hidden: [...l.hidden], columns: l.columns }
  layoutError.value = ''
  editing.value = true
}

function cancelEditing() {
  if (layoutBeforeEdit) layout.value = layoutBeforeEdit
  editing.value = false
}

async function finishEditing() {
  layoutError.value = ''
  try {
    const res = await adminApi.updatePrefs({ [DASHBOARD_LAYOUT_PREF]: serializeLayout(layout.value) })
    if (!res.ok) throw new Error()
    editing.value = false
  } catch {
    layoutError.value = 'Could not save the layout'
  }
}

function setColumns(columns: DashboardColumns) {
  layout.value = { ...layout.value, columns }
}

function sortAZ() {
  layout.value = { ...layout.value, order: sortedAZ(cards, layout.value.order) }
}

function resetLayout() {
  layout.value = defaultLayout(cards)
}

function step(key: string, delta: -1 | 1) {
  layout.value = { ...layout.value, order: moveBy(layout.value.order, key, delta) }
}

function toggle(key: string) {
  layout.value = { ...layout.value, hidden: toggleHidden(layout.value.hidden, key) }
}

// Drag & drop (desktop); the arrow buttons cover touch and keyboard.
const dragging = ref<string | null>(null)
function onDragStart(key: string, e: DragEvent) {
  dragging.value = key
  e.dataTransfer?.setData('text/plain', key)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
function onDrop(target: string) {
  if (dragging.value) layout.value = { ...layout.value, order: moveTo(layout.value.order, dragging.value, target) }
  dragging.value = null
}

const clearMessage = ref('')
function onCleared(message: string) {
  clearMessage.value = message
  refetch()
}
</script>

<template>
    <div class="space-y-4">
      <div class="rounded-lg border p-6 space-y-4">
        <div class="flex items-center justify-between">
          <h3 class="w-fit border border-t-0 border-l-0 rounded-tl-md rounded-br-md bg-muted px-4 py-2 text-sm font-bold uppercase tracking-widest">
            Dashboard
          </h3>
          <div class="flex items-center gap-2">
            <template v-if="editing">
              <Button variant="outline" size="sm" data-testid="layout-cancel" @click="cancelEditing">Cancel</Button>
              <Button size="sm" data-testid="layout-done" @click="finishEditing">Done</Button>
            </template>
            <template v-else>
              <Button variant="outline" size="sm" data-testid="layout-customize" @click="startEditing">
                <LayoutGrid class="size-4 mr-1" />
                Customize
              </Button>
              <ClearCacheButton @cleared="onCleared" />
              <RefreshButton :fetching="userRefreshing" @refresh="handleRefresh" />
            </template>
          </div>
        </div>
        <p v-if="clearMessage" class="text-sm text-muted-foreground text-right">{{ clearMessage }}</p>

        <div v-if="editing" class="rounded-md border bg-muted/40 p-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm" data-testid="layout-toolbar">
          <span class="text-muted-foreground">Drag tiles or use the arrows to move them; the eye shows or hides a tile.</span>
          <div class="flex items-center gap-1" role="group" aria-label="Columns">
            <span class="text-muted-foreground mr-1">Columns</span>
            <Button
              v-for="n in ([2, 3, 4] as const)"
              :key="n"
              size="sm"
              :variant="layout.columns === n ? 'default' : 'outline'"
              :aria-pressed="layout.columns === n"
              :data-testid="`layout-columns-${n}`"
              @click="setColumns(n)"
            >{{ n }}</Button>
          </div>
          <Button variant="outline" size="sm" data-testid="layout-sort" @click="sortAZ">Sort A–Z</Button>
          <Button variant="outline" size="sm" data-testid="layout-reset" @click="resetLayout">Reset</Button>
          <span v-if="layoutError" class="text-destructive">{{ layoutError }}</span>
        </div>

        <p v-if="!editing && visibleCards.length === 0" class="text-sm text-muted-foreground">
          All tiles are hidden. Use Customize to show some.
        </p>

        <TooltipProvider :delay-duration="150">
        <div class="grid gap-4 grid-cols-1 sm:grid-cols-2" :class="columnClass[layout.columns]" data-testid="stats-grid">
          <!-- Hovering (or focusing — tabindex) a tile explains the number; off while editing. -->
          <Tooltip v-for="(card, index) in visibleCards" :key="card.key" :disabled="editing">
            <TooltipTrigger as-child>
              <Card
                tabindex="0"
                :class="[
                  editing ? 'cursor-move border-dashed' : 'cursor-help',
                  { 'opacity-40': editing && isHidden(card.key), 'ring-2 ring-primary': dragging === card.key },
                ]"
                :data-testid="`stat-${card.key}`"
                :draggable="editing"
                @dragstart="(e: DragEvent) => onDragStart(card.key, e)"
                @dragover="(e: DragEvent) => editing && e.preventDefault()"
                @drop.prevent="onDrop(card.key)"
                @dragend="dragging = null"
              >
                <CardHeader class="pb-2">
                  <CardTitle class="text-sm font-medium text-muted-foreground flex items-center gap-1.5">
                    <span :class="{ 'line-through': editing && isHidden(card.key) }">{{ card.label }}</span>
                    <Info v-if="!editing" class="size-3.5 opacity-60" aria-hidden="true" />
                    <span v-else class="ml-auto flex items-center gap-1">
                      <button
                        type="button"
                        class="inline-flex size-7 items-center justify-center rounded border hover:bg-muted disabled:opacity-30"
                        :disabled="index === 0"
                        :aria-label="`Move ${card.label} earlier`"
                        :data-testid="`layout-left-${card.key}`"
                        @click="step(card.key, -1)"
                      ><ArrowLeft class="size-3.5" /></button>
                      <button
                        type="button"
                        class="inline-flex size-7 items-center justify-center rounded border hover:bg-muted disabled:opacity-30"
                        :disabled="index === visibleCards.length - 1"
                        :aria-label="`Move ${card.label} later`"
                        :data-testid="`layout-right-${card.key}`"
                        @click="step(card.key, 1)"
                      ><ArrowRight class="size-3.5" /></button>
                      <button
                        type="button"
                        class="inline-flex size-7 items-center justify-center rounded border hover:bg-muted"
                        :aria-label="`${isHidden(card.key) ? 'Show' : 'Hide'} ${card.label}`"
                        :aria-pressed="isHidden(card.key)"
                        :data-testid="`layout-eye-${card.key}`"
                        @click="toggle(card.key)"
                      >
                        <EyeOff v-if="isHidden(card.key)" class="size-3.5" />
                        <Eye v-else class="size-3.5" />
                      </button>
                    </span>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <Skeleton v-if="isPending" class="h-8 w-20" />
                  <p v-else class="text-2xl font-bold">{{ formatStatValue(card.key, stats?.[card.key]) }}</p>
                </CardContent>
              </Card>
            </TooltipTrigger>
            <TooltipContent class="max-w-xs text-left" :data-testid="`stat-hint-${card.key}`">
              {{ card.hint }}
            </TooltipContent>
          </Tooltip>
        </div>
        </TooltipProvider>
      </div>
    </div>
  </template>
