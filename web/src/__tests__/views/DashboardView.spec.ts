import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import DashboardView from '@/views/DashboardView.vue'

const mockAdminApi = vi.hoisted(() => ({
  getStats: vi.fn(),
  getPrefs: vi.fn(),
  updatePrefs: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  adminApi: mockAdminApi,
}))

const sampleStats = {
  total_images: 42,
  total_api_keys: 3,
  mem_cache_entries: 100,
  id_cache_entries: 50,
  ratings_cache_entries: 25,
  image_mem_cache_mb: 128,
  image_disk_cache_mb: 512.5,
  image_disk_cache_files: 4627,
  uptime_seconds: 3 * 86400 + 4 * 3600 + 5 * 60,
  database_mb: 3.25,
  image_requests: 1200,
  mem_cache_hit_rate: 87.456,
}

function mountView() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return mount(DashboardView, {
    global: {
      plugins: [createPinia(), [VueQueryPlugin, { queryClient }]],
      stubs: {
        Button: {
          template: '<button @click="$emit(\'click\')" :disabled="disabled"><slot /></button>',
          props: ['disabled', 'variant', 'size'],
          emits: ['click'],
        },
        Card: { template: '<div><slot /></div>' },
        CardHeader: { template: '<div><slot /></div>' },
        CardTitle: { template: '<div><slot /></div>' },
        CardContent: { template: '<div><slot /></div>' },
        Skeleton: { template: '<div data-testid="skeleton" />' },
        RefreshCw: { template: '<span />' },
        ClearCacheButton: {
          template: '<button @click="$emit(\'cleared\', \'Cache and images cleared — removed 42 cached images.\')">Clear Cache and Images</button>',
          emits: ['cleared'],
        },
      },
    },
  })
}

