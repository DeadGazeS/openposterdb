import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createRouter, createMemoryHistory } from 'vue-router'
import ImageListView from '@/components/ImageListView.vue'

const sampleResponse = {
  items: [
    {
      cache_key: 'imdb/tt0111161',
      release_date: '1994-09-23',
      created_at: 1710000000,
      updated_at: 1710100000,
      last_accessed: 1710200000,
    },
    {
      cache_key: 'tmdb/550',
      release_date: '1999-10-15',
      created_at: 1710000000,
      updated_at: 1710100000,
      last_accessed: 1710200000,
    },
  ],
  total: 2,
  page: 1,
  page_size: 50,
}

function makeMocks() {
  return {
    listFn: vi.fn(),
    imageFn: vi.fn(),
    fetchFn: vi.fn(),
    deleteFn: vi.fn(),
    clearAllFn: vi.fn(),
  }
}

function mountView(mocks: ReturnType<typeof makeMocks>, kind: 'poster' | 'logo' | 'backdrop' = 'poster') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
  return mount(ImageListView, {
    props: {
      kind,
      listFn: mocks.listFn,
      imageFn: mocks.imageFn,
      fetchFn: mocks.fetchFn,
      deleteFn: mocks.deleteFn,
      clearAllFn: mocks.clearAllFn,
    },
    global: {
      plugins: [createPinia(), router, [VueQueryPlugin, { queryClient }]],
      stubs: {
        Button: {
          template: '<button @click="$emit(\'click\', $event)" :disabled="disabled"><slot /></button>',
          props: ['disabled', 'variant', 'size', 'type'],
          emits: ['click'],
        },
        Skeleton: { template: '<div data-testid="skeleton" />' },
        Table: { template: '<table><slot /></table>' },
        TableHeader: { template: '<thead><slot /></thead>' },
        TableBody: { template: '<tbody><slot /></tbody>' },
        TableRow: { template: '<tr><slot /></tr>' },
        TableHead: { template: '<th><slot /></th>' },
        TableCell: { template: '<td><slot /></td>' },
        RefreshButton: {
          template: '<button @click="$emit(\'refresh\')">Refresh</button>',
          props: ['fetching'],
          emits: ['refresh'],
        },
        Dialog: { template: '<div v-if="open"><slot /></div>', props: ['open'] },
        DialogContent: { template: '<div><slot /></div>' },
        DialogHeader: { template: '<div><slot /></div>' },
        DialogTitle: { template: '<div><slot /></div>' },
        Input: {
          template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
          props: ['modelValue'],
          emits: ['update:modelValue'],
        },
        Download: { template: '<span />' },
        Loader2: { template: '<span />' },
        Eye: { template: '<span />' },
        Info: { template: '<span />' },
        Trash2: { template: '<span />' },
        ChevronRight: { template: '<span />' },
        ChevronUp: { template: '<span />' },
        ChevronDown: { template: '<span />' },
      },
    },
  })
}

