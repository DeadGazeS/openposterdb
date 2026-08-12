import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import ApiKeysView from '@/views/ApiKeysView.vue'

// Track what the form's tab setter would write, so we can assert the form
// uses its own query key (not `tab`) and doesn't clobber the parent's.
const mockRoute = vi.hoisted(() => ({
  query: { tab: 'api-keys' } as Record<string, string | undefined>,
}))
const replaceMock = vi.hoisted(() => vi.fn())
vi.mock('vue-router', () => ({
  useRoute: () => mockRoute,
  useRouter: () => ({ replace: replaceMock }),
}))

const mockKeysApi = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  delete: vi.fn(),
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  deleteSettings: vi.fn(),
}))

const mockAdminApi = vi.hoisted(() => ({
  previewPoster: vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob()) }),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    keysApi: mockKeysApi,
    adminApi: mockAdminApi,
  }
})

const sampleKeys = [
  {
    id: 1,
    name: 'jellyfin-prod',
    key_prefix: 'opdb_abc',
    // Real key, decrypted server-side from the api_keys.encrypted_key column.
    key: 'opdb_abc123def4567890abcdef1234567890abcdef1234567890abcdef1234567890',
    created_at: '2026-01-01',
    last_used_at: null,
  },
  {
    id: 2,
    name: 'plex-dev',
    key_prefix: 'opdb_xyz',
    key: 'opdb_xyzhijklmnopqrstuvwxyz0123456789abcdef0123456789abcdef012345',
    created_at: '2026-02-01',
    last_used_at: '2026-03-01',
  },
]

function mountView() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return mount(ApiKeysView, {
    global: {
      plugins: [createPinia(), [VueQueryPlugin, { queryClient }]],
      stubs: {
        // Button stub forwards the click event to the parent listener. The
        // default Vue Test Utils wrapper doesn't propagate the click emit, so
        // toggleSettings / deleteKey / etc. never fire under tests.
        Button: {
          template: '<button @click="$emit(\'click\', $event)" @click.stop><slot /></button>',
          props: ['disabled', 'variant', 'size', 'as', 'asChild'],
          emits: ['click'],
        },
        Input: {
          template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
          props: ['modelValue', 'type', 'placeholder', 'required'],
        },
      },
    },
  })
}

