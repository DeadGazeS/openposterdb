import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import ApiKeysView from '@/views/ApiKeysView.vue'

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

vi.mock('@/lib/api', () => ({
  keysApi: mockKeysApi,
  adminApi: mockAdminApi,
}))

const sampleKeys = [
  {
    id: 1,
    name: 'jellyfin-prod',
    key_prefix: 'opdb_abc',
    created_at: '2026-01-01',
    last_used_at: null,
  },
  {
    id: 2,
    name: 'plex-dev',
    key_prefix: 'opdb_xyz',
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
        Button: {
          template: '<button @click="$emit(\'click\')"><slot /></button>',
          props: ['disabled', 'variant', 'size'],
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
    expect(wrapper.text()).toContain('opdb_abc')
    expect(wrapper.text()).toContain('opdb_xyz')
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

  it('shows settings button for each key', async () => {
    mockKeysApi.list.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(sampleKeys),
    })

    const wrapper = mountView()
    await flushPromises()

    // Each key should have a settings icon button
    const settingsButtons = wrapper.findAll('button').filter((b) => {
      // The Settings button contains the Settings lucide icon
      return b.find('svg') !== undefined && !b.text().includes('Delete') && !b.text().includes('Create') && !b.text().includes('Refresh')
    })
    // At minimum we should have some non-delete, non-create buttons (the settings gear buttons)
    expect(settingsButtons.length).toBeGreaterThanOrEqual(2)
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
})
