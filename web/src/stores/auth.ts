import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/lib/auth-api'
import type { FreeKeyDefaults } from '@/lib/settings'
import { authFailure, type AuthResult, type RefreshResult } from '@/lib/auth-result'

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

  // API key session state: memory only, like the admin token (sessionStorage
  // was readable by any injected script). There is no refresh cookie for key
  // sessions, so a reload asks for the key again.
  const apiKeyToken = ref<string | null>(null)
  try {
    sessionStorage.removeItem('apiKeyToken') // drop the copy older versions stored
  } catch {
    // storage unavailable
  }
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
  // Only a definite answer (a token or a 401) settles it; after a server
  // error, busy server or network error the next navigation asks again, and
  // the refresh cookie is left alone.
  let sessionRestore: Promise<boolean> | null = null
  function restoreSession(): Promise<boolean> {
    if (!sessionRestore) {
      if (token.value) {
        sessionRestore = Promise.resolve(true)
      } else {
        const attempt = refresh().then((outcome) => {
          if (outcome === 'unavailable' && sessionRestore === attempt) sessionRestore = null
          return outcome === 'refreshed'
        })
        sessionRestore = attempt
      }
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

  // send runs a credential request: a network failure becomes a 'network'
  // result, a non-ok response is classified by status (see authFailure).
  async function send(
    request: () => Promise<Response>,
  ): Promise<{ res: Response; failure?: undefined } | { res?: undefined; failure: AuthResult }> {
    let res: Response
    try {
      res = await request()
    } catch {
      return { failure: { ok: false, reason: 'network' } }
    }
    return res.ok ? { res } : { failure: await authFailure(res) }
  }

  async function setup(username: string, password: string): Promise<AuthResult> {
    const { res, failure } = await send(() => authApi.setup(username, password))
    if (!res) return failure
    const data = await res.json()
    setToken(data.token)
    setupRequired.value = false
    return { ok: true }
  }

  async function login(username: string, password: string): Promise<AuthResult> {
    const { res, failure } = await send(() => authApi.login(username, password))
    if (!res) return failure
    const data = await res.json()
    setToken(data.token)
    return { ok: true }
  }

  async function loginWithApiKey(key: string): Promise<AuthResult> {
    const { res, failure } = await send(() => authApi.keyLogin(key))
    if (!res) return failure
    const data = await res.json()
    apiKeyToken.value = data.token
    apiKeyInfo.value = { name: data.name, key_prefix: data.key_prefix }
    return { ok: true }
  }

  function logoutApiKey() {
    apiKeyToken.value = null
    apiKeyInfo.value = null
  }

  // One refresh at a time: several requests hitting 401 together each used to
  // start their own refresh with the same cookie; the server rotates it on
  // the first, so the others failed and logged the user out.
  // Only a 401 means the refresh cookie is gone or invalid; any other failure
  // (500, 503, 429, network) is 'unavailable' and must not end the session.
  let refreshInFlight: Promise<RefreshResult> | null = null

  function refresh(): Promise<RefreshResult> {
    if (!refreshInFlight) {
      refreshInFlight = (async (): Promise<RefreshResult> => {
        try {
          const res = await authApi.refresh()
          if (res.status === 401) return 'unauthorized'
          if (!res.ok) return 'unavailable'
          const data = await res.json()
          setToken(data.token)
          return 'refreshed'
        } catch {
          return 'unavailable'
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