describe('ImageListView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('shows skeletons while loading', () => {
    const mocks = makeMocks()
    mocks.listFn.mockReturnValue(new Promise(() => {}))
    const wrapper = mountView(mocks)
    expect(wrapper.findAll('[data-testid="skeleton"]').length).toBeGreaterThan(0)
  })

  it('renders list with parsed cache keys', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleResponse),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    expect(wrapper.text()).toContain('imdb')
    expect(wrapper.text()).toContain('tt0111161')
    expect(wrapper.text()).toContain('tmdb')
    expect(wrapper.text()).toContain('550')
    expect(wrapper.text()).toContain('23.09.1994')
  })

  it('shows empty state when no items', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ items: [], total: 0, page: 1, page_size: 50 }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    expect(wrapper.text()).toContain('No posters cached yet.')
  })

  it('shows total count and pagination info', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleResponse),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    expect(wrapper.text()).toContain('2 posters total')
    expect(wrapper.text()).toContain('Page 1 of 1')
  })

  it('has a refresh button', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleResponse),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const refreshButton = wrapper.findAll('button').find((b) => b.text().includes('Refresh'))
    expect(refreshButton).toBeDefined()
  })

  it('has a fetch button', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleResponse),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const fetchButton = wrapper.findAll('button').find((b) => b.text().includes('Fetch'))
    expect(fetchButton).toBeDefined()
  })

  it('opens fetch modal when fetch button is clicked', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleResponse),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Modal content should not be visible initially
    expect(wrapper.text()).not.toContain('Fetch Poster')

    const fetchButton = wrapper.findAll('button').find((b) => b.text().includes('Fetch'))
    await fetchButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Fetch Poster')
    expect(wrapper.text()).toContain('ID Type')
    expect(wrapper.text()).toContain('ID Value')
  })

  it('calls fetchFn on form submit', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleResponse),
    })
    mocks.fetchFn.mockResolvedValue({
      ok: true,
      blob: () => Promise.resolve(new Blob()),
    })
    mocks.imageFn.mockResolvedValue({
      ok: true,
      blob: () => Promise.resolve(new Blob()),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Open modal
    const fetchButton = wrapper.findAll('button').find((b) => b.text().includes('Fetch'))
    await fetchButton!.trigger('click')
    await flushPromises()

    // Fill in ID value
    const input = wrapper.find('input')
    await input.setValue('tt0111161')
    await flushPromises()

    // Submit form
    const form = wrapper.find('form')
    await form.trigger('submit')
    await flushPromises()

    expect(mocks.fetchFn).toHaveBeenCalledWith('imdb', 'tt0111161')
  })

  async function openPurgeDialog(mocks: ReturnType<typeof makeMocks>) {
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [{ cache_key: 'imdb/tt0111161_t_de@imc', release_date: null, created_at: 1710000000, updated_at: 1710000000, last_accessed: 1710000000 }],
        total: 1,
        page: 1,
        page_size: 50,
      }),
    })
    mocks.deleteFn.mockResolvedValue({ ok: true, json: () => Promise.resolve({ ok: true }) })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Confirm dialog is not shown until the row's purge button is clicked.
    expect(wrapper.text()).not.toContain('Purge poster')

    const purgeButton = wrapper.findAll('button').find((b) => b.attributes('aria-label') === 'Purge poster')
    expect(purgeButton).toBeDefined()
    await purgeButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Purge poster')
    return wrapper
  }

  it('purges the whole title via the "Entire title" button (bare title id)', async () => {
    const mocks = makeMocks()
    const wrapper = await openPurgeDialog(mocks)

    const titleButton = wrapper.findAll('button').find((b) => b.text().trim() === 'Entire title')
    await titleButton!.trigger('click')
    await flushPromises()

    // The full cache value carries a variant + ratings suffix; the title purge
    // targets the bare title id.
    expect(mocks.deleteFn).toHaveBeenCalledWith('imdb', 'tt0111161', 'title')
  })

  it('purges a single variant via the "This variant" button (full cache value)', async () => {
    const mocks = makeMocks()
    const wrapper = await openPurgeDialog(mocks)

    const variantButton = wrapper.findAll('button').find((b) => b.text().trim() === 'This variant')
    await variantButton!.trigger('click')
    await flushPromises()

    expect(mocks.deleteFn).toHaveBeenCalledWith('imdb', 'tt0111161_t_de@imc', 'variant')
  })

  it('clears every image of the kind via the header "Clear" button', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })
    mocks.clearAllFn.mockResolvedValue({ ok: true, json: () => Promise.resolve({ ok: true, meta_deleted: 2 }) })

    const wrapper = mountView(mocks)
    await flushPromises()

    // The header button opens the confirm dialog (which adds a second "Clear posters" button).
    const trigger = wrapper.findAll('button').find((b) => b.text().trim() === 'Clear posters')
    expect(trigger).toBeDefined()
    await trigger!.trigger('click')
    await flushPromises()

    const confirmButtons = wrapper.findAll('button').filter((b) => b.text().trim() === 'Clear posters')
    expect(confirmButtons.length).toBe(2)
    await confirmButtons[confirmButtons.length - 1]!.trigger('click')
    await flushPromises()

    expect(mocks.clearAllFn).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Cleared 2 cached posters')
  })

  it('uses the singular noun when one image is cleared', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })
    mocks.clearAllFn.mockResolvedValue({ ok: true, json: () => Promise.resolve({ ok: true, meta_deleted: 1 }) })

    const wrapper = mountView(mocks)
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text().trim() === 'Clear posters')!.trigger('click')
    await flushPromises()
    const confirm = wrapper.findAll('button').filter((b) => b.text().trim() === 'Clear posters')
    await confirm[confirm.length - 1]!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Cleared 1 cached poster.')
  })

  it('shows an error and keeps the dialog open when clear fails', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })
    mocks.clearAllFn.mockResolvedValue({ ok: false, status: 500, text: () => Promise.resolve(JSON.stringify({ error: 'boom' })) })

    const wrapper = mountView(mocks)
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text().trim() === 'Clear posters')!.trigger('click')
    await flushPromises()
    const confirm = wrapper.findAll('button').filter((b) => b.text().trim() === 'Clear posters')
    await confirm[confirm.length - 1]!.trigger('click')
    await flushPromises()

    expect(mocks.clearAllFn).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('boom')
    // Dialog stays open (the confirm button is still present).
    expect(wrapper.findAll('button').filter((b) => b.text().trim() === 'Clear posters').length).toBe(2)
  })

  // --- 2026-08-11 Logos admin UI refresh -----------------------------------

  it('collapsed row reflects the active sort (first row by current sortBy)', async () => {
    const mocks = makeMocks()
    // Default sort is last_accessed DESC — both items have last_accessed: 0
    // so the stable-sort tie would pick imdb. Switch to "Last Updated" so
    // the active sort exercises a real field difference: tmdb has the larger
    // updated_at → tmdb becomes preferred.
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710000100, last_accessed: 0 },
          { cache_key: 'tmdb/550',         release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710000999, last_accessed: 0 },
        ],
        total: 2, page: 1, page_size: 50,
      }),
    })
    mocks.imageFn.mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob()) })

    const wrapper = mountView(mocks)
    await flushPromises()

    const lastUpdatedBtn = () =>
      wrapper.findAll('th').find((th) => th.text().includes('Last Updated'))!.find('button')!
    await lastUpdatedBtn().trigger('click')
    await flushPromises()

    // After clicking "Last Updated" (sortBy=updated_at, sortDir=desc), tmdb
    // (updated_at: 1710000999) wins over imdb (updated_at: 1710000100).
    const body = wrapper.findAll('tbody tr')
    expect(body.length).toBeGreaterThan(0)
    expect(body[0]!.text()).toContain('tmdb')
    expect(body[0]!.text()).toContain('550')
  })

  it('flipping the active sort to ASC swaps the preferred row within a group', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710000100, last_accessed: 0 },
          { cache_key: 'tmdb/550',         release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710000999, last_accessed: 0 },
        ],
        total: 2, page: 1, page_size: 50,
      }),
    })
    mocks.imageFn.mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob()) })

    const wrapper = mountView(mocks)
    await flushPromises()

    const lastUpdatedBtn = () =>
      wrapper.findAll('th').find((th) => th.text().includes('Last Updated'))!.find('button')!
    // First click: switch column → sortDir defaults to desc → tmdb preferred.
    await lastUpdatedBtn().trigger('click')
    await flushPromises()
    expect(wrapper.findAll('tbody tr')[0]!.text()).toContain('tmdb')

    // Second click on same column → flips to asc → imdb (smaller updated_at) preferred.
    await lastUpdatedBtn().trigger('click')
    await flushPromises()
    expect(wrapper.findAll('tbody tr')[0]!.text()).toContain('imdb')
  })

  it('passes sortBy and sortDir through to listFn on initial fetch', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })

    mountView(mocks)
    await flushPromises()

    expect(mocks.listFn).toHaveBeenCalledWith(1, 50, 'last_accessed', 'desc')
  })

  it('flips sort direction on a second click of the same header', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })

    const wrapper = mountView(mocks)
    await flushPromises()

    // The click listener is on the <button> inside the <th>, so click the button.
    const createdHeaderBtn = () =>
      wrapper.findAll('th').find((th) => th.text().includes('Created'))!.find('button')!

    // Default is last_accessed/desc; first click on Created switches column
    // (column switches default to desc), not a same-column flip.
    await createdHeaderBtn().trigger('click')
    await flushPromises()
    expect(mocks.listFn).toHaveBeenLastCalledWith(1, 50, 'created_at', 'desc')

    // Second click on the same column flips to asc.
    await createdHeaderBtn().trigger('click')
    await flushPromises()
    expect(mocks.listFn).toHaveBeenLastCalledWith(1, 50, 'created_at', 'asc')

    // Third click on the same column flips back to desc.
    await createdHeaderBtn().trigger('click')
    await flushPromises()
    expect(mocks.listFn).toHaveBeenLastCalledWith(1, 50, 'created_at', 'desc')
  })

  it('switches column and resets to desc when a different header is clicked', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })

    const wrapper = mountView(mocks)
    await flushPromises()

    const releaseHeaderBtn = () =>
      wrapper.findAll('th').find((th) => th.text().includes('Release Date'))!.find('button')!

    // Click Release Date — switches from last_accessed/desc to release_date/desc.
    await releaseHeaderBtn().trigger('click')
    await flushPromises()
    expect(mocks.listFn).toHaveBeenLastCalledWith(1, 50, 'release_date', 'desc')
  })

  it('Last Accessed header renders a clickable button and clicking it sets sortBy to last_accessed (defaulting to desc on first click from a different column)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Initial default: listFn called with last_accessed/desc.
    expect(mocks.listFn).toHaveBeenCalledWith(1, 50, 'last_accessed', 'desc')

    // Click Release Date first to leave the default column.
    const releaseBtn = () =>
      wrapper.findAll('th').find((th) => th.text().includes('Release Date'))!.find('button')!
    await releaseBtn().trigger('click')
    await flushPromises()
    expect(mocks.listFn).toHaveBeenLastCalledWith(1, 50, 'release_date', 'desc')

    // Now click Last Accessed — switches column, defaults to desc.
    const lastAccessedBtn = () =>
      wrapper.findAll('th').find((th) => th.text().includes('Last Accessed'))!.find('button')!
    expect(lastAccessedBtn().exists()).toBe(true)
    await lastAccessedBtn().trigger('click')
    await flushPromises()
    expect(mocks.listFn).toHaveBeenLastCalledWith(1, 50, 'last_accessed', 'desc')
  })

  it('Last Accessed header renders a chevron-down indicator when it is the active sort column', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Default sort is last_accessed/desc → the Last Accessed header button
    // renders the ChevronDown indicator. The lucide-vue-next stubs in the
    // test setup are not applied to the icon component (the actual lucide
    // SVG renders, with class "lucide-chevron-down"); assert against the
    // real SVG class name rather than the stubbed <span />.
    const lastAccessedHeader = wrapper.findAll('th').find((th) => th.text().includes('Last Accessed'))!
    const lastAccessedBtn = lastAccessedHeader.find('button')!
    expect(lastAccessedBtn.exists()).toBe(true)
    expect(lastAccessedBtn.find('svg.lucide-chevron-down').exists()).toBe(true)

    // The other sortable headers are inactive and have no chevron SVG.
    for (const label of ['Release Date', 'Created', 'Last Updated']) {
      const th = wrapper.findAll('th').find((t) => t.text().includes(label))!
      const btn = th.find('button')!
      expect(btn.find('svg').exists()).toBe(false)
    }
  })

  it('flipping direction reorders rows even when every row ties on the sort column (cache_key tiebreaker)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          // Same group; ALL sortable columns tie (same-second timestamps,
          // last_accessed 0). The direction-following cache_key tiebreaker is
          // the only thing that orders them — 'tmdb/550' > 'imdb/tt0111161'.
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'tmdb/550',        release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 2, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Default sort last_accessed DESC, all ties → cache_key DESC → tmdb preferred.
    expect(wrapper.findAll('tbody tr')[0]!.text()).toContain('tmdb')

    // Click the active Last Accessed header → flips to ASC → cache_key ASC → imdb preferred.
    const lastAccessedBtn = () =>
      wrapper.findAll('th').find((th) => th.text().includes('Last Accessed'))!.find('button')!
    await lastAccessedBtn().trigger('click')
    await flushPromises()
    expect(wrapper.findAll('tbody tr')[0]!.text()).toContain('imdb')
  })

  it('sortable headers are standard black; the active one is bright grey and underlined', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({ ok: true, json: () => Promise.resolve(sampleResponse) })

    const wrapper = mountView(mocks)
    await flushPromises()

    const headerBtn = (label: string) =>
      wrapper.findAll('th').find((th) => th.text().includes(label))!.find('button')!
    const labelSpan = (btn: ReturnType<typeof headerBtn>) => btn.findAll('span')[0]!

    // Default: Last Accessed is the active column → bright grey + underlined label.
    expect(headerBtn('Last Accessed').classes()).toContain('text-zinc-500')
    expect(headerBtn('Last Accessed').classes()).toContain('dark:text-zinc-400')
    expect(labelSpan(headerBtn('Last Accessed')).classes()).toContain('underline')
    for (const label of ['Release Date', 'Created', 'Last Updated']) {
      expect(headerBtn(label).classes()).toContain('text-foreground')
      expect(headerBtn(label).classes()).not.toContain('text-muted-foreground')
      expect(headerBtn(label).classes()).not.toContain('text-zinc-500')
      expect(labelSpan(headerBtn(label)).classes()).not.toContain('underline')
    }

    // Click Created → it becomes the bright grey + underlined one; Last Accessed
    // returns to standard black without underline.
    await headerBtn('Created').trigger('click')
    await flushPromises()
    expect(headerBtn('Created').classes()).toContain('text-zinc-500')
    expect(labelSpan(headerBtn('Created')).classes()).toContain('underline')
    expect(headerBtn('Last Accessed').classes()).toContain('text-foreground')
    expect(headerBtn('Last Accessed').classes()).not.toContain('text-zinc-500')
    expect(labelSpan(headerBtn('Last Accessed')).classes()).not.toContain('underline')
  })

  it('clicking the collapsed row opens preview (does not toggle expand)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          // tmdb has strictly-higher updated_at AND last_accessed than imdb
          // (same group). Default sort is last_accessed DESC, so tmdb is the
          // collapsed-row preferred.
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 1710200000 },
          { cache_key: 'tmdb/550',        release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100999, last_accessed: 1710300000 },
        ],
        total: 2, page: 1, page_size: 50,
      }),
    })
    mocks.imageFn.mockResolvedValue({
      ok: true,
      blob: () => Promise.resolve(new Blob()),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const collapsedRow = wrapper.findAll('tbody tr')[0]!
    await collapsedRow.trigger('click')
    await flushPromises()

    // imageFn is called for the preferred row's key → preview opens.
    expect(mocks.imageFn).toHaveBeenCalledWith('tmdb/550')
    expect(wrapper.text()).toContain('tmdb/550')
    // imageFn was called exactly once (the chevron/+N click would NOT have
    // produced a preview).
    expect(mocks.imageFn).toHaveBeenCalledTimes(1)
  })

  it('clicking the chevron/+N cluster toggles expand without opening preview', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 1710200000 },
          { cache_key: 'tmdb/550',        release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100999, last_accessed: 1710200000 },
        ],
        total: 2, page: 1, page_size: 50,
      }),
    })
    mocks.imageFn.mockResolvedValue({
      ok: true,
      blob: () => Promise.resolve(new Blob()),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const expandBtn = wrapper.findAll('button').find((b) => b.attributes('aria-label') === 'Expand variants')!
    expect(expandBtn).toBeDefined()
    await expandBtn.trigger('click')
    await flushPromises()

    // imageFn NOT called — expand cluster must not propagate to row click.
    expect(mocks.imageFn).not.toHaveBeenCalled()
    // After expand, the cluster is re-labelled "Collapse variants".
    const collapseBtn = wrapper.findAll('button').find((b) => b.attributes('aria-label') === 'Collapse variants')
    expect(collapseBtn).toBeDefined()
  })

  // --- 2026-08-11 followup bug fixes (post-review) -------------------------

  it('renders "—" instead of "20676d ago" when last_accessed is 0 (never served)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 1, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // The collapsed row has 8 cells; the 5th (index 4, zero-based) is
    // "Last Accessed" (column order: expand, ID Type, ID Value, Release Date,
    // Last Accessed, Last Updated, Created, actions).
    const cells = wrapper.findAll('tbody tr')[0]!.findAll('td')
    expect(cells[4]!.text()).toBe('—')
  })

  it('Created / Last Updated render relativeTime of their OWN fields; Release Date renders dd.mm.yyyy', async () => {
    // Regression: the two columns were swapped (Created showed updated_at,
    // Last Updated showed created_at via formatDate). Distinct values here
    // make a swap fail the test. Release Date must be zero-padded dd.mm.yyyy.
    // Column order: 3 = Release Date, 4 = Last Accessed, 5 = Last Updated,
    // 6 = Created (furthest right per user spec).
    const now = Math.floor(Date.now() / 1000)
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: now - 3 * 3600, updated_at: now - 30 * 60, last_accessed: 0 },
        ],
        total: 1, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const cells = wrapper.findAll('tbody tr')[0]!.findAll('td')
    expect(cells[3]!.text()).toBe('23.09.1994')
    expect(cells[4]!.text()).toBe('—')
    expect(cells[5]!.text()).toBe('30m ago')
    expect(cells[6]!.text()).toBe('3h ago')
  })

  it('expand button renders only the chevron (no count badge)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'tmdb/550',        release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'tvdb/12345',      release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 3, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const expandBtn = wrapper.findAll('button').find((b) => b.attributes('aria-label') === 'Expand variants')!
    expect(expandBtn).toBeDefined()
    // The button's visible label is just the chevron — no "+N" or "more" text.
    expect(expandBtn.text()).not.toContain('+')
    expect(expandBtn.text()).not.toContain('more')
  })

  it('expanded variant rows have no eye icon in the first cell', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'tmdb/550',        release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'tvdb/12345',      release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 3, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const expandBtn = wrapper.findAll('button').find((b) => b.attributes('aria-label') === 'Expand variants')!
    await expandBtn.trigger('click')
    await flushPromises()

    // No <svg> eye icon anywhere in the table — variant rows have empty first cells.
    expect(wrapper.findAll('svg.lucide-eye').length).toBe(0)
  })

  it('last variant row in an expanded group gets a thick black bottom border', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'tmdb/550',        release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'tvdb/12345',      release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 3, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    const expandBtn = wrapper.findAll('button').find((b) => b.attributes('aria-label') === 'Expand variants')!
    await expandBtn.trigger('click')
    await flushPromises()

    // Row layout: [0] preferred, [1] first variant, [2] last variant.
    const rows = wrapper.findAll('tbody tr')
    const lastVariantRow = rows[2]!
    const firstVariantRow = rows[1]!

    // The last variant row gets a thick black bottom border to visually
    // separate the expanded group from the next one.
    expect(lastVariantRow.classes()).toContain('border-b-2')
    expect(lastVariantRow.classes()).toContain('border-foreground')

    // The first variant row (and the preferred row) keep the normal thin border.
    expect(firstVariantRow.classes()).not.toContain('border-b-2')
    expect(rows[0]!.classes()).not.toContain('border-b-2')
  })

  it('a solo row (no variants) renders no expand button and no chevron in the first cell', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          // Single item, different release dates → three separate groups, each with no others.
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'imdb/tt0111162', release_date: '1972-03-17',  created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          { cache_key: 'imdb/tt0111163', release_date: '1997-06-11',  created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 3, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // No expand-cluster button anywhere in the rendered tree.
    expect(wrapper.findAll('button[aria-label="Expand variants"]').length).toBe(0)
    expect(wrapper.findAll('button[aria-label="Collapse variants"]').length).toBe(0)

    // Each row's first cell is empty (no chevron either — the cluster is gated on others.length).
    const rows = wrapper.findAll('tbody tr')
    expect(rows.length).toBe(3)
    for (const row of rows) {
      const firstCell = row.findAll('td')[0]!
      expect(firstCell.text()).toBe('')
      // No <button> child at all.
      expect(firstCell.find('button').exists()).toBe(false)
    }
  })

  it('multi-row group renders the expand button; solo group does not (mixed fixture)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          // Group A: imdb + tmdb same release date → 1 variant beyond preferred.
          { cache_key: 'imdb/tt0111161', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100999, last_accessed: 0 },
          { cache_key: 'tmdb/550',        release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
          // Group B: solo, different release date → 0 variants.
          { cache_key: 'imdb/tt0111162', release_date: '1972-03-17',  created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 3, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Exactly one Expand variants button (from the multi-row group). The solo
    // group's preferred row contributes no button (its cluster is gated off).
    expect(wrapper.findAll('button[aria-label="Expand variants"]').length).toBe(1)

    // Total rows: 2 preferred rows + 1 variant row (the multi-row group's
    // variant, hidden via v-show but still in the DOM).
    const rows = wrapper.findAll('tbody tr')
    expect(rows.length).toBe(3)

    // The cluster button is attached to exactly one preferred row — the
    // multi-row group's. Walk each row; only the one whose first cell
    // contains the Expand button is the multi-row preferred.
    const rowWithCluster = rows.filter(
      (r) => r.findAll('td')[0]!.find('button[aria-label="Expand variants"]').exists(),
    )
    expect(rowWithCluster.length).toBe(1)
  })

  // --- 2026-08-11 short-id + more-info dialog -----------------------------

  it('ID Value cell shows only the bare idValue (no variant + suffix tail)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          // Cache key with a logo variant + long settings tail. The ID Value
          // cell must stop at the first `_` or `@`, never rendering the tail.
          {
            cache_key: 'imdb/tt9813792_l_f_en@icatmk.stb.lh.ly45615199.ts155.bz90.bw120.bh70.shr.ba90.zm.colc9853ee4',
            release_date: '2026-01-01',
            created_at: 1710000000,
            updated_at: 1710100000,
            last_accessed: 0,
          },
          // Backdrop variant with no lang segment.
          {
            cache_key: 'tmdb/124364_b_f@imc.stb.lh',
            release_date: '2014-01-01',
            created_at: 1710000000,
            updated_at: 1710100000,
            last_accessed: 0,
          },
        ],
        total: 2, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Each row's 3rd cell (index 2) is ID Value — must be the short id only.
    const rows = wrapper.findAll('tbody tr')
    expect(rows.length).toBe(2)
    const idValueCells = rows.map((r) => r.findAll('td')[2]!)
    expect(idValueCells[0]!.text()).toBe('tt9813792')
    expect(idValueCells[1]!.text()).toBe('124364')
    // And the long tail must NOT leak into the ID Value cell text.
    for (const cell of idValueCells) {
      expect(cell.text()).not.toContain('@')
      expect(cell.text()).not.toContain('_l_')
      expect(cell.text()).not.toContain('stb')
    }
  })

  it('clicking the Info button opens the more-info dialog with the parsed breakdown', async () => {
    const mocks = makeMocks()
    const cacheKey = 'imdb/tt9813792_l_f_en@icatmk.stb.lh.ly45615199.ts155.bz90.bw120.bh70.shr.ba90.zm.colc9853ee4'
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: cacheKey, release_date: '2026-01-01', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 1, page: 1, page_size: 50,
      }),
    })

    const wrapper = mountView(mocks)
    await flushPromises()

    // Click the Info icon (aria-label "Show cache_key breakdown") in the
    // collapsed row.
    const infoBtn = wrapper.findAll('button[aria-label="Show cache_key breakdown"]')[0]!
    expect(infoBtn).toBeDefined()
    await infoBtn.trigger('click')
    await flushPromises()

    // Dialog body is rendered. The Dialog stub renders its slot when open=true,
    // so the body text is visible.
    const text = wrapper.text()
    expect(text).toContain('imdb')           // idType
    expect(text).toContain('tt9813792')      // idValue
    expect(text).toContain('logo')           // variant kind
    expect(text).toContain('fanart')         // variant source
    expect(text).toContain('en')             // variant lang
    // Ratings chips: i=IMDb, c=RTC, a=RTA, m=MC, t=TMDB, k=Trakt
    expect(text).toContain('IMDb (i)')
    expect(text).toContain('Rotten Tomatoes Critics (c)')
    expect(text).toContain('Rotten Tomatoes Audience (a)')
    expect(text).toContain('Metacritic (m)')
    expect(text).toContain('TMDB (t)')
    expect(text).toContain('Trakt (k)')
    // Some of the setting labels:
    expect(text).toContain('Badge style')
    expect(text).toContain('Layout hash')
    expect(text).toContain('Badge size')
    expect(text).toContain('Badge shape')
    expect(text).toContain('Colors hash')
    // Setting values for the longer entries:
    expect(text).toContain('45615199')
    expect(text).toContain('c9853ee4')
  })

  it('clicking the Info button does NOT trigger preview (click.stop)', async () => {
    const mocks = makeMocks()
    mocks.listFn.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { cache_key: 'imdb/tt0111161_l_f_en@ir', release_date: '1994-09-23', created_at: 1710000000, updated_at: 1710100000, last_accessed: 0 },
        ],
        total: 1, page: 1, page_size: 50,
      }),
    })
    mocks.imageFn.mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob()) })

    const wrapper = mountView(mocks)
    await flushPromises()

    const infoBtn = wrapper.findAll('button[aria-label="Show cache_key breakdown"]')[0]!
    await infoBtn.trigger('click')
    await flushPromises()

    // imageFn NOT called — Info button must not propagate to the row click.
    expect(mocks.imageFn).not.toHaveBeenCalled()
  })
})
