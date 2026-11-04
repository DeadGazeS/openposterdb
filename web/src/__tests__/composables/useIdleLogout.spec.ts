import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

import { useIdleLogout } from '@/composables/useIdleLogout'
import { useAuthStore } from '@/stores/auth'

const Host = defineComponent({
  setup() {
    useIdleLogout()
    return () => null
  },
})

describe('useIdleLogout', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    push.mockClear()
  })
  afterEach(() => vi.useRealTimers())

  // Reading a long page (scrolling only) counted as idle and logged the
  // admin out after three minutes.
  it('treats scrolling and pointer movement as activity', () => {
    const auth = useAuthStore()
    const logout = vi.spyOn(auth, 'logout')
    const w = mount(Host)
    for (let i = 0; i < 8; i++) {
      vi.advanceTimersByTime(30_000)
      document.dispatchEvent(new Event(i % 2 ? 'scroll' : 'pointermove'))
    }
    expect(logout).not.toHaveBeenCalled()
    vi.advanceTimersByTime(4 * 60_000)
    expect(logout).toHaveBeenCalled()
    w.unmount()
  })
})
