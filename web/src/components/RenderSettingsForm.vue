<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { Loader2, Check } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import RatingsOrderList from '@/components/RatingsOrderList.vue'
import LayoutEditor from '@/components/LayoutEditor.vue'
import type { SaveSettingsPayload, SourceColors } from '@/lib/api'
import { LANGUAGES, ALL_RATING_SOURCES, RATING_COLOR_ROWS, SOURCE_BADGE_SAMPLES, parseRatingsOrder, parseRatingsExclude } from '@/lib/constants'
import type { ImageLayout } from '@/lib/layout'
import { parseLayout, layoutToJSON, layoutTotal, defaultLayout } from '@/lib/layout'

export interface RenderSettings {
  image_source: string
  lang: string
  textless: boolean
  fanart_available: boolean
  ratings_limit: number
  ratings_order: string
  ratings_exclude: string
  is_default?: boolean
  poster_layout: string
  logo_ratings_limit: number
  backdrop_ratings_limit: number
  poster_badge_style: string
  logo_badge_style: string
  backdrop_badge_style: string
  poster_label_style: string
  logo_label_style: string
  backdrop_label_style: string
  poster_badge_direction: string
  poster_fit: string
  poster_text_size: number
  logo_text_size: number
  backdrop_text_size: number
  poster_badge_size: number
  logo_badge_size: number
  backdrop_badge_size: number
  poster_logo_size: number
  logo_logo_size: number
  backdrop_logo_size: number
  logo_layout: string
  backdrop_layout: string
  backdrop_badge_direction: string
  backdrop_edge_inset_x: number
  backdrop_edge_inset_y: number
  episode_ratings_limit: number
  episode_badge_style: string
  episode_label_style: string
  episode_text_size: number
  episode_badge_size: number
  episode_logo_size: number
  episode_layout: string
  episode_badge_direction: string
  episode_blur: boolean
  poster_badge_shape: string
  logo_badge_shape: string
  backdrop_badge_shape: string
  episode_badge_shape: string
  poster_badge_alpha: number
  logo_badge_alpha: number
  backdrop_badge_alpha: number
  episode_badge_alpha: number
  colors?: Record<string, SourceColors>
}

const props = withDefaults(defineProps<{
  settings: RenderSettings
  uid?: string
  showActions?: boolean
  loadSettings: () => Promise<RenderSettings | null>
  saveSettings: (s: SaveSettingsPayload) => Promise<string | null>
  resetSettings?: () => Promise<boolean>
  fetchPreview: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, badgeDirection?: string, textSize?: number, ratingsExclude?: string, layout?: string, badgeShape?: string, badgeAlpha?: number, posterFit?: string, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>) => Promise<Response>
  fetchLogoPreview?: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, badgeSize?: number, logoSize?: number, layout?: string, colors?: Record<string, SourceColors>) => Promise<Response>
  fetchBackdropPreview?: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, badgeDirection?: string, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, edgeInsetX?: number, edgeInsetY?: number, layout?: string, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>) => Promise<Response>
  fetchEpisodePreview?: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, badgeDirection?: string, blur?: boolean, layout?: string, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>) => Promise<Response>
}>(), {
  showActions: true,
})

const editFanart = ref(props.settings.image_source === 'f')
const editLang = ref(props.settings.lang || 'en')
const editTextless = ref(props.settings.textless)
const editSource = computed(() => editFanart.value ? 'f' : 't')
const editRatingsOrder = ref<string[]>(parseRatingsOrder(props.settings.ratings_order))
// Excluded sources are stored as only the explicitly-checked keys (unlike order,
// which is normalised to include every source).
const editRatingsExclude = ref<string[]>(parseRatingsExclude(props.settings.ratings_exclude))
const editPosterLayout = ref<ImageLayout>(parseLayout(props.settings.poster_layout, 'poster'))
const editPosterBadgeStyle = ref(props.settings.poster_badge_style || 'd')
const editLogoBadgeStyle = ref(props.settings.logo_badge_style || 'v')
const editBackdropBadgeStyle = ref(props.settings.backdrop_badge_style || 'v')
const editPosterLabelStyle = ref(props.settings.poster_label_style || 'o')
const editLogoLabelStyle = ref(props.settings.logo_label_style || 'o')
const editBackdropLabelStyle = ref(props.settings.backdrop_label_style || 'o')
const editPosterBadgeDirection = ref(props.settings.poster_badge_direction || 'd')
const editPosterFit = ref(props.settings.poster_fit || 'native')
const editPosterTextSize = ref(props.settings.poster_text_size ?? 100)
const editLogoTextSize = ref(props.settings.logo_text_size ?? 100)
const editBackdropTextSize = ref(props.settings.backdrop_text_size ?? 100)
const editPosterBadgeSize = ref(props.settings.poster_badge_size ?? 100)
const editLogoBadgeSize = ref(props.settings.logo_badge_size ?? 100)
const editBackdropBadgeSize = ref(props.settings.backdrop_badge_size ?? 100)
const editPosterLogoSize = ref(props.settings.poster_logo_size ?? 100)
const editLogoLogoSize = ref(props.settings.logo_logo_size ?? 100)
const editBackdropLogoSize = ref(props.settings.backdrop_logo_size ?? 100)
const editLogoLayout = ref<ImageLayout>(parseLayout(props.settings.logo_layout, 'logo'))
const editBackdropLayout = ref<ImageLayout>(parseLayout(props.settings.backdrop_layout, 'backdrop'))
const editBackdropBadgeDirection = ref(props.settings.backdrop_badge_direction || 'd')
const editBackdropEdgeInsetX = ref(props.settings.backdrop_edge_inset_x ?? 0)
const editBackdropEdgeInsetY = ref(props.settings.backdrop_edge_inset_y ?? 0)
const editEpisodeBadgeStyle = ref(props.settings.episode_badge_style || 'v')
const editEpisodeLabelStyle = ref(props.settings.episode_label_style || 'o')
const editEpisodeTextSize = ref(props.settings.episode_text_size ?? 100)
const editEpisodeBadgeSize = ref(props.settings.episode_badge_size ?? 100)
const editEpisodeLogoSize = ref(props.settings.episode_logo_size ?? 100)
const editEpisodeLayout = ref<ImageLayout>(parseLayout(props.settings.episode_layout, 'episode'))
const editEpisodeBadgeDirection = ref(props.settings.episode_badge_direction || 'v')
const editEpisodeBlur = ref(props.settings.episode_blur ?? false)
const editPosterBadgeShape = ref(props.settings.poster_badge_shape || 'r')
const editLogoBadgeShape = ref(props.settings.logo_badge_shape || 'r')
const editBackdropBadgeShape = ref(props.settings.backdrop_badge_shape || 'r')
const editEpisodeBadgeShape = ref(props.settings.episode_badge_shape || 'r')
const editPosterBadgeAlpha = ref(props.settings.poster_badge_alpha ?? 80)
const editLogoBadgeAlpha = ref(props.settings.logo_badge_alpha ?? 80)
const editBackdropBadgeAlpha = ref(props.settings.backdrop_badge_alpha ?? 80)
const editEpisodeBadgeAlpha = ref(props.settings.episode_badge_alpha ?? 80)
const editColors = ref<Record<string, SourceColors>>({ ...props.settings.colors })

// Which edges the current backdrop position anchors to. The horizontal inset
// only applies to left/right positions and the vertical inset only to top/bottom
// positions; centered axes hide their control (and the server ignores them).
const backdropShowVerticalInset = computed(() => true)
const backdropShowHorizontalInset = computed(() => true)
const backdropVerticalInsetLabel = computed(() => 'Space from edge')
const backdropHorizontalInsetLabel = computed(() => 'Space from edge')

const posterTotal = computed(() => layoutTotal(editPosterLayout.value))
const logoTotal = computed(() => layoutTotal(editLogoLayout.value))
const backdropTotal = computed(() => layoutTotal(editBackdropLayout.value))
const episodeTotal = computed(() => layoutTotal(editEpisodeLayout.value))