describe('DashboardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockAdminApi.getPrefs.mockResolvedValue({ ok: true, json: () => Promise.resolve({}) })
  })

  it('shows skeletons while loading', () => {
    mockAdminApi.getStats.mockReturnValue(new Promise(() => {})) // never resolves
    const wrapper = mountView()
    expect(wrapper.findAll('[data-testid="skeleton"]').length).toBeGreaterThan(0)
  })

  it('renders stats from API response', async () => {
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleStats),
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('100')
    expect(wrapper.text()).toContain('50')
    expect(wrapper.text()).toContain('25')
    expect(wrapper.text()).toContain('128')
  })

  it('has a refresh button', async () => {
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleStats),
    })

    const wrapper = mountView()
    await flushPromises()

    const refreshButton = wrapper.findAll('button').find((b) => b.text().includes('Refresh'))
    expect(refreshButton).toBeDefined()
  })

  it('refresh button triggers refetch', async () => {
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleStats),
    })

    const wrapper = mountView()
    await flushPromises()

    mockAdminApi.getStats.mockClear()
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ ...sampleStats, total_images: 99 }),
    })

    const refreshButton = wrapper.findAll('button').find((b) => b.text().includes('Refresh'))
    await refreshButton!.trigger('click')
    await flushPromises()

    expect(mockAdminApi.getStats).toHaveBeenCalled()
    expect(wrapper.text()).toContain('99')
  })

  it('shows a message and refetches stats when the cache is cleared', async () => {
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleStats),
    })

    const wrapper = mountView()
    await flushPromises()

    mockAdminApi.getStats.mockClear()

    // ClearCacheButton owns the confirm flow; here it emits `cleared` on click.
    const clearButton = wrapper.findAll('button').find((b) => b.text().includes('Clear Cache'))
    expect(clearButton).toBeDefined()
    await clearButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Cache and images cleared')
    expect(mockAdminApi.getStats).toHaveBeenCalled() // refetched after clear
  })

  it('rounds image_mem_cache_mb via formatStatValue (two decimals)', async () => {
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ ...sampleStats, image_mem_cache_mb: 123.4567890123 }),
    })

    const wrapper = mountView()
    await flushPromises()

    // Two-decimal rounding of image_mem_cache_mb; everything else stays as-is.
    expect(wrapper.text()).toContain('123.46')
    // The raw 12-digit float must not leak through (would not be substring of
    // the rounded display).
    expect(wrapper.text()).not.toContain('123.4567890123')
  })

  it('labels memory and disk image caches separately', async () => {
    mockAdminApi.getStats.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleStats) })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('Image Memory Cache (MB)')
    expect(wrapper.text()).toContain('Image Disk Cache (MB)')
    expect(wrapper.text()).toContain('512.50')
  })

  // Twelve tiles → four full rows of three on wide screens.
  it('shows twelve tiles including the disk cache file count', async () => {
    mockAdminApi.getStats.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleStats) })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findAll('p.text-2xl')).toHaveLength(12)
    expect(wrapper.text()).toContain('Disk Cache Files')
    expect(wrapper.text()).toContain('4627')
  })

  it('shows a dash when there is no disk cache (EXTERNAL_CACHE_ONLY)', async () => {
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ ...sampleStats, image_disk_cache_mb: null, image_disk_cache_files: null }),
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('—')
  })

  // Each tile explains its number on hover / keyboard focus.
  it('explains a tile when it is focused', async () => {
    // jsdom lacks ResizeObserver, which the tooltip's positioning uses.
    vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
    mockAdminApi.getStats.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleStats) })
    const wrapper = mountView()
    await flushPromises()
    const tile = wrapper.find('[data-testid="stat-image_disk_cache_files"]')
    expect(tile.attributes('tabindex')).toBe('0')
    await tile.trigger('focus')
    await flushPromises()
    const hint = document.body.querySelector('[data-testid="stat-hint-image_disk_cache_files"]')
    expect(hint?.textContent).toContain('out of sync')
    wrapper.unmount()
    vi.unstubAllGlobals()
  })

  it('formats uptime, database size and hit rate', async () => {
    mockAdminApi.getStats.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleStats) })
    const wrapper = mountView()
    await flushPromises()
    const value = (key: string) => wrapper.find(`[data-testid="stat-${key}"] p.text-2xl`).text()
    expect(value('uptime_seconds')).toBe('3 d 4 h')
    expect(value('database_mb')).toBe('3.25')
    expect(value('image_requests')).toBe('1200')
    expect(value('mem_cache_hit_rate')).toBe('87.5 %')
  })

  it('shows short uptimes and no hit rate before the first request', async () => {
    mockAdminApi.getStats.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ ...sampleStats, uptime_seconds: 30, image_requests: 0, mem_cache_hit_rate: null }),
    })
    const wrapper = mountView()
    await flushPromises()
    const value = (key: string) => wrapper.find(`[data-testid="stat-${key}"] p.text-2xl`).text()
    expect(value('uptime_seconds')).toBe('< 1 min')
    expect(value('mem_cache_hit_rate')).toBe('—')
  })

  // --- Layout: A–Z default, saved per user, editable ---

  const tileOrder = (w: ReturnType<typeof mountView>) =>
    w.findAll('[data-testid^="stat-"]').filter((t) => !t.attributes('data-testid')!.startsWith('stat-hint')).map((t) => t.attributes('data-testid')!.slice(5))

  function withPrefs(prefs: Record<string, string>) {
    mockAdminApi.getStats.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleStats) })
    mockAdminApi.getPrefs.mockResolvedValue({ ok: true, json: () => Promise.resolve(prefs) })
    mockAdminApi.updatePrefs.mockResolvedValue({ ok: true, json: () => Promise.resolve({}) })
  }

  it('sorts the tiles A–Z by default', async () => {
    withPrefs({})
    const wrapper = mountView()
    await flushPromises()
    const order = tileOrder(wrapper)
    expect(order[0]).toBe('total_api_keys') // "API Keys"
    expect(order[order.length - 1]).toBe('uptime_seconds') // "Uptime"
    expect(wrapper.find('[data-testid="stats-grid"]').classes()).toContain('lg:grid-cols-3')
  })

  it('applies the saved layout: order, hidden tiles and columns', async () => {
    withPrefs({
      dashboard_layout: JSON.stringify({ order: ['uptime_seconds', 'total_images'], hidden: ['total_api_keys'], columns: 4 }),
    })
    const wrapper = mountView()
    await flushPromises()
    const order = tileOrder(wrapper)
    expect(order.slice(0, 2)).toEqual(['uptime_seconds', 'total_images'])
    expect(order).not.toContain('total_api_keys')
    expect(order).toHaveLength(11)
    expect(wrapper.find('[data-testid="stats-grid"]').classes()).toContain('lg:grid-cols-4')
  })

  it('edits and saves the layout', async () => {
    withPrefs({})
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="layout-customize"]').trigger('click')
    await wrapper.find('[data-testid="layout-eye-database_mb"]').trigger('click')
    // Hidden tiles stay visible (dimmed) while editing.
    expect(tileOrder(wrapper)).toContain('database_mb')
    await wrapper.find('[data-testid="layout-right-total_api_keys"]').trigger('click')
    await wrapper.find('[data-testid="layout-columns-2"]').trigger('click')
    await wrapper.find('[data-testid="layout-done"]').trigger('click')
    await flushPromises()

    expect(mockAdminApi.updatePrefs).toHaveBeenCalledTimes(1)
    const saved = JSON.parse(mockAdminApi.updatePrefs.mock.calls[0]![0].dashboard_layout)
    expect(saved.hidden).toEqual(['database_mb'])
    expect(saved.columns).toBe(2)
    expect(saved.order.slice(0, 2)).toEqual(['database_mb', 'total_api_keys'])
    expect(tileOrder(wrapper)).not.toContain('database_mb')
    expect(wrapper.find('[data-testid="stats-grid"]').classes()).toContain('lg:grid-cols-2')
  })

  it('cancel discards the changes; Reset and Sort A–Z restore the default order', async () => {
    withPrefs({ dashboard_layout: JSON.stringify({ order: ['uptime_seconds'], hidden: [], columns: 3 }) })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="layout-customize"]').trigger('click')
    await wrapper.find('[data-testid="layout-sort"]').trigger('click')
    expect(tileOrder(wrapper)[0]).toBe('total_api_keys')
    await wrapper.find('[data-testid="layout-cancel"]').trigger('click')
    expect(tileOrder(wrapper)[0]).toBe('uptime_seconds')
    expect(mockAdminApi.updatePrefs).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="layout-customize"]').trigger('click')
    await wrapper.find('[data-testid="layout-eye-total_images"]').trigger('click')
    await wrapper.find('[data-testid="layout-reset"]').trigger('click')
    expect(tileOrder(wrapper)[0]).toBe('total_api_keys')
    expect(wrapper.find('[data-testid="layout-eye-total_images"]').attributes('aria-pressed')).toBe('false')
  })

  it('does not let a slow prefs response overwrite edits already started', async () => {
    mockAdminApi.getStats.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleStats) })
    let resolvePrefs!: (v: unknown) => void
    mockAdminApi.getPrefs.mockReturnValue(new Promise((r) => (resolvePrefs = r)))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="layout-customize"]').trigger('click')
    await wrapper.find('[data-testid="layout-columns-4"]').trigger('click')
    resolvePrefs({ ok: true, json: () => Promise.resolve({ dashboard_layout: JSON.stringify({ order: [], hidden: [], columns: 2 }) }) })
    await flushPromises()
    expect(wrapper.find('[data-testid="stats-grid"]').classes()).toContain('lg:grid-cols-4')
  })
})
