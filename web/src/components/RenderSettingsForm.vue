<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useSavedFlash } from '@/composables/useSavedFlash'
import { Loader2, Check } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import KindSettingsPanel from '@/components/KindSettingsPanel.vue'
import type { PreviewKind, PreviewParams, RenderSettings, SaveSettingsPayload, SourceColors } from '@/lib/settings'
import { LANGUAGES, ALL_RATING_SOURCES, RATING_COLOR_ROWS, SOURCE_BADGE_SAMPLES, parseRatingsOrder, parseRatingsExclude } from '@/lib/constants'
import type { ImageLayout } from '@/lib/layout'
import { parseLayout, layoutToJSON, layoutTotal } from '@/lib/layout'
import { colorsQuery } from '@/lib/api'



const props = withDefaults(defineProps<{
  settings: RenderSettings
  uid?: string
  showActions?: boolean
  loadSettings: () => Promise<RenderSettings | null>
  saveSettings: (s: SaveSettingsPayload) => Promise<string | null>
  resetSettings?: () => Promise<boolean>
  fetchPreview: (kind: PreviewKind, params: PreviewParams) => Promise<Response>
}>(), {
  showActions: true,
})

// The backend's DefaultPosterBadgeStyle is 'd' (Auto) and legacy 'h' values
// still come back from older installs, but the poster badge-style select
// intentionally offers only the four concrete directions (no Auto option).
// Normalize those to 'lr' (the render path already treats 'd'/horizontal as
// lr) so a fresh install doesn't render the select blank. The other badge
// styles (logo/backdrop/episode) keep their valid tb/bt defaults untouched.
function normalizePosterBadgeStyle(value: string | undefined): string {
  return value === 'd' || value === 'h' || !value ? 'lr' : value
}

const TAB_STORAGE_KEY = 'render-settings-active-tab'
const activeTab = ref(localStorage.getItem(TAB_STORAGE_KEY) || 'image-settings')
watch(activeTab, (v) => localStorage.setItem(TAB_STORAGE_KEY, v))