function applySettings(s: RenderSettings) {
  editFanart.value = s.image_source === 'f'
  editLang.value = s.lang || 'en'
  editTextless.value = s.textless
  editRatingsOrder.value = parseRatingsOrder(s.ratings_order)
  editRatingsExclude.value = parseRatingsExclude(s.ratings_exclude)
  editPosterLayout.value = parseLayout(s.poster_layout, 'poster')
  editPosterBadgeStyle.value = s.poster_badge_style || 'd'
  editLogoBadgeStyle.value = s.logo_badge_style || 'v'
  editBackdropBadgeStyle.value = s.backdrop_badge_style || 'v'
  editPosterLabelStyle.value = s.poster_label_style || 'o'
  editLogoLabelStyle.value = s.logo_label_style || 'o'
  editBackdropLabelStyle.value = s.backdrop_label_style || 'o'
  editPosterBadgeDirection.value = s.poster_badge_direction || 'd'
  editPosterFit.value = s.poster_fit || 'native'
  editPosterTextSize.value = s.poster_text_size ?? 100
  editLogoTextSize.value = s.logo_text_size ?? 100
  editBackdropTextSize.value = s.backdrop_text_size ?? 100
  editPosterBadgeSize.value = s.poster_badge_size ?? 100
  editLogoBadgeSize.value = s.logo_badge_size ?? 100
  editBackdropBadgeSize.value = s.backdrop_badge_size ?? 100
  editPosterLogoSize.value = s.poster_logo_size ?? 100
  editLogoLogoSize.value = s.logo_logo_size ?? 100
  editBackdropLogoSize.value = s.backdrop_logo_size ?? 100
  editBackdropBadgeDirection.value = s.backdrop_badge_direction || 'd'
  editBackdropEdgeInsetX.value = s.backdrop_edge_inset_x ?? 0
  editBackdropEdgeInsetY.value = s.backdrop_edge_inset_y ?? 0
  editEpisodeBadgeStyle.value = s.episode_badge_style || 'v'
  editEpisodeLabelStyle.value = s.episode_label_style || 'o'
  editEpisodeTextSize.value = s.episode_text_size ?? 100
  editEpisodeBadgeSize.value = s.episode_badge_size ?? 100
  editEpisodeLogoSize.value = s.episode_logo_size ?? 100
  editEpisodeBadgeDirection.value = s.episode_badge_direction || 'v'
  editEpisodeBlur.value = s.episode_blur ?? false
  editPosterBadgeShape.value = s.poster_badge_shape || 'r'
  editLogoBadgeShape.value = s.logo_badge_shape || 'r'
  editBackdropBadgeShape.value = s.backdrop_badge_shape || 'r'
  editEpisodeBadgeShape.value = s.episode_badge_shape || 'r'
  editPosterBadgeAlpha.value = s.poster_badge_alpha ?? 80
  editLogoBadgeAlpha.value = s.logo_badge_alpha ?? 80
  editBackdropBadgeAlpha.value = s.backdrop_badge_alpha ?? 80
  editEpisodeBadgeAlpha.value = s.episode_badge_alpha ?? 80
  editColors.value = { ...s.colors }
}

function setColor(sourceKey: string, attr: keyof SourceColors, value: string) {
  const cur = { ...editColors.value[sourceKey] }
  if (value === '') delete cur[attr]
  else cur[attr] = value
  editColors.value = { ...editColors.value, [sourceKey]: cur }
}

function borderEnabled(sourceKey: string): boolean {
  return !!(editColors.value[sourceKey]?.border)
}

function toggleBorder(sourceKey: string, on: boolean) {
  if (on) {
    setColor(sourceKey, 'border', editColors.value[sourceKey]?.border || '#ffffff')
  } else {
    setColor(sourceKey, 'border', '')
  }
}
const currentSettings = ref<RenderSettings>(props.settings)
const saving = ref(false)
const error = ref('')
const showCheck = ref(false)
let checkTimeout: ReturnType<typeof setTimeout> | null = null
let syncing = false

function editsSnapshot() {
  return {
    source: editSource.value,
    lang: editLang.value,
    textless: editTextless.value,
    ratings_order: editRatingsOrder.value.join(','),
    ratings_exclude: editRatingsExclude.value.join(','),
    poster_layout: layoutToJSON(editPosterLayout.value),
    poster_badge_style: editPosterBadgeStyle.value,
    logo_badge_style: editLogoBadgeStyle.value,
    backdrop_badge_style: editBackdropBadgeStyle.value,
    poster_label_style: editPosterLabelStyle.value,
    logo_label_style: editLogoLabelStyle.value,
    backdrop_label_style: editBackdropLabelStyle.value,
    poster_badge_direction: editPosterBadgeDirection.value,
    poster_fit: editPosterFit.value,
    poster_text_size: editPosterTextSize.value,
    logo_text_size: editLogoTextSize.value,
    backdrop_text_size: editBackdropTextSize.value,
    poster_badge_size: editPosterBadgeSize.value,
    logo_badge_size: editLogoBadgeSize.value,
    backdrop_badge_size: editBackdropBadgeSize.value,
    poster_logo_size: editPosterLogoSize.value,
    logo_logo_size: editLogoLogoSize.value,
    backdrop_logo_size: editBackdropLogoSize.value,
    logo_layout: layoutToJSON(editLogoLayout.value),
    backdrop_layout: layoutToJSON(editBackdropLayout.value),
    backdrop_badge_direction: editBackdropBadgeDirection.value,
    backdrop_edge_inset_x: editBackdropEdgeInsetX.value,
    backdrop_edge_inset_y: editBackdropEdgeInsetY.value,
    episode_badge_style: editEpisodeBadgeStyle.value,
    episode_label_style: editEpisodeLabelStyle.value,
    episode_text_size: editEpisodeTextSize.value,
    episode_badge_size: editEpisodeBadgeSize.value,
    episode_logo_size: editEpisodeLogoSize.value,
    episode_layout: layoutToJSON(editEpisodeLayout.value),
    episode_badge_direction: editEpisodeBadgeDirection.value,
    episode_blur: editEpisodeBlur.value,
    poster_badge_shape: editPosterBadgeShape.value,
    logo_badge_shape: editLogoBadgeShape.value,
    backdrop_badge_shape: editBackdropBadgeShape.value,
    episode_badge_shape: editEpisodeBadgeShape.value,
    poster_badge_alpha: editPosterBadgeAlpha.value,
    logo_badge_alpha: editLogoBadgeAlpha.value,
    backdrop_badge_alpha: editBackdropBadgeAlpha.value,
    episode_badge_alpha: editEpisodeBadgeAlpha.value,
    colors: editColors.value,
  }
}

function settingsSnapshot(s: RenderSettings) {
  return {
    source: s.image_source,
    lang: s.lang || 'en',
    textless: s.textless,
    ratings_order: parseRatingsOrder(s.ratings_order).join(','),
    ratings_exclude: parseRatingsExclude(s.ratings_exclude).join(','),
    poster_layout: layoutToJSON(parseLayout(s.poster_layout, 'poster')),
    poster_badge_style: s.poster_badge_style || 'd',
    logo_badge_style: s.logo_badge_style || 'v',
    backdrop_badge_style: s.backdrop_badge_style || 'v',
    poster_label_style: s.poster_label_style || 'o',
    logo_label_style: s.logo_label_style || 'o',
    backdrop_label_style: s.backdrop_label_style || 'o',
    poster_badge_direction: s.poster_badge_direction || 'd',
    poster_fit: s.poster_fit || 'native',
    poster_text_size: s.poster_text_size ?? 100,
    logo_text_size: s.logo_text_size ?? 100,
    backdrop_text_size: s.backdrop_text_size ?? 100,
    poster_badge_size: s.poster_badge_size ?? 100,
    logo_badge_size: s.logo_badge_size ?? 100,
    backdrop_badge_size: s.backdrop_badge_size ?? 100,
    poster_logo_size: s.poster_logo_size ?? 100,
    logo_logo_size: s.logo_logo_size ?? 100,
    backdrop_logo_size: s.backdrop_logo_size ?? 100,
    logo_layout: layoutToJSON(parseLayout(s.logo_layout, 'logo')),
    backdrop_layout: layoutToJSON(parseLayout(s.backdrop_layout, 'backdrop')),
    backdrop_badge_direction: s.backdrop_badge_direction || 'd',
    backdrop_edge_inset_x: s.backdrop_edge_inset_x ?? 0,
    backdrop_edge_inset_y: s.backdrop_edge_inset_y ?? 0,
    episode_badge_style: s.episode_badge_style || 'v',
    episode_label_style: s.episode_label_style || 'o',
    episode_text_size: s.episode_text_size ?? 100,
    episode_badge_size: s.episode_badge_size ?? 100,
    episode_logo_size: s.episode_logo_size ?? 100,
    episode_layout: layoutToJSON(parseLayout(s.episode_layout, 'episode')),
    episode_badge_direction: s.episode_badge_direction || 'v',
    episode_blur: s.episode_blur ?? false,
    poster_badge_shape: s.poster_badge_shape || 'r',
    logo_badge_shape: s.logo_badge_shape || 'r',
    backdrop_badge_shape: s.backdrop_badge_shape || 'r',
    episode_badge_shape: s.episode_badge_shape || 'r',
    poster_badge_alpha: s.poster_badge_alpha ?? 80,
    logo_badge_alpha: s.logo_badge_alpha ?? 80,
    backdrop_badge_alpha: s.backdrop_badge_alpha ?? 80,
    episode_badge_alpha: s.episode_badge_alpha ?? 80,
    colors: s.colors ?? {},
  }
}

