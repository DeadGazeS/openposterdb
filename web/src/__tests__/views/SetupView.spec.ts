import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SetupView from '@/views/SetupView.vue'

const mockRouter = {
  push: vi.fn(),
}

const mockAuthStore = {
  setup: vi.fn(),
}

vi.mock('vue-router', () => ({
  useRouter: () => mockRouter,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => mockAuthStore,
}))

function mountView() {
  return mount(SetupView, {
    global: {
      plugins: [createPinia()],
      stubs: {
        Button: {
          template: '<button><slot /></button>',
          props: ['disabled'],
        },
      },
    },
  })
}

describe('SetupView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockAuthStore.setup.mockReset()
  })

  it('renders setup form with confirm password field', () => {
    const wrapper = mountView()

    expect(wrapper.find('input#username').exists()).toBe(true)
    expect(wrapper.find('input#password').exists()).toBe(true)
    expect(wrapper.find('input#confirm-password').exists()).toBe(true)
    expect(wrapper.find('button[type="submit"]').exists()).toBe(true)
  })

  it('shows error when passwords do not match', async () => {
    const wrapper = mountView()

    await wrapper.find('input#username').setValue('admin')
    await wrapper.find('input#password').setValue('password123')
    await wrapper.find('input#confirm-password').setValue('different')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Passwords do not match')
    expect(mockAuthStore.setup).not.toHaveBeenCalled()
  })

  it('shows error when password is less than 8 characters', async () => {
    const wrapper = mountView()

    await wrapper.find('input#username').setValue('admin')
    await wrapper.find('input#password').setValue('short')
    await wrapper.find('input#confirm-password').setValue('short')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Password must be at least 8 characters')
    expect(mockAuthStore.setup).not.toHaveBeenCalled()
  })

  it('calls auth.setup on valid submit', async () => {
    mockAuthStore.setup.mockResolvedValue({ ok: true })
    const wrapper = mountView()

    await wrapper.find('input#username').setValue('admin')
    await wrapper.find('input#password').setValue('password123')
    await wrapper.find('input#confirm-password').setValue('password123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockAuthStore.setup).toHaveBeenCalledWith('admin', 'password123')
    expect(mockRouter.push).toHaveBeenCalledWith('/admin')
  })

  async function submitValid() {
    const wrapper = mountView()
    await wrapper.find('input#username').setValue('admin')
    await wrapper.find('input#password').setValue('password123')
    await wrapper.find('input#confirm-password').setValue('password123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    return wrapper
  }

  it('400 shows the server validation message', async () => {
    const msg = 'Invalid username: must be 1-128 characters and not contain whitespace/control characters'
    mockAuthStore.setup.mockResolvedValue({ ok: false, reason: 'invalid', message: msg })
    const wrapper = await submitValid()
    expect(wrapper.text()).toContain(msg)
    expect(wrapper.text()).not.toContain('already')
    expect(mockRouter.push).not.toHaveBeenCalled()
  })

  it('400 without a message falls back to a generic validation error', async () => {
    mockAuthStore.setup.mockResolvedValue({ ok: false, reason: 'invalid' })
    const wrapper = await submitValid()
    expect(wrapper.text()).toContain('Invalid username or password.')
  })

  it('403 says setup is already done', async () => {
    mockAuthStore.setup.mockResolvedValue({ ok: false, reason: 'forbidden', message: 'Setup already completed' })
    const wrapper = await submitValid()
    expect(wrapper.text()).toContain('Setup is already complete: an admin account exists. Sign in instead.')
  })

  it.each([
    [{ ok: false, reason: 'rate-limited', retryAfter: 30 }, 'Too many attempts. Try again in 30 s.'],
    [{ ok: false, reason: 'busy', retryAfter: 5 }, 'The server is busy. Try again in 5 s.'],
    [{ ok: false, reason: 'server' }, 'Server error. Try again later.'],
    [{ ok: false, reason: 'network' }, 'Could not reach the server. Check your connection and try again.'],
  ])('%o shows %s and not "already set up"', async (result, message) => {
    mockAuthStore.setup.mockResolvedValue(result)
    const wrapper = await submitValid()
    expect(wrapper.text()).toContain(message)
    expect(wrapper.text()).not.toContain('already')
  })
})
