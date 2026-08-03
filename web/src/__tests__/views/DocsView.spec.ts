import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount, flushPromises } from '@vue/test-utils'

vi.mock('@scalar/api-reference', () => ({
  ApiReference: {
    name: 'ApiReference',
    template: '<div class="api-reference" />',
    props: ['configuration'],
  },
}))

vi.mock('@scalar/api-reference/style.css', () => ({}))

const mockAuthStore = {
  token: null as string | null,
  apiKeyToken: null as string | null,
}

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => mockAuthStore,
}))

import DocsView from '@/views/DocsView.vue'

const fakeSpec = { openapi: '3.1.0', info: { title: 'Test' } }

function mockFetch(ok = true) {
  return vi.fn().mockResolvedValue({
    ok,
    json: () => Promise.resolve(fakeSpec),
  })
}

describe('DocsView', () => {
  beforeEach(() => {
    mockAuthStore.token = null
    mockAuthStore.apiKeyToken = null
    vi.restoreAllMocks()
  })

  function mountView() {
    return shallowMount(DocsView, {
      global: {
        stubs: {
          'router-link': {
            template: '<a :href="to"><slot /></a>',
            props: ['to'],
          },
          ArrowLeft: { template: '<svg />' },
        },
      },
    })
  }

  it('renders topbar with OpenPosterDB text and API Reference subtitle', () => {
    vi.stubGlobal('fetch', mockFetch())
    const wrapper = mountView()
    expect(wrapper.text()).toContain('OpenPosterDB')
    expect(wrapper.text()).toContain('API Reference')
  })

  it('renders ApiReference component after spec loads', async () => {
    vi.stubGlobal('fetch', mockFetch())
    const wrapper = mountView()
    await flushPromises()

    const apiRef = wrapper.findComponent({ name: 'ApiReference' })
    expect(apiRef.exists()).toBe(true)
    expect(apiRef.props('configuration')).toEqual(
      expect.objectContaining({
        content: fakeSpec,
        hideClientButton: true,
      }),
    )
  })

  it('does not render ApiReference when spec fetch fails', async () => {
    vi.stubGlobal('fetch', mockFetch(false))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findComponent({ name: 'ApiReference' }).exists()).toBe(false)
  })

  it('includes Authorization header when admin token is set', async () => {
    mockAuthStore.token = 'admin-jwt'
    const fetchMock = mockFetch()
    vi.stubGlobal('fetch', fetchMock)
    mountView()
    await flushPromises()

    const [, options] = fetchMock.mock.calls[0]!
    expect(options.headers['Authorization']).toBe('Bearer admin-jwt')
  })

  it('includes Authorization header when API key token is set', async () => {
    mockAuthStore.apiKeyToken = 'key-jwt'
    const fetchMock = mockFetch()
    vi.stubGlobal('fetch', fetchMock)
    mountView()
    await flushPromises()

    const [, options] = fetchMock.mock.calls[0]!
    expect(options.headers['Authorization']).toBe('Bearer key-jwt')
  })

  it('omits Authorization header when unauthenticated', async () => {
    const fetchMock = mockFetch()
    vi.stubGlobal('fetch', fetchMock)
    mountView()
    await flushPromises()

    const [, options] = fetchMock.mock.calls[0]!
    expect(options.headers['Authorization']).toBeUndefined()
  })
})