const dirty = computed(() => JSON.stringify(editsSnapshot()) !== JSON.stringify(settingsSnapshot(currentSettings.value)))

watch(() => props.settings, (s) => {
  syncing = true
  const before = JSON.stringify(editsSnapshot())
  currentSettings.value = s
  applySettings(s)
  nextTick(() => {
    syncing = false
    // External settings updates (react-query refetch, post-save reload) are
    // applied while `syncing` blocks the edit watchers, so previews would
    // otherwise stay on their stale render. Refresh them when the applied
    // values actually changed.
    if (JSON.stringify(editsSnapshot()) !== before) {
      updateAllPreviews()
    }
  })
})

function revertEdits() {
  syncing = true
  applySettings(currentSettings.value)
  nextTick(() => { syncing = false })
}

// An emptied `v-model.number` input yields '' (and a partial entry can yield a
// non-integer); the backend types edge insets as a required i32, so send a clean,
// clamped integer to avoid a 400 mid-edit. Matches the server's 0–50 clamp.
function coerceInset(value: number): number {
  return Number.isFinite(value) ? Math.min(50, Math.max(0, Math.round(value))) : 0
}

async function save() {
  if (saving.value) return
  saving.value = true
  error.value = ''
  showCheck.value = false
  if (checkTimeout) clearTimeout(checkTimeout)
  try {
    const err = await props.saveSettings({
      image_source: editSource.value,
      lang: editLang.value,
      textless: editTextless.value,
        ratings_order: editRatingsOrder.value.join(','),
      ratings_exclude: editRatingsExclude.value.join(','),
      poster_layout: JSON.parse(JSON.stringify(editPosterLayout.value)),
          poster_badge_style: editPosterBadgeStyle.value,
      logo_badge_style: editLogoBadgeStyle.value,
      backdrop_badge_style: editBackdropBadgeStyle.value,
      poster_label_style: editPosterLabelStyle.value,
      logo_label_style: editLogoLabelStyle.value,
      backdrop_label_style: editBackdropLabelStyle.value,
      poster_badge_direction: editPosterBadgeDirection.value,
      poster_fit: editPosterFit.value,
      poster_text_size: editPosterTextSize.value,
      logo_text_size: editLogoTextSize.value,
      backdrop_text_size: editBackdropTextSize.value,
      poster_badge_size: editPosterBadgeSize.value,
      logo_badge_size: editLogoBadgeSize.value,
      backdrop_badge_size: editBackdropBadgeSize.value,
      poster_logo_size: editPosterLogoSize.value,
      logo_logo_size: editLogoLogoSize.value,
      backdrop_logo_size: editBackdropLogoSize.value,
      logo_layout: JSON.parse(JSON.stringify(editLogoLayout.value)),
      backdrop_layout: JSON.parse(JSON.stringify(editBackdropLayout.value)),
      backdrop_badge_direction: editBackdropBadgeDirection.value,
      backdrop_edge_inset_x: coerceInset(editBackdropEdgeInsetX.value),
      backdrop_edge_inset_y: coerceInset(editBackdropEdgeInsetY.value),
        episode_badge_style: editEpisodeBadgeStyle.value,
      episode_label_style: editEpisodeLabelStyle.value,
      episode_text_size: editEpisodeTextSize.value,
      episode_badge_size: editEpisodeBadgeSize.value,
      episode_logo_size: editEpisodeLogoSize.value,
      episode_layout: JSON.parse(JSON.stringify(editEpisodeLayout.value)),
      episode_badge_direction: editEpisodeBadgeDirection.value,
      episode_blur: editEpisodeBlur.value,
      poster_badge_shape: editPosterBadgeShape.value,
      logo_badge_shape: editLogoBadgeShape.value,
      backdrop_badge_shape: editBackdropBadgeShape.value,
      episode_badge_shape: editEpisodeBadgeShape.value,
      poster_badge_alpha: editPosterBadgeAlpha.value,
      logo_badge_alpha: editLogoBadgeAlpha.value,
      backdrop_badge_alpha: editBackdropBadgeAlpha.value,
      episode_badge_alpha: editEpisodeBadgeAlpha.value,
      colors: editColors.value,
    })
    if (err) {
      error.value = err
      revertEdits()
    } else {
      const updated = await props.loadSettings()
      if (updated) {
        syncing = true
        currentSettings.value = updated
        applySettings(updated)
        nextTick(() => { syncing = false })
      }
      showCheck.value = true
      checkTimeout = setTimeout(() => (showCheck.value = false), 1500)
    }
  } catch {
    error.value = 'Failed to save'
    revertEdits()
  } finally {
    saving.value = false
  }
}

function discard() {
  error.value = ''
  revertEdits()
}

defineExpose({ save, discard, dirty, saving, showCheck, error })

async function handleReset() {
  if (!props.resetSettings) return
  saving.value = true
  error.value = ''
  showCheck.value = false
  if (checkTimeout) clearTimeout(checkTimeout)
  try {
    const ok = await props.resetSettings()
    if (ok) {
      await props.loadSettings()
      showCheck.value = true
      checkTimeout = setTimeout(() => (showCheck.value = false), 1500)
    } else {
      error.value = 'Failed to reset'
    }
  } catch {
    error.value = 'Failed to reset'
  } finally {
    saving.value = false
  }
}

// --- Preview state for poster, logo, backdrop ---
interface PreviewState {
  src: string
  loading: boolean
  error: boolean
  size: { w: number; h: number } | null
  generation: number
}

function makePreviewState(): PreviewState {
  return { src: '', loading: false, error: false, size: null, generation: 0 }
}

const posterPreview = ref<PreviewState>(makePreviewState())
const logoPreview = ref<PreviewState>(makePreviewState())
const backdropPreview = ref<PreviewState>(makePreviewState())
const episodePreview = ref<PreviewState>(makePreviewState())

function onPreviewLoad(state: PreviewState, e: Event) {
  const img = e.target as HTMLImageElement
  if (img.naturalWidth && img.naturalHeight) {
    state.size = { w: img.naturalWidth, h: img.naturalHeight }
  }
  state.loading = false
  state.error = false
}

async function fetchPreviewImage(
  state: PreviewState,
  fetcher: (ratingsLimit: number, ratingsOrder: string) => Promise<Response>,
) {
  state.loading = true
  state.error = false
  const generation = ++state.generation

  try {
    const res = await fetcher(0, editRatingsOrder.value.join(','))
    if (generation !== state.generation) return
    if (!res.ok) {
      state.error = true
      state.loading = false
      return
    }
    const blob = await res.blob()
    if (generation !== state.generation) return
    if (state.src) URL.revokeObjectURL(state.src)
    state.src = URL.createObjectURL(blob)
  } catch {
    if (generation === state.generation) {
      state.error = true
      state.loading = false
    }
  }
}