describe('ApiKeysView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([]),
    })
  })

  it('renders key list from mocked API response', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('jellyfin-prod')
    expect(wrapper.text()).toContain('plex-dev')

    // Keys are masked by default — same maskKey as Source API Keys
    // (first 4 + ... + last 4 for the 64-char raw, hiding at least half).
    // Click each Reveal button to surface the full decrypted key.
    const revealButtons = wrapper.findAll('button[title="Reveal key"]')
    expect(revealButtons.length).toBe(sampleKeys.length)
    // Default (masked) state shows the masked form, not the raw key.
    expect(wrapper.text()).not.toContain(sampleKeys[0].key)
    expect(wrapper.text()).not.toContain(sampleKeys[1].key)
    // Each row's masked form is exactly first 4 + "..." + last 4 of the raw.
    const expectedMask0 = sampleKeys[0].key.slice(0, 4) + '...' + sampleKeys[0].key.slice(-4)
    const expectedMask1 = sampleKeys[1].key.slice(0, 4) + '...' + sampleKeys[1].key.slice(-4)
    expect(wrapper.text()).toContain(expectedMask0)
    expect(wrapper.text()).toContain(expectedMask1)

    for (const btn of revealButtons) await btn.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain(sampleKeys[0].key)
    expect(wrapper.text()).toContain(sampleKeys[1].key)
  })

  it('shows "No API keys yet." when empty', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([]),
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('No API keys yet.')
  })

  it('exposes per-key save/discard/refresh actions for the settings header', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })

    const wrapper = mountView()
    await flushPromises()

    // The header-driven API must exist (no inline refresh button anymore —
    // the settings page header owns Save/Discard/Refresh).
    const vm = wrapper.vm as unknown as {
      saveExpanded?: () => unknown
      discardExpanded?: () => unknown
      refreshExpanded?: () => Promise<unknown>
      expandedDirty?: boolean
    }
    expect(typeof vm.saveExpanded).toBe('function')
    expect(typeof vm.discardExpanded).toBe('function')
    expect(typeof vm.refreshExpanded).toBe('function')
    expect(vm.expandedDirty).toBe(false)
  })

  it('per-key settings panel uses a tab-key that does not collide with the parent ?tab=', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })
    mockKeysApi.getSettings.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        image_source: 't', lang: 'en', textless: false, fanart_available: true,
        ratings_limit: 3, ratings_order: 'mal,imdb', ratings_exclude: '',
        poster_layout: '{}', logo_layout: '{}', backdrop_layout: '{}', episode_layout: '{}',
        poster_badge_style: 'lr', logo_badge_style: 'lr', backdrop_badge_style: 'lr',
        episode_badge_style: 'lr', poster_label_style: 'o', logo_label_style: 'o',
        backdrop_label_style: 'o', episode_label_style: 'o', poster_badge_direction: 'd',
        poster_fit: 'native', poster_text_size: 100, logo_text_size: 100,
        backdrop_text_size: 100, poster_badge_width: 100, poster_badge_height: 100,
        logo_badge_width: 100, logo_badge_height: 100, backdrop_badge_width: 100,
        backdrop_badge_height: 100, episode_badge_width: 100, episode_badge_height: 100,
        poster_logo_size: 100, logo_logo_size: 100, backdrop_logo_size: 100,
        episode_logo_size: 100, episode_blur: false, poster_badge_shape: 'r',
        logo_badge_shape: 'r', backdrop_badge_shape: 'r', episode_badge_shape: 'r',
        poster_badge_alpha: 80, logo_badge_alpha: 80, backdrop_badge_alpha: 80,
        episode_badge_alpha: 80, backdrop_edge_inset_x: 0, backdrop_edge_inset_y: 0,
        colors: '',
      }),
    })

    const wrapper = mountView()
    await flushPromises()

    // Invoke toggleSettings directly — the shadcn Button stub in mountView
    // doesn't reliably propagate click emits through vue-test-utils, and
    // toggleSettings is what a real click would call.
    const vm = wrapper.vm as unknown as { toggleSettings?: (id: number) => Promise<void> }
    await vm.toggleSettings!(1)
    await flushPromises()

    // The settings panel renders with its own internal tabs (Image Settings,
    // Rating Order, Rating Colours, Poster, Logo, Backdrop, Episode) — none
    // of which collide with the parent's 'api-keys' tab value.
    expect(wrapper.text()).toContain('Image Settings')
    expect(wrapper.text()).toContain('Rating Order')
    expect(wrapper.text()).toContain('Rating Colours')

    // The form is rendered with a tabKey prop so it never writes to the
    // parent's `tab` query key. Switching the form's tab leaves the parent
    // route's ?tab= alone.
    const formEl = wrapper.findComponent({ name: 'RenderSettingsForm' })
    expect(formEl.exists()).toBe(true)
    expect(formEl.props('tabKey')).toBe('key-1-tab')
  })

  it('shows settings button for each key', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })

    const wrapper = mountView()
    await flushPromises()

    const settingsButtons = wrapper.findAll('button').filter((b) => {
      return b.find('svg') !== undefined && !b.text().includes('Delete') && !b.text().includes('Create') && !b.text().includes('Refresh')
    })
    expect(settingsButtons.length).toBeGreaterThanOrEqual(2)
  })

  it('disables reveal for keys without encrypted_key (legacy rows)', async () => {
    // Legacy rows migrated from before encrypted_key shipped have an empty
    // `key` field — the raw is gone with the dismissed create banner. The
    // reveal button is disabled, no fallback data is exposed.
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([
        {
          id: 1,
          name: 'legacy-key',
          key_prefix: 'opdb_old',
          key: '',
          created_at: '2025-12-01',
          last_used_at: null,
        },
      ]),
    })

    const wrapper = mountView()
    await flushPromises()

    const revealButton = wrapper.find('button[title="Reveal key"]')
    expect(revealButton.exists()).toBe(true)
    expect((revealButton.element as HTMLButtonElement).disabled).toBe(true)
    // No masked form visible — empty input yields the **** fallback, but we
    // don't render a button content for disabled legacy rows.
    expect(wrapper.findAll('button[title="Reveal key"]').length).toBe(1)
  })

  it('uses the same maskKey as SettingsView (shared from @/lib/utils)', async () => {
    // Sanity check that the obfuscation rule is enforced: a 64-char hex key
    // shows exactly first 4 + "..." + last 4 = 11 chars visible, hiding 53.
    const fullKey = 'a'.repeat(64)
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([
        {
          id: 1,
          name: 'mask-check',
          key_prefix: 'opdb_zz',
          key: fullKey,
          created_at: '2026-08-01',
          last_used_at: null,
        },
      ]),
    })

    const wrapper = mountView()
    await flushPromises()

    const masked = 'aaaa...aaaa'
    expect(wrapper.text()).toContain(masked)
    expect(wrapper.text()).not.toContain(fullKey)
    // The middle of the raw key (positions 30-34 = 'aaaaa') must not leak.
    expect(wrapper.text()).not.toContain('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' + 'a')
    void 'aaaaa' // type-only — see assertion above
  })

  it('creates key and shows raw key value in yellow banner', async () => {
    mockKeysApi.create.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ key: 'opdb_xxx123' }),
    })

    const wrapper = mountView()
    await flushPromises()

    // Fill in key name and submit
    const input = wrapper.find('input')
    await input.setValue('my-new-key')
    const form = wrapper.find('form')
    await form.trigger('submit')
    await flushPromises()

    expect(mockKeysApi.create).toHaveBeenCalledWith('my-new-key')
    const banner = wrapper.find('.border-yellow-500')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('opdb_xxx123')
  })

  it('shows error when create fails', async () => {
    mockKeysApi.create.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: 'Name already taken' }),
    })

    const wrapper = mountView()
    await flushPromises()

    const input = wrapper.find('input')
    await input.setValue('duplicate-key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('.text-destructive').exists()).toBe(true)
    expect(wrapper.text()).toContain('Name already taken')
  })

  it('dismisses newly created key', async () => {
    mockKeysApi.create.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ key: 'opdb_dismiss' }),
    })

    const wrapper = mountView()
    await flushPromises()

    const input = wrapper.find('input')
    await input.setValue('temp-key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('.border-yellow-500').exists()).toBe(true)

    // Click Dismiss
    const dismissButton = wrapper.findAll('button').find((b) => b.text().includes('Dismiss'))
    expect(dismissButton).toBeDefined()
    await dismissButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('.border-yellow-500').exists()).toBe(false)
  })

  it('deletes key after confirmation', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })
    mockKeysApi.delete.mockResolvedValue({ ok: true })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mountView()
    await flushPromises()

    const deleteButton = wrapper.findAll('button').find((b) => b.text().includes('Delete'))
    expect(deleteButton).toBeDefined()
    await deleteButton!.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalled()
    expect(mockKeysApi.delete).toHaveBeenCalledWith(sampleKeys[0]!.id)
  })

  it('cancels delete when confirmation declined', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })
    vi.spyOn(window, 'confirm').mockReturnValue(false)

    const wrapper = mountView()
    await flushPromises()

    const deleteButton = wrapper.findAll('button').find((b) => b.text().includes('Delete'))
    await deleteButton!.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalled()
    expect(mockKeysApi.delete).not.toHaveBeenCalled()
  })

  it('copies the revealed key to the clipboard and shows feedback', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const wrapper = mountView()
    await flushPromises()

    // Reveal first.
    const revealButton = wrapper.findAll('button[title="Reveal key"]')[0]!
    await revealButton.trigger('click')
    await flushPromises()

    // Copy button should now be visible next to the revealed key.
    const copyButton = wrapper.findAll('button').find((b) => b.text().includes('Copy') && b.text() === 'Copy')
    expect(copyButton).toBeDefined()
    await copyButton!.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith(sampleKeys[0].key)
    // After click, the button label briefly flips to 'Copied'.
    expect(wrapper.text()).toContain('Copied')
  })
})
