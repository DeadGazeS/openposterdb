import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/lib/auth-api'
import type { FreeKeyDefaults } from '@/lib/settings'

export const useAuthStore = defineStore('auth', () => {
  // The admin access token lives in memory only (it used to sit in
  // localStorage, readable by any injected script). A reload restores the
  // session through the HttpOnly refresh cookie — see restoreSession.
  const token = ref<string | null>(null)
  try {
    localStorage.removeItem('token') // drop the copy older versions stored
  } catch {
    // storage unavailable
  }
  const setupRequired = ref<boolean | null>(null)
  const freeApiKeyEnabled = ref(false)
  const disablePublicPages = ref(false)

  // Global default render settings the free key serves with. Fetched lazily and
  // memoized so the card (rendered on both the landing and login pages) reads it
  // once per session; the backend serves it from cache, so no repeat DB reads.
  const freeKeyDefaults = ref<FreeKeyDefaults | null>(null)
  let freeKeyDefaultsPromise: Promise<void> | null = null

  async function loadFreeKeyDefaults(): Promise<void> {
    if (freeKeyDefaults.value) return
    if (freeKeyDefaultsPromise) return freeKeyDefaultsPromise
    freeKeyDefaultsPromise = (async () => {
      try {
        const res = await authApi.freeKeySettings()
        if (res.ok) {
          freeKeyDefaults.value = await res.json()
        }
      } catch {
        // Non-fatal: the card falls back to its built-in defaults.
      } finally {
        freeKeyDefaultsPromise = null
      }
    })()
    return freeKeyDefaultsPromise
  }

  // API key session state (sessionStorage so it clears on tab close)
  const apiKeyToken = ref<string | null>(sessionStorage.getItem('apiKeyToken'))
  const apiKeyInfo = ref<{ name: string; key_prefix: string } | null>(null)

  const isAdminSession = computed(() => !!token.value)
  const isApiKeySession = computed(() => !!apiKeyToken.value)
  const isAuthenticated = computed(() => isAdminSession.value || isApiKeySession.value)

  function setToken(t: string) {
    token.value = t
  }

  function clearToken() {
    token.value = null
  }

  // restoreSession runs once per page load: with no token in memory it asks
  // the server for one using the refresh cookie (if the browser has one).
  let sessionRestore: Promise<boolean> | null = null
  function restoreSession(): Promise<boolean> {
    if (!sessionRestore) {
      sessionRestore = token.value ? Promise.resolve(true) : refresh()
    }
    return sessionRestore
  }

  async function checkSetupRequired(): Promise<boolean> {
    if (setupRequired.value !== null) return setupRequired.value
    const res = await authApi.status()
    if (!res.ok) throw new Error(`status check failed: ${res.status}`)
    const data = await res.json()
    setupRequired.value = data.setup_required
    freeApiKeyEnabled.value = !!data.free_api_key_enabled
    disablePublicPages.value = !!data.disable_public_pages
    return data.setup_required
  }

  async function setup(username: string, password: string): Promise<boolean> {
    const res = await authApi.setup(username, password)
    if (!res.ok) return false
    const data = await res.json()
    setToken(data.token)
    setupRequired.value = false
    return true
  }

  async function login(username: string, password: string): Promise<boolean> {
    const res = await authApi.login(username, password)
    if (!res.ok) return false
    const data = await res.json()
    setToken(data.token)
    return true
  }

  async function loginWithApiKey(key: string): Promise<boolean> {
    const res = await authApi.keyLogin(key)
    if (!res.ok) return false
    const data = await res.json()
    apiKeyToken.value = data.token
    sessionStorage.setItem('apiKeyToken', data.token)
    apiKeyInfo.value = { name: data.name, key_prefix: data.key_prefix }
    return true
  }

  function logoutApiKey() {
    apiKeyToken.value = null
    apiKeyInfo.value = null
    sessionStorage.removeItem('apiKeyToken')
  }

  // One refresh at a time: several requests hitting 401 together each used to
  // start their own refresh with the same cookie; the server rotates it on
  // the first, so the others failed and logged the user out.
  let refreshInFlight: Promise<boolean> | null = null

  function refresh(): Promise<boolean> {
    if (!refreshInFlight) {
      refreshInFlight = (async () => {
        try {
          const res = await authApi.refresh()
          if (!res.ok) return false
          const data = await res.json()
          setToken(data.token)
          return true
        } catch {
          return false
        } finally {
          refreshInFlight = null
        }
      })()
    }
    return refreshInFlight
  }

  function logout() {
    // Revoke the refresh token server-side (it stayed valid for 7 days after
    // a local-only logout). Best effort: local state is cleared regardless.
    if (token.value) {
      authApi.logout().catch(() => {})
    }
    clearToken()
    logoutApiKey()
    setupRequired.value = null
  }

  return {
    token,
    apiKeyToken,
    apiKeyInfo,
    isAuthenticated,
    isAdminSession,
    isApiKeySession,
    setupRequired,
    freeApiKeyEnabled,
    disablePublicPages,
    freeKeyDefaults,
    loadFreeKeyDefaults,
    checkSetupRequired,
    setup,
    login,
    loginWithApiKey,
    logoutApiKey,
    refresh,
    restoreSession,
    logout,
  }
})