let posterPreviewTimer: ReturnType<typeof setTimeout> | null = null
let logoPreviewTimer: ReturnType<typeof setTimeout> | null = null
let backdropPreviewTimer: ReturnType<typeof setTimeout> | null = null
let episodePreviewTimer: ReturnType<typeof setTimeout> | null = null

function updatePosterPreview() {
  fetchPreviewImage(posterPreview.value, (_limit, order) => props.fetchPreview(posterTotal.value, order, editPosterBadgeStyle.value, editPosterLabelStyle.value, editPosterBadgeDirection.value, editPosterTextSize.value, editRatingsExclude.value.join(','), layoutToJSON(editPosterLayout.value), editPosterBadgeShape.value, editPosterBadgeAlpha.value, editPosterFit.value, editPosterBadgeSize.value, editPosterLogoSize.value, editColors.value))
}

function updateLogoPreview() {
  if (props.fetchLogoPreview) {
    fetchPreviewImage(logoPreview.value, (_limit, order) => props.fetchLogoPreview!(logoTotal.value, order, editLogoBadgeStyle.value, editLogoLabelStyle.value, editLogoTextSize.value, editRatingsExclude.value.join(','), editLogoBadgeShape.value, editLogoBadgeAlpha.value, editLogoBadgeSize.value, editLogoLogoSize.value, layoutToJSON(editLogoLayout.value), editColors.value))
  }
}

function updateBackdropPreview() {
  if (props.fetchBackdropPreview) {
    fetchPreviewImage(backdropPreview.value, (_limit, order) => props.fetchBackdropPreview!(backdropTotal.value, order, editBackdropBadgeStyle.value, editBackdropLabelStyle.value, editBackdropTextSize.value, editBackdropBadgeDirection.value, editRatingsExclude.value.join(','), editBackdropBadgeShape.value, editBackdropBadgeAlpha.value, editBackdropEdgeInsetX.value, editBackdropEdgeInsetY.value, layoutToJSON(editBackdropLayout.value), editBackdropBadgeSize.value, editBackdropLogoSize.value, editColors.value))
  }
}

function updateEpisodePreview() {
  if (props.fetchEpisodePreview) {
    fetchPreviewImage(episodePreview.value, (_limit, order) => props.fetchEpisodePreview!(episodeTotal.value, order, editEpisodeBadgeStyle.value, editEpisodeLabelStyle.value, editEpisodeTextSize.value, editEpisodeBadgeDirection.value, editEpisodeBlur.value, layoutToJSON(editEpisodeLayout.value), editRatingsExclude.value.join(','), editEpisodeBadgeShape.value, editEpisodeBadgeAlpha.value, editEpisodeBadgeSize.value, editEpisodeLogoSize.value, editColors.value))
  }
}

function updateAllPreviews() {
  updatePosterPreview()
  updateLogoPreview()
  updateBackdropPreview()
  updateEpisodePreview()
}

// Global settings: refresh all previews (order and exclude affect every type)
watch([editRatingsOrder, editRatingsExclude, editColors], () => {
  if (syncing) return
  if (posterPreviewTimer) clearTimeout(posterPreviewTimer)
  if (logoPreviewTimer) clearTimeout(logoPreviewTimer)
  if (backdropPreviewTimer) clearTimeout(backdropPreviewTimer)
  if (episodePreviewTimer) clearTimeout(episodePreviewTimer)
  posterPreviewTimer = setTimeout(updatePosterPreview, 500)
  logoPreviewTimer = setTimeout(updateLogoPreview, 500)
  backdropPreviewTimer = setTimeout(updateBackdropPreview, 500)
  episodePreviewTimer = setTimeout(updateEpisodePreview, 500)
}, { deep: true })

// Poster-only settings
watch([editPosterLayout, editPosterBadgeStyle, editPosterLabelStyle, editPosterBadgeDirection, editPosterFit, editPosterTextSize, editPosterBadgeSize, editPosterLogoSize, editPosterBadgeShape, editPosterBadgeAlpha], () => {
  if (syncing) return
  if (posterPreviewTimer) clearTimeout(posterPreviewTimer)
  posterPreviewTimer = setTimeout(updatePosterPreview, 500)
})

// Logo-only settings
watch([editLogoLayout, editLogoBadgeStyle, editLogoLabelStyle, editLogoTextSize, editLogoBadgeSize, editLogoLogoSize, editLogoBadgeShape, editLogoBadgeAlpha], () => {
  if (syncing) return
  if (logoPreviewTimer) clearTimeout(logoPreviewTimer)
  logoPreviewTimer = setTimeout(updateLogoPreview, 500)
})

// Backdrop-only settings
watch([editBackdropLayout, editBackdropBadgeStyle, editBackdropLabelStyle, editBackdropTextSize, editBackdropBadgeSize, editBackdropLogoSize, editBackdropBadgeDirection, editBackdropEdgeInsetX, editBackdropEdgeInsetY, editBackdropBadgeShape, editBackdropBadgeAlpha], () => {
  if (syncing) return
  if (backdropPreviewTimer) clearTimeout(backdropPreviewTimer)
  backdropPreviewTimer = setTimeout(updateBackdropPreview, 500)
})

// Episode-only settings
watch([editEpisodeLayout, editEpisodeBadgeStyle, editEpisodeLabelStyle, editEpisodeTextSize, editEpisodeBadgeSize, editEpisodeLogoSize, editEpisodeBadgeDirection, editEpisodeBlur, editEpisodeBadgeShape, editEpisodeBadgeAlpha], () => {
  if (syncing) return
  if (episodePreviewTimer) clearTimeout(episodePreviewTimer)
  episodePreviewTimer = setTimeout(updateEpisodePreview, 500)
})

// Initial preview on mount
updateAllPreviews()

// The badge gallery mirrors the poster preview's render params (colors, alpha,
// shape, label style, order) so live, unsaved changes show up there too. The
// serialized params are debounced so dragging a colour picker doesn't trigger a
// reload per event.
const galleryParams = ref('')
let galleryTimer: ReturnType<typeof setTimeout> | null = null

function buildGalleryParams(): string {
  const p = new URLSearchParams()
  p.set('ratings_order', editRatingsOrder.value.join(','))
  p.set('ratings_exclude', editRatingsExclude.value.join(','))
  p.set('ratings_limit', String(posterTotal.value))
  p.set('badge_alpha', String(editPosterBadgeAlpha.value))
  p.set('badge_shape', editPosterBadgeShape.value)
  p.set('label_style', editPosterLabelStyle.value)
  const compact: Record<string, SourceColors> = {}
  for (const [k, c] of Object.entries(editColors.value)) {
    if (c.accent || c.value || c.border || c.text) compact[k] = c
  }
  if (Object.keys(compact).length) p.set('colors', JSON.stringify(compact))
  return p.toString()
}

galleryParams.value = buildGalleryParams()

watch([editRatingsOrder, editRatingsExclude, editPosterLayout, editPosterBadgeAlpha, editPosterBadgeShape, editPosterLabelStyle, editColors], () => {
  if (syncing) return
  if (galleryTimer) clearTimeout(galleryTimer)
  galleryTimer = setTimeout(() => {
    galleryParams.value = buildGalleryParams()
  }, 300)
}, { deep: true })

function badgePreviewUrl(sourceKey: string, value: string): string {
  return `/api/icons/badge?source=${encodeURIComponent(sourceKey)}&value=${encodeURIComponent(value)}&${galleryParams.value}`
}

// Close any open native colour picker when the user clicks outside it.
function onDocumentClick(e: MouseEvent) {
  const target = e.target as HTMLElement | null
  if (!target || target.closest('input[type="color"]')) return
  document.querySelectorAll('input[type="color"]').forEach((el) => (el as HTMLInputElement).blur())
}