const editFanart = ref(props.settings.image_source === 'f')
const editLang = ref(props.settings.lang || 'en')
const editTextless = ref(props.settings.textless)
const editSource = computed(() => editFanart.value ? 'f' : 't')
const editRatingsOrder = ref<string[]>(parseRatingsOrder(props.settings.ratings_order))
// Excluded sources are stored as only the explicitly-checked keys (unlike order,
// which is normalised to include every source).
const editRatingsExclude = ref<string[]>(parseRatingsExclude(props.settings.ratings_exclude))
const editPosterLayout = ref<ImageLayout>(parseLayout(props.settings.poster_layout, 'poster'))
const editPosterBadgeStyle = ref(normalizePosterBadgeStyle(props.settings.poster_badge_style))
const editLogoBadgeStyle = ref(props.settings.logo_badge_style || 'v')
const editBackdropBadgeStyle = ref(props.settings.backdrop_badge_style || 'v')
const editPosterLabelStyle = ref(props.settings.poster_label_style || 'o')
const editLogoLabelStyle = ref(props.settings.logo_label_style || 'o')
const editBackdropLabelStyle = ref(props.settings.backdrop_label_style || 'o')
const editPosterFit = ref(props.settings.poster_fit || 'native')
const editPosterTextSize = ref(props.settings.poster_text_size ?? 100)
const editLogoTextSize = ref(props.settings.logo_text_size ?? 100)
const editBackdropTextSize = ref(props.settings.backdrop_text_size ?? 100)
const editPosterBadgeWidth = ref(props.settings.poster_badge_width ?? 100)
const editPosterBadgeHeight = ref(props.settings.poster_badge_height ?? 100)
const editLogoBadgeWidth = ref(props.settings.logo_badge_width ?? 100)
const editLogoBadgeHeight = ref(props.settings.logo_badge_height ?? 100)
const editBackdropBadgeWidth = ref(props.settings.backdrop_badge_width ?? 100)
const editBackdropBadgeHeight = ref(props.settings.backdrop_badge_height ?? 100)
const editPosterLogoSize = ref(props.settings.poster_logo_size ?? 100)
const editLogoLogoSize = ref(props.settings.logo_logo_size ?? 100)
const editBackdropLogoSize = ref(props.settings.backdrop_logo_size ?? 100)
const editLogoLayout = ref<ImageLayout>(parseLayout(props.settings.logo_layout, 'logo'))
const editBackdropLayout = ref<ImageLayout>(parseLayout(props.settings.backdrop_layout, 'backdrop'))
const editBackdropEdgeInsetX = ref(props.settings.backdrop_edge_inset_x ?? 0)
const editBackdropEdgeInsetY = ref(props.settings.backdrop_edge_inset_y ?? 0)
const editEpisodeBadgeStyle = ref(props.settings.episode_badge_style || 'v')
const editEpisodeLabelStyle = ref(props.settings.episode_label_style || 'o')
const editEpisodeTextSize = ref(props.settings.episode_text_size ?? 100)
const editEpisodeBadgeWidth = ref(props.settings.episode_badge_width ?? 100)
const editEpisodeBadgeHeight = ref(props.settings.episode_badge_height ?? 100)
const editEpisodeLogoSize = ref(props.settings.episode_logo_size ?? 100)
const editEpisodeLayout = ref<ImageLayout>(parseLayout(props.settings.episode_layout, 'episode'))
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
  editPosterBadgeStyle.value = normalizePosterBadgeStyle(s.poster_badge_style)
  editLogoBadgeStyle.value = s.logo_badge_style || 'v'
  editBackdropBadgeStyle.value = s.backdrop_badge_style || 'v'
  editPosterLabelStyle.value = s.poster_label_style || 'o'
  editLogoLabelStyle.value = s.logo_label_style || 'o'
  editBackdropLabelStyle.value = s.backdrop_label_style || 'o'
  editPosterFit.value = s.poster_fit || 'native'
  editPosterTextSize.value = s.poster_text_size ?? 100
  editLogoTextSize.value = s.logo_text_size ?? 100
  editBackdropTextSize.value = s.backdrop_text_size ?? 100
  editPosterBadgeWidth.value = s.poster_badge_width ?? 100
  editPosterBadgeHeight.value = s.poster_badge_height ?? 100
  editLogoBadgeWidth.value = s.logo_badge_width ?? 100
  editLogoBadgeHeight.value = s.logo_badge_height ?? 100
  editBackdropBadgeWidth.value = s.backdrop_badge_width ?? 100
  editBackdropBadgeHeight.value = s.backdrop_badge_height ?? 100
  editPosterLogoSize.value = s.poster_logo_size ?? 100
  editLogoLogoSize.value = s.logo_logo_size ?? 100
  editBackdropLogoSize.value = s.backdrop_logo_size ?? 100
  editBackdropEdgeInsetX.value = s.backdrop_edge_inset_x ?? 0
  editBackdropEdgeInsetY.value = s.backdrop_edge_inset_y ?? 0
  editEpisodeBadgeStyle.value = s.episode_badge_style || 'v'
  editEpisodeLabelStyle.value = s.episode_label_style || 'o'
  editEpisodeTextSize.value = s.episode_text_size ?? 100
  editEpisodeBadgeWidth.value = s.episode_badge_width ?? 100
  editEpisodeBadgeHeight.value = s.episode_badge_height ?? 100
  editEpisodeLogoSize.value = s.episode_logo_size ?? 100
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
const { active: showCheck, flash: flashSaved } = useSavedFlash()
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
    poster_fit: editPosterFit.value,
    poster_text_size: editPosterTextSize.value,
    logo_text_size: editLogoTextSize.value,
    backdrop_text_size: editBackdropTextSize.value,
    poster_badge_width: editPosterBadgeWidth.value,
    poster_badge_height: editPosterBadgeHeight.value,
    logo_badge_width: editLogoBadgeWidth.value,
    logo_badge_height: editLogoBadgeHeight.value,
    backdrop_badge_width: editBackdropBadgeWidth.value,
    backdrop_badge_height: editBackdropBadgeHeight.value,
    poster_logo_size: editPosterLogoSize.value,
    logo_logo_size: editLogoLogoSize.value,
    backdrop_logo_size: editBackdropLogoSize.value,
    logo_layout: layoutToJSON(editLogoLayout.value),
    backdrop_layout: layoutToJSON(editBackdropLayout.value),
    backdrop_edge_inset_x: editBackdropEdgeInsetX.value,
    backdrop_edge_inset_y: editBackdropEdgeInsetY.value,
    episode_badge_style: editEpisodeBadgeStyle.value,
    episode_label_style: editEpisodeLabelStyle.value,
    episode_text_size: editEpisodeTextSize.value,
      episode_badge_width: editEpisodeBadgeWidth.value,
      episode_badge_height: editEpisodeBadgeHeight.value,
    episode_logo_size: editEpisodeLogoSize.value,
    episode_layout: layoutToJSON(editEpisodeLayout.value),
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
    poster_badge_style: normalizePosterBadgeStyle(s.poster_badge_style),
    logo_badge_style: s.logo_badge_style || 'v',
    backdrop_badge_style: s.backdrop_badge_style || 'v',
    poster_label_style: s.poster_label_style || 'o',
    logo_label_style: s.logo_label_style || 'o',
    backdrop_label_style: s.backdrop_label_style || 'o',
    poster_fit: s.poster_fit || 'native',
    poster_text_size: s.poster_text_size ?? 100,
    logo_text_size: s.logo_text_size ?? 100,
    backdrop_text_size: s.backdrop_text_size ?? 100,
    poster_badge_width: s.poster_badge_width ?? 100,
    poster_badge_height: s.poster_badge_height ?? 100,
    logo_badge_width: s.logo_badge_width ?? 100,
    logo_badge_height: s.logo_badge_height ?? 100,
    backdrop_badge_width: s.backdrop_badge_width ?? 100,
    backdrop_badge_height: s.backdrop_badge_height ?? 100,
    poster_logo_size: s.poster_logo_size ?? 100,
    logo_logo_size: s.logo_logo_size ?? 100,
    backdrop_logo_size: s.backdrop_logo_size ?? 100,
    logo_layout: layoutToJSON(parseLayout(s.logo_layout, 'logo')),
    backdrop_layout: layoutToJSON(parseLayout(s.backdrop_layout, 'backdrop')),
    backdrop_edge_inset_x: s.backdrop_edge_inset_x ?? 0,
    backdrop_edge_inset_y: s.backdrop_edge_inset_y ?? 0,
    episode_badge_style: s.episode_badge_style || 'v',
    episode_label_style: s.episode_label_style || 'o',
    episode_text_size: s.episode_text_size ?? 100,
    episode_badge_width: s.episode_badge_width ?? 100,
    episode_badge_height: s.episode_badge_height ?? 100,
    episode_logo_size: s.episode_logo_size ?? 100,
    episode_layout: layoutToJSON(parseLayout(s.episode_layout, 'episode')),
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
    // values actually changed — including the Rating Colours badge gallery,
    // whose debounced watcher also bails out during `syncing`.
    if (JSON.stringify(editsSnapshot()) !== before) {
      updateAllPreviews()
      galleryParams.value = buildGalleryParams()
    }
  })
})

