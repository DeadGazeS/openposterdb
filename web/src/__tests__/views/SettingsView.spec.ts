import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import SettingsView from '@/views/SettingsView.vue'
import { shadcnStubs } from '@/__tests__/stubs'

// The active settings section is derived from the route, so give the view a
// fake route. Each test can switch the section by mutating mockRoute before
// mounting (matching how the router names the settings routes).
const mockRoute = vi.hoisted(() => ({
  name: 'settings-general',
  path: '/admin/settings/general',
  query: {} as Record<string, string | undefined>,
}))

vi.mock('vue-router', () => ({
  useRoute: () => mockRoute,
  useRouter: () => ({ replace: vi.fn() }),
}))

const mockAdminApi = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  previewPoster: vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob()) }),
  previewLogo: vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob()) }),
  previewBackdrop: vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob()) }),
}))

const mockKeysApi = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) }),
  create: vi.fn(),
  delete: vi.fn(),
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  deleteSettings: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    adminApi: mockAdminApi,
    keysApi: mockKeysApi,
  }
})

const defaultSettings = {
  image_source: 't',
  lang: 'en',
  textless: false,
  fanart_available: true,
  ratings_limit: 3,
  ratings_order: 'mal,imdb,lb,rt,rta,mc,tmdb,trakt',
  ratings_exclude: '',
  free_api_key_enabled: false,
  poster_layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 3, rows: 1, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
  logo_ratings_limit: 3,
  backdrop_ratings_limit: 3,
  poster_badge_style: 'h',
  logo_badge_style: 'h',
  backdrop_badge_style: 'v',
  poster_label_style: 't',
  logo_label_style: 't',
  backdrop_label_style: 't',
  poster_badge_direction: 'd',
  
  backdrop_layout: JSON.stringify({ top: { per_row: 5, rows: 1, start: 'r' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 0, rows: 0, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
  backdrop_badge_direction: 'v',
  backdrop_edge_inset_x: 0,
  backdrop_edge_inset_y: 0,
  episode_ratings_limit: 1,
  episode_badge_style: 'v',
  episode_label_style: 'o',
  episode_text_size: 100,
  episode_layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 1, rows: 1, start: 't' }, bottom: { per_row: 0, rows: 0, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
  episode_badge_direction: 'v',
  episode_blur: false,
}

function mountView() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return mount(SettingsView, {
    global: {
      plugins: [createPinia(), [VueQueryPlugin, { queryClient }]],
      stubs: {
        ...shadcnStubs,
        Input: {
          template:
            '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
          props: ['modelValue', 'type', 'placeholder'],
        },
        RefreshButton: {
          template: '<button @click="$emit(\'refresh\')">Refresh</button>',
          props: ['fetching'],
        },
        ClearCacheButton: {
          template: '<button @click="$emit(\'cleared\', \'Cache cleared — removed 7 cached images.\')">Clear cache</button>',
          emits: ['cleared'],
        },
      },
    },
  })
}

describe('SettingsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockRoute.name = 'settings-api'
    mockRoute.path = '/admin/settings/api'
    mockRoute.query = { tab: 'free-api-key' }
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(defaultSettings),
    })
  })

  it('renders the API box with the API section by default', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('Free API Key')
    expect(wrapper.text()).toContain('Source API Keys')
    expect(wrapper.find('[data-testid="save-settings-button"]').exists()).toBe(true)
  })

  it('renders the Global Image Settings box on the image route', async () => {
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Global Image Settings')
    expect(wrapper.find('[data-testid="save-settings-button"]').exists()).toBe(true)
  })

  it('loads and displays current settings with fanart enabled', async () => {
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...defaultSettings,
          image_source: 'f',
          lang: 'de',
          textless: true,
        }),
    })

    const wrapper = mountView()
    await flushPromises()

    const fanartCheckbox = wrapper.find('[data-testid="fanart-checkbox"]')
    expect((fanartCheckbox.element as HTMLInputElement).checked).toBe(true)
    const textlessCheckbox = wrapper.find('[data-testid="textless-checkbox"]')
    expect((textlessCheckbox.element as HTMLInputElement).checked).toBe(true)
  })

  it('shows fanart checkbox when fanart is available', async () => {
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="fanart-checkbox"]').exists()).toBe(true)
  })

  it('hides fanart options when not available', async () => {
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...defaultSettings,
          fanart_available: false,
        }),
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="fanart-checkbox"]').exists()).toBe(false)
  })

  it('auto-saves when fanart checkbox is toggled', async () => {
    vi.useFakeTimers()
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="fanart-checkbox"]').setValue(true)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        image_source: 'f',
        lang: 'en',
        textless: false,
      }),
    )
    vi.useRealTimers()
  })

  it('shows saved indicator after auto-save', async () => {
    vi.useFakeTimers()
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="fanart-checkbox"]').setValue(true)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('.text-green-500').exists()).toBe(true)
    vi.useRealTimers()
  })

  it('shows error message on auto-save failure', async () => {
    vi.useFakeTimers()
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.updateSettings.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: 'Invalid language' }),
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="fanart-checkbox"]').setValue(true)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Invalid language')
    vi.useRealTimers()
  })

  it('includes ratings fields in auto-save payload', async () => {
    vi.useFakeTimers()
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...defaultSettings,
          ratings_limit: 3,
          ratings_order: 'mal,imdb,trakt,rt,rta,mc,tmdb,lb',
        }),
    })
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    // Toggle fanart to trigger auto-save
    await wrapper.find('[data-testid="fanart-checkbox"]').setValue(true)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        ratings_order: expect.stringContaining('mal'),
      }),
    )
    vi.useRealTimers()
  })

  it('shows generic error on network failure', async () => {
    vi.useFakeTimers()
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.updateSettings.mockRejectedValue(new Error('Network error'))

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="fanart-checkbox"]').setValue(true)
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Failed to save')
    vi.useRealTimers()
  })

  // --- Free API Key toggle ---

  it('renders Free API Key section', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Free API Key')
    expect(wrapper.text()).toContain('t0-free-rpdb')
  })

  it('shows toggle as disabled by default', async () => {
    const wrapper = mountView()
    await flushPromises()

    const toggle = wrapper.find('button[role="switch"]')
    expect(toggle.exists()).toBe(true)
    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(wrapper.text()).toContain('Disabled')
  })

  it('shows toggle as enabled when settings say so', async () => {
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...defaultSettings,
          free_api_key_enabled: true,
        }),
    })

    const wrapper = mountView()
    await flushPromises()

    const toggle = wrapper.find('button[role="switch"]')
    expect(toggle.attributes('aria-checked')).toBe('true')
    expect(wrapper.text()).toContain('Enabled')
  })

  it('toggles free API key and calls updateSettings', async () => {
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    const toggle = wrapper.find('button[role="switch"]')
    await toggle.trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        free_api_key_enabled: true,
      }),
    )
  })

  it('save button includes free_api_key_enabled', async () => {
    vi.useFakeTimers()
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...defaultSettings,
          free_api_key_enabled: true,
        }),
    })
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        free_api_key_enabled: true,
      }),
    )
    vi.useRealTimers()
  })

  it('save button payload includes episode settings', async () => {
    vi.useFakeTimers()
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...defaultSettings,
          episode_ratings_limit: 2,
          episode_badge_style: 'h',
          episode_label_style: 'i',
          episode_text_size: 120,
          episode_layout: JSON.stringify({ top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 1, rows: 1, start: 't' }, bottom: { per_row: 0, rows: 0, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
          episode_badge_direction: 'h',
          episode_blur: true,
        }),
    })
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        episode_badge_style: 'h',
        episode_label_style: 'i',
        episode_text_size: 120,
        episode_layout: expect.objectContaining({ right: expect.objectContaining({ per_row: 1 }) }),
        episode_blur: true,
      }),
    )
    vi.useRealTimers()
  })

  it('toggleFreeApiKey save sends only the toggle payload', async () => {
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        free_api_key_enabled: false,
      }),
    )
    // The minimal API-page payload must NOT contain settings fields — the
    // backend keeps omitted fields, so toggling the free key can't clobber
    // unrelated settings (episode/backdrop etc.).
    const call = mockAdminApi.updateSettings.mock.calls[0]![0] as Record<string, unknown>
    expect('episode_badge_style' in call).toBe(false)
    expect('backdrop_edge_inset_x' in call).toBe(false)
  })

  it('toggleFreeApiKey payload preserves backdrop position and edge insets', async () => {
    // Regression guard: the free-key toggle save must stay minimal — the
    // backend keeps omitted fields, so flipping the switch can't silently
    // reset backdrop settings to their serde defaults.
    mockAdminApi.getSettings.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...defaultSettings,
          backdrop_layout: JSON.stringify({ top: { per_row: 5, rows: 1, start: 'r' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 0, rows: 0, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }),
          backdrop_badge_direction: 'h',
          backdrop_edge_inset_x: 12,
          backdrop_edge_inset_y: 7,
        }),
    })
    mockAdminApi.updateSettings.mockResolvedValue({ ok: true })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button[role="switch"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="save-settings-button"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        free_api_key_enabled: true,
      }),
    )
    const call = mockAdminApi.updateSettings.mock.calls[0]![0] as Record<string, unknown>
    expect('backdrop_layout' in call).toBe(false)
    expect('backdrop_edge_inset_x' in call).toBe(false)
  })

  it('Refresh pulls the last saved config and overrides unsaved edits', async () => {
    mockRoute.name = 'settings-image'
    mockRoute.path = '/admin/settings/image'
    // The saved config has a non-default badge width; the mock must return a
    // FRESH object per call (like the real res.json()) so the refetch delivers
    // a new reference and the form's settings watcher re-applies it.
    mockAdminApi.getSettings.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ ...defaultSettings, poster_badge_width: 120 }),
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    // Activate the Poster tab so the badge width slider is rendered.
    await wrapper.find('[data-testid="form-tab-poster"]').trigger('click')
    await flushPromises()

    const slider = wrapper.find('[data-testid="poster-badge-width-slider"]')
    expect((slider.element as HTMLInputElement).value).toBe('120')

    // Make an unsaved edit: 120 -> 150; the Discard button appears (dirty).
    await slider.setValue('150')
    await flushPromises()
    expect((slider.element as HTMLInputElement).value).toBe('150')
    expect(wrapper.find('[data-testid="discard-settings-button"]').exists()).toBe(true)

    // Refresh must pull the saved config (120) and override the edit.
    const refreshButton = wrapper.findAll('button').find((b) => b.text() === 'Refresh')
    expect(refreshButton).toBeDefined()
    await refreshButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect((slider.element as HTMLInputElement).value).toBe('120')
    expect(wrapper.find('[data-testid="discard-settings-button"]').exists()).toBe(false)
  })
})