onMounted(() => document.addEventListener('click', onDocumentClick))

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick)
  if (galleryTimer) clearTimeout(galleryTimer)
  if (posterPreviewTimer) clearTimeout(posterPreviewTimer)
  if (logoPreviewTimer) clearTimeout(logoPreviewTimer)
  if (backdropPreviewTimer) clearTimeout(backdropPreviewTimer)
  if (episodePreviewTimer) clearTimeout(episodePreviewTimer)
  if (posterPreview.value.src) URL.revokeObjectURL(posterPreview.value.src)
  if (logoPreview.value.src) URL.revokeObjectURL(logoPreview.value.src)
  if (backdropPreview.value.src) URL.revokeObjectURL(backdropPreview.value.src)
  if (episodePreview.value.src) URL.revokeObjectURL(episodePreview.value.src)
})

const inputId = (name: string) => props.uid ? `${name}-${props.uid}` : name

function isExcluded(key: string): boolean {
  return editRatingsExclude.value.includes(key)
}

function toggleExclude(key: string, checked: boolean) {
  const set = new Set(editRatingsExclude.value)
  if (checked) set.add(key)
  else set.delete(key)
  // Keep canonical source order so the saved value is stable regardless of click order.
  editRatingsExclude.value = ALL_RATING_SOURCES.map(s => s.key).filter(k => set.has(k))
}
</script>

