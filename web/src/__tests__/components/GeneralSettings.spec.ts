import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import GeneralSettings from '@/components/GeneralSettings.vue'
import { shadcnStubs } from '../stubs'

const mockRoute = vi.hoisted(() => ({ query: { tab: 'config' } as Record<string, string | undefined> }))
vi.mock('vue-router', () => ({
  useRoute: () => mockRoute,
  useRouter: () => ({ replace: vi.fn() }),
}))

const mockAdminApi = vi.hoisted(() => ({
  getNetworkSettings: vi.fn(),
  updateNetworkSettings: vi.fn(),
  checkCloudflareRanges: vi.fn(),
}))
vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return { ...actual, adminApi: mockAdminApi }
})

function networkResponse(overrides: Record<string, unknown> = {}) {
  return {
    locked: false,
    settings: { trust_private: true, trust_cloudflare: false, custom: [] },
    cloudflare: { ranges: 22, source: 'built-in', checked_at: 0 },
    detected: { client_ip: '162.158.1.1', remote_addr: '172.18.0.3:5000', forwarded_for: '9.9.9.9, 162.158.1.1' },
    ...overrides,
  }
}

function mountView() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return mount(GeneralSettings, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: {
        ...shadcnStubs,
        Input: {
          template: '<input :value="modelValue" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" />',
          props: ['modelValue', 'disabled', 'placeholder'],
        },
      },
    },
  })
}

const cloudflareBox = (w: ReturnType<typeof mountView>) => w.find('[data-testid="trust-cloudflare"]')

describe('GeneralSettings (Config: client IP & reverse proxy)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAdminApi.getNetworkSettings.mockResolvedValue({ ok: true, json: () => Promise.resolve(networkResponse()) })
    mockAdminApi.checkCloudflareRanges.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ ranges: 23, source: 'cloudflare.com', checked_at: 1_800_000_000, checked: true, changed: true, next_check_at: 0 }),
    })
  })
  afterEach(() => {
    vi.useRealTimers()
    mockRoute.query.tab = 'config'
  })

  it('has a Backup tab with export and import', async () => {
    mockRoute.query.tab = 'backup'
    const wrapper = mountView()
    await flushPromises()
    const backup = wrapper.find('[data-testid="backup-settings"]')
    expect(backup.exists()).toBe(true)
    expect(backup.text()).toContain('Export settings')
    expect(backup.text()).toContain('Import settings')
  })

  it('shows the saved settings and how the server sees this request', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect((wrapper.find('[data-testid="trust-private"]').element as HTMLInputElement).checked).toBe(true)
    expect((cloudflareBox(wrapper).element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.find('[data-testid="network-detected"]').text()).toContain('162.158.1.1')
  })

  it('saves the structured settings', async () => {
    mockAdminApi.updateNetworkSettings.mockResolvedValue({ ok: true, json: () => Promise.resolve(networkResponse()) })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="trust-custom"]').setValue('203.0.113.10, 198.51.100.0/24')
    await wrapper.find('[data-testid="network-save"]').trigger('click')
    await flushPromises()
    expect(mockAdminApi.updateNetworkSettings).toHaveBeenCalledWith({
      trust_private: true,
      trust_cloudflare: false,
      custom: ['203.0.113.10', '198.51.100.0/24'],
    })
  })

  it('shows the server error for an invalid entry', async () => {
    mockAdminApi.updateNetworkSettings.mockResolvedValue({ ok: false, json: () => Promise.resolve({ error: '"0.0.0.0/0" would trust every address' }) })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="trust-custom"]').setValue('0.0.0.0/0')
    await wrapper.find('[data-testid="network-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('would trust every address')
  })

  it('is read-only when TRUSTED_PROXIES is set in the environment', async () => {
    mockAdminApi.getNetworkSettings.mockResolvedValue({ ok: true, json: () => Promise.resolve(networkResponse({ locked: true })) })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('TRUSTED_PROXIES')
    expect((cloudflareBox(wrapper).element as HTMLInputElement).disabled).toBe(true)
  })

  // Ticking/unticking in quick succession sends at most one check, once the
  // box has stayed ticked for the debounce window.
  it('debounces the Cloudflare range check', async () => {
    vi.useFakeTimers()
    const wrapper = mountView()
    await flushPromises()
    for (const on of [true, false, true, false, true]) {
      await cloudflareBox(wrapper).setValue(on)
      await vi.advanceTimersByTimeAsync(300)
    }
    expect(mockAdminApi.checkCloudflareRanges).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1500)
    await flushPromises()
    expect(mockAdminApi.checkCloudflareRanges).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="cloudflare-status"]').text()).toContain('updated just now')
  })

  it('does not check when the box ends up unticked', async () => {
    vi.useFakeTimers()
    const wrapper = mountView()
    await flushPromises()
    await cloudflareBox(wrapper).setValue(true)
    await vi.advanceTimersByTimeAsync(500)
    await cloudflareBox(wrapper).setValue(false)
    await vi.advanceTimersByTimeAsync(3000)
    expect(mockAdminApi.checkCloudflareRanges).not.toHaveBeenCalled()
  })
})
