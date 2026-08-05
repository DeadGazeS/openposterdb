import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RenderSettingsForm from '@/components/RenderSettingsForm.vue'
import type { RenderSettings } from '@/lib/settings'
import { shadcnStubs, SelectStub } from '@/__tests__/stubs'

vi.mock('@/lib/api', () => ({}))

const defaultSettings: RenderSettings = {
  image_source: 't',
  lang: 'en',
  textless: false,
  fanart_available: true,
  ratings_limit: 3,
  ratings_order: 'mal,imdb,lb,rt,rta,mc,tmdb,trakt',
  ratings_exclude: '',
  poster_layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 3, rows: 1, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
  logo_ratings_limit: 3,
  backdrop_ratings_limit: 3,
  poster_badge_style: 'h',
  logo_badge_style: 'h',
  backdrop_badge_style: 'v',
  poster_label_style: 'i',
  logo_label_style: 'i',
  backdrop_label_style: 'i',
  poster_badge_direction: 'd',
  poster_fit: 'native',
  poster_text_size: 100,
  logo_text_size: 100,
  backdrop_text_size: 100,
  poster_badge_size: 100,
  poster_badge_width: 100,
  poster_badge_height: 100,
  logo_badge_size: 100,
  logo_badge_width: 100,
  logo_badge_height: 100,
  backdrop_badge_size: 100,
  backdrop_badge_width: 100,
  backdrop_badge_height: 100,
  poster_logo_size: 100,
  logo_logo_size: 100,
  backdrop_logo_size: 100,
  logo_layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 5, rows: 1, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
  backdrop_layout: JSON.stringify({ top: { per_row: 5, rows: 1, start: 'r' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 0, rows: 0, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
  backdrop_badge_direction: 'v',
  backdrop_edge_inset_x: 0,
  backdrop_edge_inset_y: 0,
  episode_ratings_limit: 1,
  episode_badge_style: 'v',
  episode_label_style: 'o',
  episode_text_size: 100,
  episode_badge_size: 100,
  episode_badge_width: 100,
  episode_badge_height: 100,
  episode_logo_size: 100,
  episode_layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 1, rows: 1, start: 't' }, bottom: { per_row: 0, rows: 0, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
  episode_badge_direction: 'v',
  episode_blur: false,
  poster_badge_shape: 'r',
  logo_badge_shape: 'r',
  backdrop_badge_shape: 'r',
  episode_badge_shape: 'r',
  poster_badge_alpha: 80,
  logo_badge_alpha: 80,
  backdrop_badge_alpha: 80,
  episode_badge_alpha: 80,
}

function makeFetchPreview() {
  return vi.fn().mockResolvedValue({
    ok: true,
    blob: () => Promise.resolve(new Blob(['fake-jpeg'], { type: 'image/jpeg' })),
  })
}

function mountForm(overrides: Partial<RenderSettings> = {}, fetchPreview = makeFetchPreview()) {
  const settings = { ...defaultSettings, ...overrides }
  return mount(RenderSettingsForm, {
    props: {
      settings,
      loadSettings: vi.fn().mockResolvedValue(settings),
      saveSettings: vi.fn().mockResolvedValue(null),
      fetchPreview,
    },
    global: {
      plugins: [createPinia()],
      stubs: shadcnStubs,
    },
  })
}

describe('RenderSettingsForm', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders preview section', () => {
    const wrapper = mountForm()
    expect(wrapper.text()).toContain('Poster')
    expect(wrapper.find('img[alt="Poster preview"]').exists()).toBe(true)
  })

  it('calls fetchPreview on mount', async () => {
    const fetchPreview = makeFetchPreview()
    mountForm({}, fetchPreview)
    await flushPromises()

    expect(fetchPreview).toHaveBeenCalledWith('poster', expect.objectContaining({
      ratingsLimit: 3,
      ratingsOrder: 'mal,imdb,lb,rt,rta,mc,tmdb,trakt,mdblist,ebert',
      badgeStyle: 'lr',
      labelStyle: 'i',
      badgeDirection: 'd',
      textSize: 100,
      layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 3, rows: 1, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
      badgeShape: 'r',
      badgeAlpha: 80,
      posterFit: 'native',
      logoSize: 100,
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  it('calls fetchPreview with correct params for custom settings', async () => {
    const fetchPreview = makeFetchPreview()
    const layout = JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 5, rows: 1, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] })
    mountForm({ poster_layout: layout, ratings_order: 'imdb,rt,tmdb' }, fetchPreview)
    await flushPromises()

    expect(fetchPreview).toHaveBeenCalledWith('poster', expect.objectContaining({
      ratingsLimit: 5,
      ratingsOrder: expect.stringContaining('imdb'),
      textSize: 100,
      layout,
      badgeShape: 'r',
      badgeAlpha: 80,
      posterFit: 'native',
      logoSize: 100,
    }))
  })

  it('sets preview src from blob after fetch', async () => {
    const wrapper = mountForm()
    await flushPromises()

    const img = wrapper.find('img[alt="Poster preview"]')
    const src = img.attributes('src')
    expect(src).toBeTruthy()
    expect(src).toContain('blob:')
  })

  it('updates preview when layout changes', async () => {
    const fetchPreview = makeFetchPreview()
    const wrapper = mountForm({}, fetchPreview)
    await flushPromises()
    fetchPreview.mockClear()

    // Change the badges-per-row for the bottom side via the layout editor
    const perRowInput = wrapper.find('[data-testid="poster-bottom-per-row"]')
    await perRowInput.setValue(5)

    // Advance past preview debounce timer
    vi.advanceTimersByTime(500)
    await flushPromises()

    expect(fetchPreview).toHaveBeenCalledWith('poster', expect.objectContaining({
      ratingsLimit: 5,
      textSize: 100,
      layout: expect.stringContaining('"per_row":5'),
      logoSize: expect.any(Number),
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  it('shows loading state while preview loads', async () => {
    // Use a fetch that never resolves to keep loading state
    const fetchPreview = vi.fn().mockReturnValue(new Promise(() => {}))
    const wrapper = mountForm({}, fetchPreview)

    // previewLoading starts true on mount (updatePreview is called)
    const spinner = wrapper.find('.animate-spin')
    expect(spinner.exists()).toBe(true)

    // Image should be hidden while loading (v-show)
    const img = wrapper.find('img[alt="Poster preview"]')
    expect(img.isVisible()).toBe(false)
  })

  it('hides loading spinner and shows image after successful fetch', async () => {
    const wrapper = mountForm()
    await flushPromises()

    // After fetch resolves, trigger image load
    const img = wrapper.find('img[alt="Poster preview"]')
    await img.trigger('load')
    await flushPromises()

    // The preview lives on the form's "Images" sub-tab. jsdom reports the
    // tabpanel as display:none regardless of the active tab, so assert on the
    // meaningful bits: the blob src is set and the loading spinner is gone.
    // All four kind previews fetch on mount now; each needs its img 'load'
    // event (jsdom never fires it) to clear its spinner.
    for (const previewImg of wrapper.findAll('img[alt*="preview"]')) {
      await previewImg.trigger('load')
    }
    await flushPromises()
    expect(img.attributes('src')).toBeTruthy()
    expect(wrapper.find('.animate-spin').exists()).toBe(false)
  })

  it('shows no aspect ratio readout until the poster image loads', async () => {
    const wrapper = mountForm()
    await flushPromises()
    expect(wrapper.find('[data-testid="poster-aspect-ratio"]').exists()).toBe(false)
  })

  it('shows simplified aspect ratio and dimensions after the poster image loads', async () => {
    const wrapper = mountForm()
    await flushPromises()

    const img = wrapper.find('img[alt="Poster preview"]')
    Object.defineProperty(img.element, 'naturalWidth', { value: 500, configurable: true })
    Object.defineProperty(img.element, 'naturalHeight', { value: 750, configurable: true })
    await img.trigger('load')
    await flushPromises()

    const readout = wrapper.find('[data-testid="poster-aspect-ratio"]')
    expect(readout.exists()).toBe(true)
    expect(readout.text()).toBe('2:3 · 500×750')
  })

  it('shows error message when preview fetch fails', async () => {
    const fetchPreview = vi.fn().mockResolvedValue({ ok: false })
    const wrapper = mountForm({}, fetchPreview)
    await flushPromises()

    expect(wrapper.text()).toContain('Failed')
  })

  it('shows error message when preview fetch throws', async () => {
    const fetchPreview = vi.fn().mockRejectedValue(new Error('Network error'))
    const wrapper = mountForm({}, fetchPreview)
    await flushPromises()

    expect(wrapper.text()).toContain('Failed')
  })

  it('renders poster layout editor', () => {
    const wrapper = mountForm()
    expect(wrapper.find('[data-testid="poster-layout-editor"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="poster-bottom-per-row"]').exists()).toBe(true)
  })

  it('calls fetchPreview with poster layout', async () => {
    const fetchPreview = makeFetchPreview()
    mountForm({}, fetchPreview)
    await flushPromises()

    expect(fetchPreview).toHaveBeenCalledWith('poster', expect.objectContaining({
      ratingsLimit: 3,
      textSize: 100,
      layout: expect.stringContaining('"bottom"'),
      logoSize: expect.any(Number),
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  it('hides fanart checkbox when fanart_available is false', () => {
    const wrapper = mountForm({ fanart_available: false })
    expect(wrapper.find('[data-testid="fanart-checkbox"]').exists()).toBe(false)
  })

  it('shows fanart checkbox when fanart_available is true', () => {
    const wrapper = mountForm({ fanart_available: true })
    expect(wrapper.find('[data-testid="fanart-checkbox"]').exists()).toBe(true)
  })

  it('checks fanart checkbox when source is fanart', () => {
    const wrapper = mountForm({ image_source: 'f' })
    const checkbox = wrapper.find('[data-testid="fanart-checkbox"]')
    expect((checkbox.element as HTMLInputElement).checked).toBe(true)
  })

  it('language and textless are always enabled regardless of fanart checkbox', () => {
    const wrapper = mountForm({ image_source: 't' })
    expect((wrapper.find('[data-testid="textless-checkbox"]').element as HTMLInputElement).disabled).toBe(false)
    expect((wrapper.find('[data-testid="lang-select"]').element as HTMLInputElement).disabled).toBe(false)
  })

  it('defaults language to en when lang is empty', async () => {
    const saveSettings = vi.fn().mockResolvedValue(null)
    const settings = { ...defaultSettings, image_source: 'f', lang: '' }
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings,
        fetchPreview: makeFetchPreview(),
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })

    // Toggle textless, then save via the Save button
    await wrapper.find('[data-testid="textless-checkbox"]').setValue(true)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(saveSettings).toHaveBeenCalledWith(
      expect.objectContaining({ lang: 'en' }),
    )
  })

  // --- Exclude ratings (via the eye toggle in the rating order list) ---

  it('renders an eye toggle for every rating source', () => {
    const wrapper = mountForm()
    for (const key of ['imdb', 'tmdb', 'rt', 'rta', 'mc', 'trakt', 'lb', 'mal', 'mdblist', 'ebert']) {
      expect(wrapper.find(`[data-testid="exclude-${key}-eye"]`).exists()).toBe(true)
    }
  })

  it('initializes eye toggles from ratings_exclude', () => {
    const wrapper = mountForm({ ratings_exclude: 'rt' })
    expect(wrapper.find('[data-testid="exclude-rt-eye"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="exclude-imdb-eye"]').attributes('aria-pressed')).toBe('false')
  })

  it('toggling an eye saves ratings_exclude', async () => {
    const saveSettings = vi.fn().mockResolvedValue(null)
    const settings = { ...defaultSettings, ratings_exclude: '' }
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings,
        fetchPreview: makeFetchPreview(),
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })

    await wrapper.find('[data-testid="exclude-rt-eye"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(saveSettings).toHaveBeenCalledWith(
      expect.objectContaining({ ratings_exclude: 'rt' }),
    )
  })

  it('always passes auto badge direction to fetchPreview (direction no longer configurable)', async () => {
    const fetchPreview = makeFetchPreview()
    mountForm({ poster_badge_direction: 'v' }, fetchPreview)
    await flushPromises()

    expect(fetchPreview).toHaveBeenCalledWith('poster', expect.objectContaining({
      ratingsLimit: 3,
      badgeStyle: 'lr',
      labelStyle: 'i',
      badgeDirection: 'd',
      textSize: 100,
      layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 3, rows: 1, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
      badgeShape: 'r',
      badgeAlpha: 80,
      posterFit: 'native',
      logoSize: 100,
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  it('normalizes the auto/legacy poster badge style to a valid select value', async () => {
    const fetchPreview = makeFetchPreview()
    const wrapper = mountForm({ poster_badge_style: 'd' }, fetchPreview)
    await flushPromises()

    // 'd' (Auto) isn't among the four select options (no Auto option is offered),
    // so it must load as 'lr' (horizontal) to keep the select from rendering blank
    // on a fresh install. Legacy 'h' normalizes the same way. The native <select>
    // stub reads its value from the Select stub's modelValue prop.
    const posterSelect = wrapper.findAllComponents(SelectStub).find(c => c.find('[data-testid="poster-badge-style-select"]').exists())
    expect(posterSelect?.props('modelValue')).toBe('lr')

    expect(fetchPreview).toHaveBeenCalledWith('poster', expect.objectContaining({
      ratingsLimit: 3,
      badgeStyle: 'lr',
      badgeAlpha: expect.any(Number),
      logoSize: expect.any(Number),
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  // --- Episode preview ---

  it('renders episode section when fetchEpisodePreview is provided', () => {
    const fetchEpisodePreview = makeFetchPreview()
    const settings = { ...defaultSettings }
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings: vi.fn().mockResolvedValue(null),
        fetchPreview: makeFetchPreview(),
        fetchEpisodePreview,
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
    expect(wrapper.text()).toContain('Episode')
    expect(wrapper.find('img[alt="Episode preview"]').exists()).toBe(true)
  })

  it('renders the episode section by default', () => {
    const wrapper = mountForm()
    expect(wrapper.find('img[alt="Episode preview"]').exists()).toBe(true)
  })

  it('calls fetchPreview for the episode on mount with episode settings', async () => {
    const fetchPreview = makeFetchPreview()
    const settings = { ...defaultSettings }
    mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings: vi.fn().mockResolvedValue(null),
        fetchPreview,
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
    await flushPromises()

    expect(fetchPreview).toHaveBeenCalledWith('episode', expect.objectContaining({
      ratingsLimit: 1, // episode_ratings_limit
      badgeStyle: 'v', // episode_badge_style
      labelStyle: 'o', // episode_label_style
      textSize: 100, // episode_text_size
      badgeDirection: 'd', // no longer configurable
      blur: false, // episode_blur
      layout: expect.stringContaining('"right"'), // episode_layout
      badgeShape: 'r', // episode_badge_shape
      badgeAlpha: 80, // episode_badge_alpha
      logoSize: 100, // episode_logo_size
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  it('renders episode layout editor and blur controls', () => {
    const fetchEpisodePreview = makeFetchPreview()
    const settings = { ...defaultSettings }
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings: vi.fn().mockResolvedValue(null),
        fetchPreview: makeFetchPreview(),
        fetchEpisodePreview,
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
    expect(wrapper.find('[data-testid="episode-layout-editor"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="episode-badge-style-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="episode-blur-checkbox"]').exists()).toBe(true)
  })

  // --- Backdrop edge inset ---

  function mountWithBackdrop(
    overrides: Partial<RenderSettings> = {},
    fetchPreview = makeFetchPreview(),
    saveSettings = vi.fn().mockResolvedValue(null),
  ) {
    const settings = { ...defaultSettings, ...overrides }
    return mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings,
        fetchPreview,
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
  }

  it('shows both edge-inset inputs', () => {
    const wrapper = mountWithBackdrop({})
    expect(wrapper.find('[data-testid="backdrop-edge-inset-x"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="backdrop-edge-inset-y"]').exists()).toBe(true)
  })

  it('passes edge insets to fetchBackdropPreview', async () => {
    const fetchBackdropPreview = makeFetchPreview()
    mountWithBackdrop({ backdrop_edge_inset_x: 12, backdrop_edge_inset_y: 7 }, fetchBackdropPreview)
    await flushPromises()

    expect(fetchBackdropPreview).toHaveBeenCalledWith('backdrop', expect.objectContaining({
      ratingsLimit: 5, // backdrop layout total (top 5x1)
      badgeStyle: 'v', // backdrop_badge_style
      labelStyle: 'i', // backdrop_label_style
      textSize: 100, // backdrop_text_size
      badgeDirection: 'd', // no longer configurable
      badgeShape: 'r', // backdrop_badge_shape
      badgeAlpha: 80, // backdrop_badge_alpha
      edgeInsetX: 12, // backdrop_edge_inset_x
      edgeInsetY: 7, // backdrop_edge_inset_y
      layout: expect.stringContaining('"top"'), // backdrop_layout
      logoSize: 100, // backdrop_logo_size
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  it('coerces a cleared edge-inset input to 0 in the preview fetch', async () => {
    const fetchBackdropPreview = makeFetchPreview()
    const wrapper = mountWithBackdrop({ backdrop_edge_inset_x: 10, backdrop_edge_inset_y: 10 }, fetchBackdropPreview)
    await flushPromises()
    fetchBackdropPreview.mockClear()

    // Clearing a v-model.number input emits '' (not 0). The preview fetch must
    // send the same clamped integer the save path sends via coerceInset.
    await wrapper.find('[data-testid="backdrop-edge-inset-x"]').setValue('')
    vi.advanceTimersByTime(500)
    await flushPromises()

    expect(fetchBackdropPreview).toHaveBeenCalledWith('backdrop', expect.objectContaining({
      ratingsLimit: 5, // backdrop layout total (top 5x1)
      badgeStyle: 'v', // backdrop_badge_style
      labelStyle: 'i', // backdrop_label_style
      textSize: 100, // backdrop_text_size
      badgeDirection: 'd', // no longer configurable
      badgeShape: 'r', // backdrop_badge_shape
      badgeAlpha: 80, // backdrop_badge_alpha
      edgeInsetX: 0, // edge_inset_x coerced from '' -> 0
      edgeInsetY: 10, // edge_inset_y unchanged
      layout: expect.stringContaining('"top"'), // backdrop_layout
      logoSize: 100, // backdrop_logo_size
      badgeWidth: 100,
      badgeHeight: 100,
    }))
  })

  it('editing a backdrop edge inset auto-saves the new value', async () => {
    const saveSettings = vi.fn().mockResolvedValue(null)
    const wrapper = mountWithBackdrop({}, makeFetchPreview(), saveSettings)

    await wrapper.find('[data-testid="backdrop-edge-inset-y"]').setValue(15)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(saveSettings).toHaveBeenCalledWith(
      expect.objectContaining({ backdrop_edge_inset_y: 15 }),
    )
  })

  it('is not dirty on fresh load (Discard hidden)', async () => {
    const settings = { ...defaultSettings, colors: { imdb: { accent: '#b4910f', value: '#000000c8', border: '', text: '#ffffff' } } }
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings: vi.fn().mockResolvedValue(null),
        fetchPreview: makeFetchPreview(),
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="discard-settings-button"]').exists()).toBe(false)
  })

  it('hides Discard after saving', async () => {
    const saveSettings = vi.fn().mockResolvedValue(null)
    const settings = { ...defaultSettings }
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings,
        fetchPreview: makeFetchPreview(),
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
    await flushPromises()

    await wrapper.find('[data-testid="textless-checkbox"]').setValue(true)
    await flushPromises()
    expect(wrapper.find('[data-testid="discard-settings-button"]').exists()).toBe(true)

    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="discard-settings-button"]').exists()).toBe(false)
  })

  // --- Badge width / height (per-axis badge scaling) ---

  it('renders badge width and height sliders (no single badge-size slider) for every kind', () => {
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings: { ...defaultSettings },
        loadSettings: vi.fn().mockResolvedValue(defaultSettings),
        saveSettings: vi.fn().mockResolvedValue(null),
        fetchPreview: makeFetchPreview(),
        fetchLogoPreview: makeFetchPreview(),
        fetchBackdropPreview: makeFetchPreview(),
        fetchEpisodePreview: makeFetchPreview(),
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
    for (const kind of ['poster', 'logo', 'backdrop', 'episode']) {
      expect(wrapper.find(`[data-testid="${kind}-badge-width-slider"]`).exists()).toBe(true)
      expect(wrapper.find(`[data-testid="${kind}-badge-height-slider"]`).exists()).toBe(true)
      expect(wrapper.find(`[data-testid="${kind}-badge-size-slider"]`).exists()).toBe(false)
    }
  })

  it('saves badge width and height and omits the legacy badge size', async () => {
    const saveSettings = vi.fn().mockResolvedValue(null)
    const settings = { ...defaultSettings }
    const wrapper = mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings,
        fetchPreview: makeFetchPreview(),
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })

    await wrapper.find('[data-testid="poster-badge-width-slider"]').setValue(130)
    await wrapper.find('[data-testid="poster-badge-height-slider"]').setValue(85)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(saveSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        poster_badge_width: 130,
        poster_badge_height: 85,
      }),
    )
    const payload = saveSettings.mock.calls[0]?.[0] as Record<string, unknown>
    expect(payload.poster_badge_size).toBeUndefined()
    expect(payload.episode_badge_size).toBeUndefined()
  })

  it('passes badge width and height to the preview fetch', async () => {
    const fetchPreview = makeFetchPreview()
    const settings = { ...defaultSettings, poster_badge_width: 120, poster_badge_height: 90 }
    mount(RenderSettingsForm, {
      props: {
        settings,
        loadSettings: vi.fn().mockResolvedValue(settings),
        saveSettings: vi.fn().mockResolvedValue(null),
        fetchPreview,
      },
      global: {
        plugins: [createPinia()],
        stubs: shadcnStubs,
      },
    })
    await flushPromises()

    expect(fetchPreview).toHaveBeenCalledWith('poster', expect.objectContaining({
      ratingsLimit: 3,
      logoSize: expect.any(Number),
      badgeWidth: 120,
      badgeHeight: 90,
    }))
  })
})