<template>
  <div class="space-y-4">
    <div v-if="showActions !== false" class="flex flex-wrap items-center gap-3">
      <span v-if="showCheck" class="flex items-center gap-1.5 text-sm text-green-500">
        <Check class="size-4" />
        Saved
      </span>
      <span v-if="error" class="text-sm text-destructive">{{ error }}</span>
      <Button size="sm" :disabled="saving" data-testid="save-settings-button" @click="save">
        <Loader2 v-if="saving" class="size-4 animate-spin mr-1" />
        Save
      </Button>
      <Button v-if="dirty" variant="outline" size="sm" :disabled="saving" data-testid="discard-settings-button" @click="discard">
        Discard changes
      </Button>
      <Button
        v-if="resetSettings && !currentSettings.is_default"
        variant="outline"
        size="sm"
        :disabled="saving"
        @click="handleReset"
      >
        Reset to defaults
      </Button>
    </div>

    <Tabs default-value="image-settings" :unmount-on-hide="false">
      <TabsList class="h-auto flex-wrap">
        <TabsTrigger value="image-settings" data-testid="form-tab-image-settings">Image Settings</TabsTrigger>
        <TabsTrigger value="ratings" data-testid="form-tab-ratings">Ratings</TabsTrigger>
        <TabsTrigger value="poster" data-testid="form-tab-poster">Poster</TabsTrigger>
        <TabsTrigger value="logo" data-testid="form-tab-logo">Logo</TabsTrigger>
        <TabsTrigger value="backdrop" data-testid="form-tab-backdrop">Backdrop</TabsTrigger>
        <TabsTrigger value="episode" data-testid="form-tab-episode">Episode</TabsTrigger>
      </TabsList>

      <TabsContent value="image-settings" class="mt-3">
        <div class="rounded-md border p-4 space-y-3">
          <div class="flex items-center gap-2">
            <p class="text-sm font-semibold">Image Settings</p>
            <span
              v-if="resetSettings && currentSettings.is_default"
              class="text-xs bg-secondary text-secondary-foreground px-2 py-0.5 rounded"
            >
              Using defaults
            </span>
          </div>

    <!-- Image Settings: Language (always visible) -->
    <div class="space-y-1">
      <div class="flex items-center gap-3">
        <Label :for="inputId('lang')">Language</Label>
        <Select
          :model-value="editLang"
          @update:model-value="editLang = $event as string"
        >
          <SelectTrigger :id="inputId('lang')" class="max-w-[200px]" data-testid="lang-select">
            <SelectValue placeholder="Select language" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="lang in LANGUAGES" :key="lang.code" :value="lang.code">
              {{ lang.code }} - {{ lang.name }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
      <p class="text-xs text-muted-foreground">Best effort — falls back to English if unavailable.</p>
    </div>

    <!-- Fanart.tv preference -->
    <template v-if="currentSettings.fanart_available">
      <div class="flex items-center gap-2">
        <Checkbox
          :id="inputId('fanart')"
          :model-value="editFanart"
          data-testid="fanart-checkbox"
          @update:model-value="(v) => editFanart = !!v"
        />
        <Label :for="inputId('fanart')">Prefer Fanart.tv as image source</Label>
      </div>
    </template>
        </div>
      </TabsContent>

      <TabsContent value="ratings" class="mt-3">
        <div class="space-y-4">
          <div class="rounded-md border p-4 space-y-3">
            <p class="text-sm font-semibold">Rating Display</p>

      <div class="space-y-2">
        <Label>Rating order</Label>
        <p class="text-xs text-muted-foreground">Use the arrows to reorder. Higher items have priority.</p>
        <RatingsOrderList v-model="editRatingsOrder" />
      </div>

      <div class="space-y-2 pt-1">
        <Label>Exclude ratings</Label>
        <p class="text-xs text-muted-foreground">Hide specific rating sources entirely. An excluded source never appears, freeing its slot for the next preferred source.</p>
        <div class="grid grid-cols-1 gap-x-4 gap-y-1.5 max-w-sm sm:grid-cols-2">
          <div
            v-for="source in ALL_RATING_SOURCES"
            :key="source.key"
            class="flex items-center gap-2"
          >
            <Checkbox
              :id="inputId(`exclude-${source.key}`)"
              :model-value="isExcluded(source.key)"
              :data-testid="`exclude-${source.key}-checkbox`"
              @update:model-value="(v) => toggleExclude(source.key, !!v)"
            />
            <span
              class="inline-block w-2.5 h-2.5 rounded-full shrink-0"
              :style="{ backgroundColor: source.color }"
            ></span>
            <Label :for="inputId(`exclude-${source.key}`)" class="text-sm font-normal">{{ source.label }}</Label>
          </div>
        </div>
      </div>
    </div>

    <!-- Rating Colours (global only) -->
    <template v-if="uid === 'global'">
      <div class="rounded-md border p-4 space-y-3">
        <p class="text-sm font-semibold">Rating Colours</p>
        <p class="text-xs text-muted-foreground">Customize each rating source's badge colours. The badge background opacity is set by the background-opacity sliders above. Changes apply after saving.</p>
        <div class="space-y-1 text-xs text-muted-foreground">
          <span class="flex items-center gap-2">
            <span aria-hidden="true">-</span>
            Logo background — background behind the rating source logo
          </span>
          <span class="flex items-center gap-2">
            <span aria-hidden="true">-</span>
            Text background — background behind the rating number
          </span>
          <span class="flex items-center gap-2">
            <span aria-hidden="true">-</span>
            Border — outline around the badge (off by default)
          </span>
          <span class="flex items-center gap-2">
            <span aria-hidden="true">-</span>
            Text colour — colour of the rating number
          </span>
        </div>
        <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <div class="grid grid-cols-1 gap-x-4 gap-y-2">
          <div
            v-for="source in RATING_COLOR_ROWS"
            :key="source.key"
            class="flex items-center gap-2"
          >
            <Label class="w-44 text-sm font-normal shrink-0">{{ source.label }}</Label>
            <input
              type="color"
              :value="(editColors[source.key] && (editColors[source.key] as SourceColors).accent) || '#000000'"
              :aria-label="`${source.label} logo background`"
              :title="`${source.label} — logo background`"
              class="h-7 w-9 shrink-0 cursor-pointer rounded border border-input bg-transparent p-0.5"
              @input="setColor(source.key, 'accent', ($event.target as HTMLInputElement).value)"
            />
            <input
              type="color"
              :value="(editColors[source.key] && (editColors[source.key] as SourceColors).value) || '#000000'"
              :aria-label="`${source.label} text background`"
              :title="`${source.label} — text background`"
              class="h-7 w-9 shrink-0 cursor-pointer rounded border border-input bg-transparent p-0.5"
              @input="setColor(source.key, 'value', ($event.target as HTMLInputElement).value)"
            />
            <Checkbox
              :id="inputId(`border-${source.key}`)"
              :model-value="borderEnabled(source.key)"
              :aria-label="`${source.label} border on/off`"
              :title="`${source.label}: toggle border`"
              data-testid="border-toggle"
              @update:model-value="(v) => toggleBorder(source.key, !!v)"
            />
            <input
              type="color"
              :value="(editColors[source.key] && (editColors[source.key] as SourceColors).border) || '#ffffff'"
              :disabled="!borderEnabled(source.key)"
              :aria-label="`${source.label} border color`"
              :title="`${source.label} border — outline around the badge`"
              class="h-7 w-9 shrink-0 cursor-pointer rounded border border-input bg-transparent p-0.5 disabled:cursor-not-allowed disabled:opacity-40"
              @input="setColor(source.key, 'border', ($event.target as HTMLInputElement).value)"
            />
            <input
              type="color"
              :value="(editColors[source.key] && (editColors[source.key] as SourceColors).text) || '#ffffff'"
              :aria-label="`${source.label} text colour`"
              :title="`${source.label} — text colour`"
              class="h-7 w-9 shrink-0 cursor-pointer rounded border border-input bg-transparent p-0.5"
              @input="setColor(source.key, 'text', ($event.target as HTMLInputElement).value)"
            />
            <span class="text-[10px] text-muted-foreground whitespace-nowrap">logo bg · text bg · border · text</span>
          </div>
        </div>
        <div class="space-y-3">
          <!-- Rating Source Badges -->
          <p class="text-sm font-semibold">Rating Source Badges</p>
        <p class="text-xs text-muted-foreground">
          A sample badge for each rating source, matching your current colour settings (updates live
          with the controls beside it). Sources with multiple logos — Rotten Tomatoes switches logo based
          on the score — show every variant.
        </p>
        <div class="space-y-3">
          <div
            v-for="source in ALL_RATING_SOURCES"
            :key="source.key"
            class="flex items-start gap-3"
          >
            <span class="w-44 shrink-0 pt-1 text-sm font-normal">{{ source.label }}</span>
            <div class="flex flex-wrap items-start gap-x-4 gap-y-2">
              <div v-for="sample in SOURCE_BADGE_SAMPLES[source.key] ?? []" :key="sample.value" class="flex flex-col items-center gap-1">
                <img
                  :src="badgePreviewUrl(source.key, sample.value)"
                  :alt="`${source.label} badge (${sample.label})`"
                  :title="`${source.label} badge — ${sample.label}`"
                  class="h-10 w-auto"
                />
                <span class="text-[10px] text-muted-foreground">{{ sample.label }}</span>
              </div>
            </div>
          </div>
        </div>
        </div>
        </div>
      </div>
    </template>
        </div>
      </TabsContent>

      <TabsContent value="poster" class="mt-3">
        <div class="rounded-md border p-4 space-y-3">
          <p class="text-sm font-semibold">Poster</p>
          <div class="flex flex-col md:flex-row gap-4 items-start">
            <div class="relative w-[220px] shrink-0" :style="posterPreview.size ? { aspectRatio: `${posterPreview.size.w} / ${posterPreview.size.h}` } : undefined">
              <img
                v-show="posterPreview.src && !posterPreview.error"
                :src="posterPreview.src"
                alt="Poster preview"
                class="rounded border w-full"
                @load="(e: Event) => onPreviewLoad(posterPreview, e)"
                @error="posterPreview.loading = false; posterPreview.error = true"
              />
              <p v-if="posterPreview.error && !posterPreview.loading" class="text-sm text-muted-foreground py-4">Failed</p>
              <div v-if="posterPreview.loading" class="absolute inset-0 flex items-center justify-center rounded">
                <Loader2 class="size-5 animate-spin text-white drop-shadow-md" />
              </div>
            </div>
            <LayoutEditor v-model="editPosterLayout" kind="poster" test-prefix="poster" class="flex-1 min-w-0" />
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-lg">
            <div class="space-y-2">
              <Label :for="inputId('poster-badge-style')">Badge style</Label>
              <Select
                :model-value="editPosterBadgeStyle"
                :disabled="editPosterBadgeShape === 'p'"
                @update:model-value="editPosterBadgeStyle = $event as string"
              >
                <SelectTrigger :id="inputId('poster-badge-style')" class="max-w-xs" data-testid="poster-badge-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="d">Default</SelectItem>
                  <SelectItem value="h">Horizontal</SelectItem>
                  <SelectItem value="v">Vertical</SelectItem>
                </SelectContent>
              </Select>
              <p v-if="editPosterBadgeShape === 'p'" class="text-xs text-muted-foreground">Pills always render horizontally.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('poster-label-style')">Label style</Label>
              <Select
                :model-value="editPosterLabelStyle"
                @update:model-value="editPosterLabelStyle = $event as string"
              >
                <SelectTrigger :id="inputId('poster-label-style')" class="max-w-xs" data-testid="poster-label-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="t">Text</SelectItem>
                  <SelectItem value="i">White</SelectItem>
                  <SelectItem value="o">Official</SelectItem>
                  <SelectItem value="h">High Res</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('poster-text-size')">Text size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editPosterTextSize }}%</span>
              </div>
              <input
                :id="inputId('poster-text-size')"
                v-model.number="editPosterTextSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`poster-text-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating text font size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('poster-badge-size')">Badge size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editPosterBadgeSize }}%</span>
              </div>
              <input
                :id="inputId('poster-badge-size')"
                v-model.number="editPosterBadgeSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`poster-badge-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Overall badge size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('poster-logo-size')">Logo size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editPosterLogoSize }}%</span>
              </div>
              <input
                :id="inputId('poster-logo-size')"
                v-model.number="editPosterLogoSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`poster-logo-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating source logo size. The badge auto-sizes to fit, so nothing overflows.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('poster-badge-shape')">Badge shape</Label>
              <Select
                :model-value="editPosterBadgeShape"
                @update:model-value="editPosterBadgeShape = $event as string"
              >
                <SelectTrigger :id="inputId('poster-badge-shape')" class="max-w-xs" data-testid="poster-badge-shape-select">
                  <SelectValue placeholder="Select shape" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="r">Rounded</SelectItem>
                  <SelectItem value="p">Pill</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('poster-badge-alpha')">Background opacity</Label>
                <span class="text-xs text-muted-foreground w-8 text-right">{{ editPosterBadgeAlpha }}%</span>
              </div>
              <input
                :id="inputId('poster-badge-alpha')"
                v-model.number="editPosterBadgeAlpha"
                type="range"
                min="0"
                max="100"
                step="5"
                :data-testid="`poster-badge-alpha-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">0% = no background, 100% = opaque black</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('poster-badge-direction')">Badge direction</Label>
              <Select
                :model-value="editPosterBadgeDirection"
                @update:model-value="editPosterBadgeDirection = $event as string"
              >
                <SelectTrigger :id="inputId('poster-badge-direction')" class="max-w-xs" data-testid="poster-badge-direction-select">
                  <SelectValue placeholder="Select direction" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="d">Default</SelectItem>
                  <SelectItem value="h">Horizontal</SelectItem>
                  <SelectItem value="v">Vertical</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('poster-fit')">Aspect ratio</Label>
              <Select
                :model-value="editPosterFit"
                @update:model-value="editPosterFit = $event as string"
              >
                <SelectTrigger :id="inputId('poster-fit')" class="max-w-xs" data-testid="poster-fit-select">
                  <SelectValue placeholder="Select fit" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="native">Native (source ratio)</SelectItem>
                  <SelectItem value="cover">Crop to 2:3</SelectItem>
                  <SelectItem value="blur">Blur fill to 2:3</SelectItem>
                  <SelectItem value="pad">Letterbox to 2:3</SelectItem>
                </SelectContent>
              </Select>
              <p class="text-xs text-muted-foreground">
                How non-2:3 posters are fit to the standard 2:3 frame so clients don't crop them.
              </p>
            </div>
            <div class="flex items-center gap-2">
              <Checkbox
                :id="inputId('textless')"
                :model-value="editTextless"
                data-testid="textless-checkbox"
                @update:model-value="(v) => editTextless = !!v"
              />
              <Label :for="inputId('textless')">Prefer textless posters</Label>
            </div>
          </div>
        </div>
      </TabsContent>

      <TabsContent value="logo" class="mt-3">
        <div v-if="fetchLogoPreview" class="rounded-md border p-4 space-y-3">
          <p class="text-sm font-semibold">Logo</p>
          <div class="flex flex-col md:flex-row gap-4 items-start">
            <div class="relative w-[240px] shrink-0" :style="logoPreview.size ? { aspectRatio: `${logoPreview.size.w} / ${logoPreview.size.h}` } : undefined">
              <img
                v-show="logoPreview.src && !logoPreview.error"
                :src="logoPreview.src"
                alt="Logo preview"
                class="rounded border w-full bg-neutral-900"
                @load="(e: Event) => onPreviewLoad(logoPreview, e)"
                @error="logoPreview.loading = false; logoPreview.error = true"
              />
              <p v-if="logoPreview.error && !logoPreview.loading" class="text-sm text-muted-foreground py-4">Failed</p>
              <div v-if="logoPreview.loading" class="absolute inset-0 flex items-center justify-center rounded">
                <Loader2 class="size-5 animate-spin text-white drop-shadow-md" />
              </div>
            </div>
            <LayoutEditor v-model="editLogoLayout" kind="logo" test-prefix="logo" class="flex-1 min-w-0" />
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-lg">
            <div class="space-y-2">
              <Label :for="inputId('logo-badge-style')">Badge style</Label>
              <Select
                :model-value="editLogoBadgeStyle"
                :disabled="editLogoBadgeShape === 'p'"
                @update:model-value="editLogoBadgeStyle = $event as string"
              >
                <SelectTrigger :id="inputId('logo-badge-style')" class="max-w-xs" data-testid="logo-badge-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="d">Auto</SelectItem>
                  <SelectItem value="lr">Logo left · value right</SelectItem>
                  <SelectItem value="rl">Value left · logo right</SelectItem>
                  <SelectItem value="tb">Logo top · value bottom</SelectItem>
                  <SelectItem value="bt">Value top · logo bottom</SelectItem>
                </SelectContent>
              </Select>
              <p v-if="editLogoBadgeShape === 'p'" class="text-xs text-muted-foreground">Pills always render horizontally.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('logo-label-style')">Label style</Label>
              <Select
                :model-value="editLogoLabelStyle"
                @update:model-value="editLogoLabelStyle = $event as string"
              >
                <SelectTrigger :id="inputId('logo-label-style')" class="max-w-xs" data-testid="logo-label-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="t">Text</SelectItem>
                  <SelectItem value="i">White</SelectItem>
                  <SelectItem value="o">Official</SelectItem>
                  <SelectItem value="h">High Res</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('logo-text-size')">Text size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editLogoTextSize }}%</span>
              </div>
              <input
                :id="inputId('logo-text-size')"
                v-model.number="editLogoTextSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`logo-text-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating text font size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('logo-badge-size')">Badge size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editLogoBadgeSize }}%</span>
              </div>
              <input
                :id="inputId('logo-badge-size')"
                v-model.number="editLogoBadgeSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`logo-badge-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Overall badge size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('logo-logo-size')">Logo size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editLogoLogoSize }}%</span>
              </div>
              <input
                :id="inputId('logo-logo-size')"
                v-model.number="editLogoLogoSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`logo-logo-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating source logo size. The badge auto-sizes to fit, so nothing overflows.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('logo-badge-shape')">Badge shape</Label>
              <Select
                :model-value="editLogoBadgeShape"
                @update:model-value="editLogoBadgeShape = $event as string"
              >
                <SelectTrigger :id="inputId('logo-badge-shape')" class="max-w-xs" data-testid="logo-badge-shape-select">
                  <SelectValue placeholder="Select shape" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="r">Rounded</SelectItem>
                  <SelectItem value="p">Pill</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('logo-badge-alpha')">Background opacity</Label>
                <span class="text-xs text-muted-foreground w-8 text-right">{{ editLogoBadgeAlpha }}%</span>
              </div>
              <input
                :id="inputId('logo-badge-alpha')"
                v-model.number="editLogoBadgeAlpha"
                type="range"
                min="0"
                max="100"
                step="5"
                :data-testid="`logo-badge-alpha-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">0% = no background, 100% = opaque black</p>
            </div>
          </div>
        </div>
      </TabsContent>

      <TabsContent value="backdrop" class="mt-3">
        <div v-if="fetchBackdropPreview" class="rounded-md border p-4 space-y-3">
          <p class="text-sm font-semibold">Backdrop (movie/series)</p>
          <div class="flex flex-col md:flex-row gap-4 items-start">
            <div class="relative w-[360px] shrink-0" :style="backdropPreview.size ? { aspectRatio: `${backdropPreview.size.w} / ${backdropPreview.size.h}` } : undefined">
              <img
                v-show="backdropPreview.src && !backdropPreview.error"
                :src="backdropPreview.src"
                alt="Backdrop preview"
                class="rounded border w-full"
                @load="(e: Event) => onPreviewLoad(backdropPreview, e)"
                @error="backdropPreview.loading = false; backdropPreview.error = true"
              />
              <p v-if="backdropPreview.error && !backdropPreview.loading" class="text-sm text-muted-foreground py-4">Failed</p>
              <div v-if="backdropPreview.loading" class="absolute inset-0 flex items-center justify-center rounded">
                <Loader2 class="size-5 animate-spin text-white drop-shadow-md" />
              </div>
            </div>
            <LayoutEditor v-model="editBackdropLayout" kind="backdrop" test-prefix="backdrop" class="flex-1 min-w-0" />
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-lg">
            <div class="space-y-2">
              <Label :for="inputId('backdrop-badge-style')">Badge style</Label>
              <Select
                :model-value="editBackdropBadgeStyle"
                :disabled="editBackdropBadgeShape === 'p'"
                @update:model-value="editBackdropBadgeStyle = $event as string"
              >
                <SelectTrigger :id="inputId('backdrop-badge-style')" class="max-w-xs" data-testid="backdrop-badge-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="d">Auto</SelectItem>
                  <SelectItem value="lr">Logo left · value right</SelectItem>
                  <SelectItem value="rl">Value left · logo right</SelectItem>
                  <SelectItem value="tb">Logo top · value bottom</SelectItem>
                  <SelectItem value="bt">Value top · logo bottom</SelectItem>
                </SelectContent>
              </Select>
              <p v-if="editBackdropBadgeShape === 'p'" class="text-xs text-muted-foreground">Pills always render horizontally.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('backdrop-label-style')">Label style</Label>
              <Select
                :model-value="editBackdropLabelStyle"
                @update:model-value="editBackdropLabelStyle = $event as string"
              >
                <SelectTrigger :id="inputId('backdrop-label-style')" class="max-w-xs" data-testid="backdrop-label-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="t">Text</SelectItem>
                  <SelectItem value="i">White</SelectItem>
                  <SelectItem value="o">Official</SelectItem>
                  <SelectItem value="h">High Res</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('backdrop-text-size')">Text size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editBackdropTextSize }}%</span>
              </div>
              <input
                :id="inputId('backdrop-text-size')"
                v-model.number="editBackdropTextSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`backdrop-text-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating text font size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('backdrop-badge-size')">Badge size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editBackdropBadgeSize }}%</span>
              </div>
              <input
                :id="inputId('backdrop-badge-size')"
                v-model.number="editBackdropBadgeSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`backdrop-badge-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Overall badge size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('backdrop-logo-size')">Logo size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editBackdropLogoSize }}%</span>
              </div>
              <input
                :id="inputId('backdrop-logo-size')"
                v-model.number="editBackdropLogoSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`backdrop-logo-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating source logo size. The badge auto-sizes to fit, so nothing overflows.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('backdrop-badge-shape')">Badge shape</Label>
              <Select
                :model-value="editBackdropBadgeShape"
                @update:model-value="editBackdropBadgeShape = $event as string"
              >
                <SelectTrigger :id="inputId('backdrop-badge-shape')" class="max-w-xs" data-testid="backdrop-badge-shape-select">
                  <SelectValue placeholder="Select shape" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="r">Rounded</SelectItem>
                  <SelectItem value="p">Pill</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('backdrop-badge-alpha')">Background opacity</Label>
                <span class="text-xs text-muted-foreground w-8 text-right">{{ editBackdropBadgeAlpha }}%</span>
              </div>
              <input
                :id="inputId('backdrop-badge-alpha')"
                v-model.number="editBackdropBadgeAlpha"
                type="range"
                min="0"
                max="100"
                step="5"
                :data-testid="`backdrop-badge-alpha-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">0% = no background, 100% = opaque black</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('backdrop-badge-direction')">Badge direction</Label>
              <Select
                :model-value="editBackdropBadgeDirection"
                @update:model-value="editBackdropBadgeDirection = $event as string"
              >
                <SelectTrigger :id="inputId('backdrop-badge-direction')" class="max-w-xs" data-testid="backdrop-badge-direction-select">
                  <SelectValue placeholder="Select direction" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="d">Default</SelectItem>
                  <SelectItem value="h">Horizontal</SelectItem>
                  <SelectItem value="v">Vertical</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div v-if="backdropShowVerticalInset || backdropShowHorizontalInset" class="space-y-1">
              <Label>Distance from edge</Label>
              <div class="flex flex-wrap gap-4">
                <div v-if="backdropShowVerticalInset" class="space-y-1">
                  <Label :for="inputId('backdrop-edge-inset-y')" class="text-xs font-normal text-muted-foreground">{{ backdropVerticalInsetLabel }}</Label>
                  <div class="flex items-center gap-1">
                    <Input
                      :id="inputId('backdrop-edge-inset-y')"
                      v-model.number="editBackdropEdgeInsetY"
                      type="number"
                      :min="0"
                      :max="50"
                      class="w-[80px]"
                      data-testid="backdrop-edge-inset-y"
                    />
                    <span class="text-xs text-muted-foreground">%</span>
                  </div>
                </div>
                <div v-if="backdropShowHorizontalInset" class="space-y-1">
                  <Label :for="inputId('backdrop-edge-inset-x')" class="text-xs font-normal text-muted-foreground">{{ backdropHorizontalInsetLabel }}</Label>
                  <div class="flex items-center gap-1">
                    <Input
                      :id="inputId('backdrop-edge-inset-x')"
                      v-model.number="editBackdropEdgeInsetX"
                      type="number"
                      :min="0"
                      :max="50"
                      class="w-[80px]"
                      data-testid="backdrop-edge-inset-x"
                    />
                    <span class="text-xs text-muted-foreground">%</span>
                  </div>
                </div>
              </div>
              <p class="text-xs text-muted-foreground">Inset ratings from the image edge (% of size) — useful when a player crops the backdrop.</p>
            </div>
          </div>
        </div>
      </TabsContent>

      <TabsContent value="episode" class="mt-3">
        <div v-if="fetchEpisodePreview" class="rounded-md border p-4 space-y-3">
          <p class="text-sm font-semibold">Episode</p>
          <div class="flex flex-col md:flex-row gap-4 items-start">
            <div class="relative w-[360px] shrink-0" :style="episodePreview.size ? { aspectRatio: `${episodePreview.size.w} / ${episodePreview.size.h}` } : undefined">
              <img
                v-show="episodePreview.src && !episodePreview.error"
                :src="episodePreview.src"
                alt="Episode preview"
                class="rounded border w-full"
                @load="(e: Event) => onPreviewLoad(episodePreview, e)"
                @error="episodePreview.loading = false; episodePreview.error = true"
              />
              <p v-if="episodePreview.error && !episodePreview.loading" class="text-sm text-muted-foreground py-4">Failed</p>
              <div v-if="episodePreview.loading" class="absolute inset-0 flex items-center justify-center rounded">
                <Loader2 class="size-5 animate-spin text-white drop-shadow-md" />
              </div>
            </div>
            <LayoutEditor v-model="editEpisodeLayout" kind="episode" test-prefix="episode" class="flex-1 min-w-0" />
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-lg">
            <div class="space-y-2">
              <Label :for="inputId('episode-badge-style')">Badge style</Label>
              <Select
                :model-value="editEpisodeBadgeStyle"
                :disabled="editEpisodeBadgeShape === 'p'"
                @update:model-value="editEpisodeBadgeStyle = $event as string"
              >
                <SelectTrigger :id="inputId('episode-badge-style')" class="max-w-xs" data-testid="episode-badge-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="d">Auto</SelectItem>
                  <SelectItem value="lr">Logo left · value right</SelectItem>
                  <SelectItem value="rl">Value left · logo right</SelectItem>
                  <SelectItem value="tb">Logo top · value bottom</SelectItem>
                  <SelectItem value="bt">Value top · logo bottom</SelectItem>
                </SelectContent>
              </Select>
              <p v-if="editEpisodeBadgeShape === 'p'" class="text-xs text-muted-foreground">Pills always render horizontally.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('episode-label-style')">Label style</Label>
              <Select
                :model-value="editEpisodeLabelStyle"
                @update:model-value="editEpisodeLabelStyle = $event as string"
              >
                <SelectTrigger :id="inputId('episode-label-style')" class="max-w-xs" data-testid="episode-label-style-select">
                  <SelectValue placeholder="Select style" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="t">Text</SelectItem>
                  <SelectItem value="i">White</SelectItem>
                  <SelectItem value="o">Official</SelectItem>
                  <SelectItem value="h">High Res</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('episode-text-size')">Text size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editEpisodeTextSize }}%</span>
              </div>
              <input
                :id="inputId('episode-text-size')"
                v-model.number="editEpisodeTextSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`episode-text-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating text font size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('episode-badge-size')">Badge size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editEpisodeBadgeSize }}%</span>
              </div>
              <input
                :id="inputId('episode-badge-size')"
                v-model.number="editEpisodeBadgeSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`episode-badge-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Overall badge size. 100% = default.</p>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('episode-logo-size')">Logo size</Label>
                <span class="text-xs text-muted-foreground w-10 text-right">{{ editEpisodeLogoSize }}%</span>
              </div>
              <input
                :id="inputId('episode-logo-size')"
                v-model.number="editEpisodeLogoSize"
                type="range"
                min="50"
                max="400"
                step="1"
                :data-testid="`episode-logo-size-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">Rating source logo size. The badge auto-sizes to fit, so nothing overflows.</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('episode-badge-shape')">Badge shape</Label>
              <Select
                :model-value="editEpisodeBadgeShape"
                @update:model-value="editEpisodeBadgeShape = $event as string"
              >
                <SelectTrigger :id="inputId('episode-badge-shape')" class="max-w-xs" data-testid="episode-badge-shape-select">
                  <SelectValue placeholder="Select shape" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="r">Rounded</SelectItem>
                  <SelectItem value="p">Pill</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <Label :for="inputId('episode-badge-alpha')">Background opacity</Label>
                <span class="text-xs text-muted-foreground w-8 text-right">{{ editEpisodeBadgeAlpha }}%</span>
              </div>
              <input
                :id="inputId('episode-badge-alpha')"
                v-model.number="editEpisodeBadgeAlpha"
                type="range"
                min="0"
                max="100"
                step="5"
                :data-testid="`episode-badge-alpha-slider`"
                class="w-full max-w-xs accent-primary"
              />
              <p class="text-xs text-muted-foreground">0% = no background, 100% = opaque black</p>
            </div>
            <div class="space-y-2">
              <Label :for="inputId('episode-badge-direction')">Direction</Label>
              <Select
                :model-value="editEpisodeBadgeDirection"
                @update:model-value="editEpisodeBadgeDirection = $event as string"
              >
                <SelectTrigger :id="inputId('episode-badge-direction')" class="max-w-xs" data-testid="episode-badge-direction-select">
                  <SelectValue placeholder="Select direction" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="d">Auto</SelectItem>
                  <SelectItem value="lr">Logo left · value right</SelectItem>
                  <SelectItem value="rl">Value left · logo right</SelectItem>
                  <SelectItem value="tb">Logo top · value bottom</SelectItem>
                  <SelectItem value="bt">Value top · logo bottom</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="flex items-center gap-2">
              <Checkbox
                :id="inputId('episode-blur')"
                :model-value="editEpisodeBlur"
                data-testid="episode-blur-checkbox"
                @update:model-value="(v) => editEpisodeBlur = !!v"
              />
              <Label :for="inputId('episode-blur')">Blur (spoiler protection)</Label>
            </div>
          </div>
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
