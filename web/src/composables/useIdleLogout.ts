import { onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const IDLE_TIMEOUT_MS = 3 * 60 * 1000 // 3 minutes
const CHECK_INTERVAL_MS = 15 * 1000 // how often the idle check runs

// useIdleLogout logs the admin user out and redirects to the login page when
// there has been no user interaction for the configured timeout.
//
// The listeners only stamp a timestamp — they never touch a timer — and a
// single periodic interval performs the actual check, so frequent activity
// (every click/keypress) can't churn the scheduler.
export function useIdleLogout() {
  const router = useRouter()
  const auth = useAuthStore()

  let lastActivity = Date.now()
  let interval: ReturnType<typeof setInterval> | null = null

  function onActivity() {
    lastActivity = Date.now()
  }

  function checkIdle() {
    if (Date.now() - lastActivity > IDLE_TIMEOUT_MS) {
      auth.logout()
      router.push({ name: 'login' })
    }
  }

  const events = ['click', 'keydown', 'touchstart'] as const

  onMounted(() => {
    for (const e of events) window.addEventListener(e, onActivity, { passive: true })
    interval = setInterval(checkIdle, CHECK_INTERVAL_MS)
  })

  onBeforeUnmount(() => {
    for (const e of events) window.removeEventListener(e, onActivity)
    if (interval) clearInterval(interval)
  })
}
