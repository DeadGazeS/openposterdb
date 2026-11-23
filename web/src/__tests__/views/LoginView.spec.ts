import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import LoginView from '@/views/LoginView.vue'

const mockRouter = {
  push: vi.fn(),
  replace: vi.fn(),
}

const mockAuthStore = {
  checkSetupRequired: vi.fn().mockResolvedValue(false),
  login: vi.fn(),
  loginWithApiKey: vi.fn(),
  loadFreeKeyDefaults: vi.fn().mockResolvedValue(undefined),
  isAuthenticated: false,
  freeApiKeyEnabled: false,
  disablePublicPages: false,
  freeKeyDefaults: null,
}

vi.mock('vue-router', () => ({
  useRouter: () => mockRouter,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => mockAuthStore,
}))

function mountView() {
  return mount(LoginView, {
    global: {
      plugins: [createPinia()],
      stubs: {
        Button: {
          template: '<button><slot /></button>',
          props: ['disabled'],
        },
        'router-link': {
          template: '<a><slot /></a>',
          props: ['to'],
        },
      },
    },
  })
}

describe('LoginView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockAuthStore.checkSetupRequired.mockResolvedValue(false)
    mockAuthStore.login.mockReset()
    mockAuthStore.loginWithApiKey.mockReset()
    mockAuthStore.freeApiKeyEnabled = false
    mockAuthStore.disablePublicPages = false
  })

  it('renders admin login form by default', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('input#username').exists()).toBe(true)
    expect(wrapper.find('input#password').exists()).toBe(true)
    expect(wrapper.find('button[type="submit"]').exists()).toBe(true)
  })

  it('shows error message on failed admin login', async () => {
    mockAuthStore.login.mockResolvedValue({ ok: false, reason: 'unauthorized' })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('input#username').setValue('user')
    await wrapper.find('input#password').setValue('wrong')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Invalid username or password')
  })

  it('calls auth.login on admin form submit', async () => {
    mockAuthStore.login.mockResolvedValue({ ok: true })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('input#username').setValue('admin')
    await wrapper.find('input#password').setValue('secret')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockAuthStore.login).toHaveBeenCalledWith('admin', 'secret')
    expect(mockRouter.push).toHaveBeenCalledWith('/admin')
  })

  it('toggles to API key mode', async () => {
    const wrapper = mountView()
    await flushPromises()

    // Click the toggle link
    await wrapper.find('button.underline').trigger('click')
    await flushPromises()

    // Should show API key input, not username/password
    expect(wrapper.find('input#apikey').exists()).toBe(true)
    expect(wrapper.find('input#username').exists()).toBe(false)
    expect(wrapper.find('input#password').exists()).toBe(false)
    expect(wrapper.text()).toContain('Sign in as admin instead')
  })

  it('calls loginWithApiKey in apikey mode', async () => {
    mockAuthStore.loginWithApiKey.mockResolvedValue({ ok: true })
    const wrapper = mountView()
    await flushPromises()

    // Switch to API key mode
    await wrapper.find('button.underline').trigger('click')
    await flushPromises()

    await wrapper.find('input#apikey').setValue('my-secret-key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockAuthStore.loginWithApiKey).toHaveBeenCalledWith('my-secret-key')
    expect(mockRouter.push).toHaveBeenCalledWith('/individual-image-settings')
  })

  it('shows error on failed API key login', async () => {
    mockAuthStore.loginWithApiKey.mockResolvedValue({ ok: false, reason: 'unauthorized' })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button.underline').trigger('click')
    await flushPromises()

    await wrapper.find('input#apikey').setValue('bad-key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Invalid API key')
  })

  async function submitAdmin() {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('input#username').setValue('admin')
    await wrapper.find('input#password').setValue('secret')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    return wrapper
  }

  async function submitKey() {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('button.underline').trigger('click')
    await flushPromises()
    await wrapper.find('input#apikey').setValue('some-key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    return wrapper
  }

  // Only a 401 means wrong credentials; the other failures say what happened.
  it.each([
    [{ ok: false, reason: 'rate-limited', retryAfter: 42 }, 'Too many attempts. Try again in 42 s.'],
    [{ ok: false, reason: 'rate-limited' }, 'Too many attempts. Try again shortly.'],
    [{ ok: false, reason: 'busy', retryAfter: 5 }, 'The server is busy. Try again in 5 s.'],
    [{ ok: false, reason: 'server' }, 'Server error. Try again later.'],
    [{ ok: false, reason: 'network' }, 'Could not reach the server. Check your connection and try again.'],
  ])('admin login %o shows %s', async (result, message) => {
    mockAuthStore.login.mockResolvedValue(result)
    const wrapper = await submitAdmin()
    expect(wrapper.text()).toContain(message)
    expect(wrapper.text()).not.toContain('Invalid username or password')
    expect(mockRouter.push).not.toHaveBeenCalled()
  })

  it.each([
    [{ ok: false, reason: 'rate-limited', retryAfter: 7 }, 'Too many attempts. Try again in 7 s.'],
    [{ ok: false, reason: 'busy', retryAfter: 5 }, 'The server is busy. Try again in 5 s.'],
    [{ ok: false, reason: 'server' }, 'Server error. Try again later.'],
  ])('API key login %o shows %s', async (result, message) => {
    mockAuthStore.loginWithApiKey.mockResolvedValue(result)
    const wrapper = await submitKey()
    expect(wrapper.text()).toContain(message)
    expect(wrapper.text()).not.toContain('Invalid API key')
  })

  it('API key login 403 says the key is disabled', async () => {
    mockAuthStore.loginWithApiKey.mockResolvedValue({
      ok: false,
      reason: 'forbidden',
      message: 'Free API key is disabled',
    })
    const wrapper = await submitKey()
    expect(wrapper.text()).toContain('This API key is disabled.')
    expect(wrapper.text()).not.toContain('Invalid API key')
  })

  it('shows free API key card when freeApiKeyEnabled is true', async () => {
    mockAuthStore.freeApiKeyEnabled = true
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Free API Key Available')
    expect(wrapper.text()).toContain('t0-free-rpdb')
  })

  it('hides free API key card when freeApiKeyEnabled is false', async () => {
    mockAuthStore.freeApiKeyEnabled = false
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('Free API Key Available')
    expect(wrapper.text()).not.toContain('t0-free-rpdb')
  })

  it('shows back to home link when disablePublicPages is false', async () => {
    mockAuthStore.disablePublicPages = false
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Back to home')
  })

  it('hides back to home link when disablePublicPages is true', async () => {
    mockAuthStore.disablePublicPages = true
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('Back to home')
  })

  it('toggles back to admin mode from apikey mode', async () => {
    const wrapper = mountView()
    await flushPromises()

    // Toggle to apikey
    await wrapper.find('button.underline').trigger('click')
    await flushPromises()
    expect(wrapper.find('input#apikey').exists()).toBe(true)

    // Toggle back to admin
    await wrapper.find('button.underline').trigger('click')
    await flushPromises()
    expect(wrapper.find('input#username').exists()).toBe(true)
    expect(wrapper.find('input#apikey').exists()).toBe(false)
    expect(wrapper.text()).toContain('Sign in with API key instead')
  })
})