function revertEdits() {
  syncing = true
  const before = JSON.stringify(editsSnapshot())
  applySettings(currentSettings.value)
  nextTick(() => {
    syncing = false
    // Same rationale as the props.settings watcher: while `syncing` blocks the
    // edit watchers, the previews stay on their stale render. Refresh them when
    // reverting actually changed the edited values (discard / save failure).
    if (JSON.stringify(editsSnapshot()) !== before) {
      updateAllPreviews()
      galleryParams.value = buildGalleryParams()
    }
  })
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
      poster_fit: editPosterFit.value,
      poster_text_size: editPosterTextSize.value,
      logo_text_size: editLogoTextSize.value,
      backdrop_text_size: editBackdropTextSize.value,
      poster_badge_width: editPosterBadgeWidth.value,
      poster_badge_height: editPosterBadgeHeight.value,
      logo_badge_width: editLogoBadgeWidth.value,
      logo_badge_height: editLogoBadgeHeight.value,
      backdrop_badge_width: editBackdropBadgeWidth.value,
      backdrop_badge_height: editBackdropBadgeHeight.value,
      poster_logo_size: editPosterLogoSize.value,
      logo_logo_size: editLogoLogoSize.value,
      backdrop_logo_size: editBackdropLogoSize.value,
      logo_layout: JSON.parse(JSON.stringify(editLogoLayout.value)),
      backdrop_layout: JSON.parse(JSON.stringify(editBackdropLayout.value)),
      backdrop_edge_inset_x: coerceInset(editBackdropEdgeInsetX.value),
      backdrop_edge_inset_y: coerceInset(editBackdropEdgeInsetY.value),
        episode_badge_style: editEpisodeBadgeStyle.value,
      episode_label_style: editEpisodeLabelStyle.value,
      episode_text_size: editEpisodeTextSize.value,
    episode_badge_width: editEpisodeBadgeWidth.value,
    episode_badge_height: editEpisodeBadgeHeight.value,
      episode_logo_size: editEpisodeLogoSize.value,
      episode_layout: JSON.parse(JSON.stringify(editEpisodeLayout.value)),
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
      flashSaved()
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
  try {
    const ok = await props.resetSettings()
    if (ok) {
      await props.loadSettings()
      flashSaved()
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

async function fetchPreviewImage(
  state: PreviewState,
  fetcher: () => Promise<Response>,
) {
  state.loading = true
  state.error = false
  const generation = ++state.generation

  try {
    const res = await fetcher()
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
  fetchPreviewImage(posterPreview.value, () => props.fetchPreview('poster', {
    ratingsLimit: posterTotal.value,
    ratingsOrder: editRatingsOrder.value.join(','),
    ratingsExclude: editRatingsExclude.value.join(','),
    badgeStyle: editPosterBadgeStyle.value,
    labelStyle: editPosterLabelStyle.value,
    badgeDirection: 'd',
    textSize: editPosterTextSize.value,
    layout: layoutToJSON(editPosterLayout.value),
    badgeShape: editPosterBadgeShape.value,
    badgeAlpha: editPosterBadgeAlpha.value,
    posterFit: editPosterFit.value,
    logoSize: editPosterLogoSize.value,
    badgeWidth: editPosterBadgeWidth.value,
    badgeHeight: editPosterBadgeHeight.value,
    colors: editColors.value,
  }))
}

function updateLogoPreview() {
  fetchPreviewImage(logoPreview.value, () => props.fetchPreview('logo', {
    ratingsLimit: logoTotal.value,
    ratingsOrder: editRatingsOrder.value.join(','),
    ratingsExclude: editRatingsExclude.value.join(','),
    badgeStyle: editLogoBadgeStyle.value,
    labelStyle: editLogoLabelStyle.value,
    textSize: editLogoTextSize.value,
    layout: layoutToJSON(editLogoLayout.value),
    badgeShape: editLogoBadgeShape.value,
    badgeAlpha: editLogoBadgeAlpha.value,
    logoSize: editLogoLogoSize.value,
    badgeWidth: editLogoBadgeWidth.value,
    badgeHeight: editLogoBadgeHeight.value,
    colors: editColors.value,
  }))
}

function updateBackdropPreview() {
  fetchPreviewImage(backdropPreview.value, () => props.fetchPreview('backdrop', {
    ratingsLimit: backdropTotal.value,
    ratingsOrder: editRatingsOrder.value.join(','),
    ratingsExclude: editRatingsExclude.value.join(','),
    badgeStyle: editBackdropBadgeStyle.value,
    labelStyle: editBackdropLabelStyle.value,
    badgeDirection: 'd',
    textSize: editBackdropTextSize.value,
    layout: layoutToJSON(editBackdropLayout.value),
    badgeShape: editBackdropBadgeShape.value,
    badgeAlpha: editBackdropBadgeAlpha.value,
    edgeInsetX: coerceInset(Number(editBackdropEdgeInsetX.value)),
    edgeInsetY: coerceInset(Number(editBackdropEdgeInsetY.value)),
    logoSize: editBackdropLogoSize.value,
    badgeWidth: editBackdropBadgeWidth.value,
    badgeHeight: editBackdropBadgeHeight.value,
    colors: editColors.value,
  }))
}

function updateEpisodePreview() {
  fetchPreviewImage(episodePreview.value, () => props.fetchPreview('episode', {
    ratingsLimit: episodeTotal.value,
    ratingsOrder: editRatingsOrder.value.join(','),
    ratingsExclude: editRatingsExclude.value.join(','),
    badgeStyle: editEpisodeBadgeStyle.value,
    labelStyle: editEpisodeLabelStyle.value,
    badgeDirection: 'd',
    textSize: editEpisodeTextSize.value,
    layout: layoutToJSON(editEpisodeLayout.value),
    badgeShape: editEpisodeBadgeShape.value,
    badgeAlpha: editEpisodeBadgeAlpha.value,
    blur: editEpisodeBlur.value,
    logoSize: editEpisodeLogoSize.value,
    badgeWidth: editEpisodeBadgeWidth.value,
    badgeHeight: editEpisodeBadgeHeight.value,
    colors: editColors.value,
  }))
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
watch([editPosterLayout, editPosterBadgeStyle, editPosterLabelStyle, editPosterFit, editPosterTextSize, editPosterBadgeWidth, editPosterBadgeHeight, editPosterLogoSize, editPosterBadgeShape, editPosterBadgeAlpha], () => {
  if (syncing) return
  if (posterPreviewTimer) clearTimeout(posterPreviewTimer)
  posterPreviewTimer = setTimeout(updatePosterPreview, 500)
})

// Logo-only settings
watch([editLogoLayout, editLogoBadgeStyle, editLogoLabelStyle, editLogoTextSize, editLogoBadgeWidth, editLogoBadgeHeight, editLogoLogoSize, editLogoBadgeShape, editLogoBadgeAlpha], () => {
  if (syncing) return
  if (logoPreviewTimer) clearTimeout(logoPreviewTimer)
  logoPreviewTimer = setTimeout(updateLogoPreview, 500)
})

// Backdrop-only settings
watch([editBackdropLayout, editBackdropBadgeStyle, editBackdropLabelStyle, editBackdropTextSize, editBackdropBadgeWidth, editBackdropBadgeHeight, editBackdropLogoSize, editBackdropEdgeInsetX, editBackdropEdgeInsetY, editBackdropBadgeShape, editBackdropBadgeAlpha], () => {
  if (syncing) return
  if (backdropPreviewTimer) clearTimeout(backdropPreviewTimer)
  backdropPreviewTimer = setTimeout(updateBackdropPreview, 500)
})

// Episode-only settings
watch([editEpisodeLayout, editEpisodeBadgeStyle, editEpisodeLabelStyle, editEpisodeTextSize, editEpisodeBadgeWidth, editEpisodeBadgeHeight, editEpisodeLogoSize, editEpisodeBlur, editEpisodeBadgeShape, editEpisodeBadgeAlpha], () => {
  if (syncing) return
  if (episodePreviewTimer) clearTimeout(episodePreviewTimer)
  episodePreviewTimer = setTimeout(updateEpisodePreview, 500)
})

// Initial preview on mount
updateAllPreviews()

// The badge previews in the "Rating Colours" rows mirror the poster preview's
// render params (colors, alpha, shape, label style, order) so live, unsaved
// changes show up there too. The serialized params are debounced so dragging a
// colour picker doesn't trigger a reload per event.
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
  const colors = colorsQuery(editColors.value)
  if (colors) p.set('colors', colors)
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

// The badge API resolves source logos by their base key ("rt"/"rta"); the
// Rating Colours rows use one row per RT logo variant (rt_cf, rt_pos, …), so
// the variant key must be mapped back to the base key for the URL.
function baseSourceKey(row: { key: string }): string {
  if (row.key.startsWith('rta_')) return 'rta'
  if (row.key.startsWith('rt_')) return 'rt'
  return row.key
}

// Badge sample to show in a "Rating Colours" row. Rotten Tomatoes rows are one
// per logo variant (rt_cf, rt_pos, …), so each row shows only the sample whose
// label matches the variant named in its row label ("… · Certified Fresh" etc.)
// — e.g. the rt_cf row renders just the Certified Fresh badge. Non-RT rows have
// a single sample and are returned unchanged.
function badgeSamplesForRow(row: { key: string; label: string }): { value: string; label: string }[] {
  const base = row.key.startsWith('rta_') ? 'rta' : row.key.startsWith('rt_') ? 'rt' : row.key
  const samples = SOURCE_BADGE_SAMPLES[base] ?? []
  if (samples.length <= 1) return samples
  const variant = row.label.split(' · ').pop() ?? ''
  const match = samples.find(s => s.label.endsWith(`· ${variant}`))
  return match ? [match] : [samples[0]!]
}

// Close any open native colour picker when the user clicks outside it.
function onDocumentClick(e: MouseEvent) {
  const target = e.target as HTMLElement | null
  if (!target || target.closest('input[type="color"]')) return
  document.querySelectorAll('input[type="color"]').forEach((el) => (el as HTMLInputElement).blur())
}

onMounted(() => document.addEventListener('click', onDocumentClick))

onBeforeUnmount(() => {
  // Leaving the settings page resets the tab to the first category on return;
  // a hard refresh does NOT run this hook, so the tab still persists there.
  localStorage.removeItem(TAB_STORAGE_KEY)
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

    <Tabs :model-value="activeTab" @update:model-value="activeTab = String($event)" :unmount-on-hide="false">
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
        <p class="text-xs text-muted-foreground">Use the arrows to reorder. Higher items have priority. Use the eye to hide or show a rating source.</p>
        <RatingsOrderList
          v-model="editRatingsOrder"
          :excluded="editRatingsExclude"
          toggleable
          @toggle-exclude="(key) => toggleExclude(key, !isExcluded(key))"
        />
      </div>
    </div>

    <!-- Rating Colours (global only) -->
    <template v-if="uid === 'global'">
      <div class="rounded-md border p-4 space-y-3">
        <p class="text-sm font-semibold">Rating Colours</p>
        <p class="text-xs text-muted-foreground">Customize each rating source's badge colours. The badge background opacity is set by the background-opacity sliders above.</p>
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
        <div class="grid grid-cols-1 gap-x-4 gap-y-2">
          <div
            v-for="source in RATING_COLOR_ROWS"
            :key="source.key"
            class="flex items-start gap-2 pt-1"
          >
            <Label class="w-44 text-sm font-normal shrink-0">{{ source.label }}</Label>
            <div class="flex flex-wrap items-start gap-x-3 gap-y-2">
              <img
                v-for="sample in badgeSamplesForRow(source)"
                :key="sample.value"
                :src="badgePreviewUrl(baseSourceKey(source), sample.value)"
                :alt="`${source.label} badge (${sample.label})`"
                :title="`${source.label} badge — ${sample.label}`"
                class="h-10 w-auto"
              />
            </div>
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
          </div>
        </div>
      </div>
    </template>
        </div>
      </TabsContent>

      <TabsContent value="poster" class="mt-3">
        <KindSettingsPanel
          kind="poster"
          title="Poster"
          :preview="posterPreview"
          :uid="uid"
          :edit-layout="editPosterLayout"
          @update:edit-layout="editPosterLayout = $event"
          v-model:badge-style="editPosterBadgeStyle"
          v-model:label-style="editPosterLabelStyle"
          v-model:shape="editPosterBadgeShape"
          v-model:alpha="editPosterBadgeAlpha"
          v-model:text-size="editPosterTextSize"
          v-model:badge-width="editPosterBadgeWidth"
          v-model:badge-height="editPosterBadgeHeight"
          v-model:logo-size="editPosterLogoSize"
          v-model:fit="editPosterFit"
          v-model:textless="editTextless"
        />
      </TabsContent>

      <TabsContent value="logo" class="mt-3">
        <KindSettingsPanel
          kind="logo"
          title="Logo"
          :preview="logoPreview"
          :uid="uid"
          :edit-layout="editLogoLayout"
          @update:edit-layout="editLogoLayout = $event"
          v-model:badge-style="editLogoBadgeStyle"
          v-model:label-style="editLogoLabelStyle"
          v-model:shape="editLogoBadgeShape"
          v-model:alpha="editLogoBadgeAlpha"
          v-model:text-size="editLogoTextSize"
          v-model:badge-width="editLogoBadgeWidth"
          v-model:badge-height="editLogoBadgeHeight"
          v-model:logo-size="editLogoLogoSize"
        />
      </TabsContent>

      <TabsContent value="backdrop" class="mt-3">
        <KindSettingsPanel
          kind="backdrop"
          title="Backdrop (movie/series)"
          :preview="backdropPreview"
          :uid="uid"
          :edit-layout="editBackdropLayout"
          @update:edit-layout="editBackdropLayout = $event"
          v-model:badge-style="editBackdropBadgeStyle"
          v-model:label-style="editBackdropLabelStyle"
          v-model:shape="editBackdropBadgeShape"
          v-model:alpha="editBackdropBadgeAlpha"
          v-model:text-size="editBackdropTextSize"
          v-model:badge-width="editBackdropBadgeWidth"
          v-model:badge-height="editBackdropBadgeHeight"
          v-model:logo-size="editBackdropLogoSize"
          v-model:edge-inset-x="editBackdropEdgeInsetX"
          v-model:edge-inset-y="editBackdropEdgeInsetY"
        />
      </TabsContent>

      <TabsContent value="episode" class="mt-3">
        <KindSettingsPanel
          kind="episode"
          title="Episode"
          :preview="episodePreview"
          :uid="uid"
          :edit-layout="editEpisodeLayout"
          @update:edit-layout="editEpisodeLayout = $event"
          v-model:badge-style="editEpisodeBadgeStyle"
          v-model:label-style="editEpisodeLabelStyle"
          v-model:shape="editEpisodeBadgeShape"
          v-model:alpha="editEpisodeBadgeAlpha"
          v-model:text-size="editEpisodeTextSize"
          v-model:badge-width="editEpisodeBadgeWidth"
          v-model:badge-height="editEpisodeBadgeHeight"
          v-model:logo-size="editEpisodeLogoSize"
          v-model:blur="editEpisodeBlur"
        />
      </TabsContent>
    </Tabs>
  </div>
</template>
