import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'

const localStorageMock: Record<string, string> = {}
const localStorageStub = {
  getItem: vi.fn((key: string) => localStorageMock[key] ?? null),
  setItem: vi.fn((key: string, value: string) => {
    localStorageMock[key] = value
  }),
  removeItem: vi.fn((key: string) => {
    delete localStorageMock[key]
  }),
}

const sessionStorageMock: Record<string, string> = {}
const sessionStorageStub = {
  getItem: vi.fn((key: string) => sessionStorageMock[key] ?? null),
  setItem: vi.fn((key: string, value: string) => {
    sessionStorageMock[key] = value
  }),
  removeItem: vi.fn((key: string) => {
    delete sessionStorageMock[key]
  }),
}

vi.stubGlobal('localStorage', localStorageStub)
vi.stubGlobal('sessionStorage', sessionStorageStub)

function mockFetchSuccess(data: Record<string, unknown>, ok = true) {
  return vi.fn().mockResolvedValue({
    ok,
    status: ok ? 200 : 401,
    headers: new Headers(),
    json: () => Promise.resolve(data),
  })
}

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.keys(localStorageMock).forEach((k) => delete localStorageMock[k])
    Object.keys(sessionStorageMock).forEach((k) => delete sessionStorageMock[k])
    vi.restoreAllMocks()
    vi.stubGlobal('localStorage', localStorageStub)
    vi.stubGlobal('sessionStorage', sessionStorageStub)
  })

  // The admin token stays in memory only (localStorage is readable by any
  // injected script).
  it('token is set via login and kept out of localStorage', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({ token: 'abc123' }))
    const auth = useAuthStore()
    await auth.login('user', 'pass')
    expect(auth.token).toBe('abc123')
    expect(localStorageStub.setItem).not.toHaveBeenCalledWith('token', expect.anything())
  })

  it('logout clears token from localStorage and sets null', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({ token: 'abc123' }))
    const auth = useAuthStore()
    await auth.login('user', 'pass')
    auth.logout()
    expect(auth.token).toBeNull()
    expect(localStorageStub.removeItem).toHaveBeenCalledWith('token')
  })

  it('isAuthenticated is true when token set', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({ token: 'abc123' }))
    const auth = useAuthStore()
    await auth.login('user', 'pass')
    expect(auth.isAuthenticated).toBe(true)
  })

  it('isAuthenticated is false when token is null', () => {
    const auth = useAuthStore()
    expect(auth.isAuthenticated).toBe(false)
  })

  it('login returns true on success', async () => {
    const fetchMock = mockFetchSuccess({ token: 'new-token' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    const result = await auth.login('user', 'pass')

    expect(result).toEqual({ ok: true })
    expect(auth.token).toBe('new-token')
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/auth/login'),
      expect.objectContaining({ method: 'POST' }),
    )
  })

  it('login returns false on failure', async () => {
    const fetchMock = mockFetchSuccess({}, false)
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    const result = await auth.login('user', 'wrong')

    expect(result).toEqual({ ok: false, reason: 'unauthorized' })
    expect(auth.token).toBeNull()
  })

  it('setup sets token and setupRequired=false', async () => {
    const fetchMock = mockFetchSuccess({ token: 'setup-token' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    const result = await auth.setup('admin', 'password123')

    expect(result).toEqual({ ok: true })
    expect(auth.token).toBe('setup-token')
    expect(auth.setupRequired).toBe(false)
  })

  it('refresh updates token', async () => {
    const fetchMock = mockFetchSuccess({ token: 'refreshed-token' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    const result = await auth.refresh()

    expect(result).toBe('refreshed')
    expect(auth.token).toBe('refreshed-token')
  })

  it('logout clears token and sets setupRequired to null', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({ token: 'abc' }))
    const auth = useAuthStore()
    await auth.login('user', 'pass')
    auth.logout()
    expect(auth.token).toBeNull()
    expect(auth.setupRequired).toBeNull()
  })

  it('checkSetupRequired caches result on second call', async () => {
    const fetchMock = mockFetchSuccess({ setup_required: true })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    const first = await auth.checkSetupRequired()
    const second = await auth.checkSetupRequired()

    expect(first).toBe(true)
    expect(second).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('refresh returns false on failure', async () => {
    const fetchMock = mockFetchSuccess({}, false)
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    const result = await auth.refresh()

    expect(result).toBe('unauthorized')
    expect(auth.token).toBeNull()
  })

  it('setup returns false on failure', async () => {
    const fetchMock = mockFetchSuccess({}, false)
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    const result = await auth.setup('admin', 'password123')

    expect(result.ok).toBe(false)
    expect(auth.token).toBeNull()
  })

  it('checkSetupRequired throws on network error', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.resolve({}),
    })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await expect(auth.checkSetupRequired()).rejects.toThrow('status check failed')
  })

  it('login sends credentials include', async () => {
    const fetchMock = mockFetchSuccess({ token: 'tok' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.login('user', 'pass')

    const [, options] = fetchMock.mock.calls[0]!
    expect(options.credentials).toBe('include')
  })

  it('setup sends credentials include', async () => {
    const fetchMock = mockFetchSuccess({ token: 'tok' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.setup('user', 'password123')

    const [, options] = fetchMock.mock.calls[0]!
    expect(options.credentials).toBe('include')
  })

  it('refresh sends credentials include', async () => {
    const fetchMock = mockFetchSuccess({ token: 'tok' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.refresh()

    const [, options] = fetchMock.mock.calls[0]!
    expect(options.credentials).toBe('include')
  })

  it('setup failure does not change setupRequired', async () => {
    const fetchMock = mockFetchSuccess({}, false)
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.setup('admin', 'password123')

    // setupRequired should still be null (not changed on failure)
    expect(auth.setupRequired).toBeNull()
  })

  it('ignores and removes a token older versions left in localStorage', () => {
    localStorageMock['token'] = 'persisted-token'
    const auth = useAuthStore()
    expect(auth.token).toBeNull()
    expect(localStorageMock['token']).toBeUndefined()
  })

  it('restoreSession gets a token from the refresh cookie once per load', async () => {
    const fetchMock = mockFetchSuccess({ token: 'restored' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()
    expect(await auth.restoreSession()).toBe(true)
    expect(await auth.restoreSession()).toBe(true)
    expect(auth.token).toBe('restored')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/auth/refresh'), expect.anything())
  })

  it('isAdminSession is true only with admin token', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({ token: 'abc123' }))
    const auth = useAuthStore()
    await auth.login('user', 'pass')
    expect(auth.isAdminSession).toBe(true)
    expect(auth.isApiKeySession).toBe(false)
  })

  it('isApiKeySession is true only with API key token', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt-token', name: 'test-key', key_prefix: 'abc' }),
    )
    const auth = useAuthStore()
    await auth.loginWithApiKey('raw-key')
    expect(auth.isApiKeySession).toBe(true)
    expect(auth.isAdminSession).toBe(false)
  })

  it('loginWithApiKey keeps the JWT in memory only', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt-token', name: 'my-key', key_prefix: 'xyz' }),
    )
    const auth = useAuthStore()
    const ok = await auth.loginWithApiKey('raw-key')
    expect(ok).toEqual({ ok: true })
    expect(auth.apiKeyToken).toBe('jwt-token')
    expect(sessionStorageStub.setItem).not.toHaveBeenCalledWith('apiKeyToken', expect.anything())
    expect(auth.apiKeyInfo).toEqual({ name: 'my-key', key_prefix: 'xyz' })
  })

  it('loginWithApiKey returns false on failure', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({}, false))
    const auth = useAuthStore()
    const ok = await auth.loginWithApiKey('bad-key')
    expect(ok).toEqual({ ok: false, reason: 'unauthorized' })
    expect(auth.apiKeyToken).toBeNull()
  })

  it('logoutApiKey clears API key session', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt-token', name: 'k', key_prefix: 'x' }),
    )
    const auth = useAuthStore()
    await auth.loginWithApiKey('key')
    auth.logoutApiKey()
    expect(auth.apiKeyToken).toBeNull()
    expect(auth.apiKeyInfo).toBeNull()
    expect(auth.isApiKeySession).toBe(false)
  })

  it('isAuthenticated is true for either session type', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt', name: 'k', key_prefix: 'x' }),
    )
    const auth = useAuthStore()
    expect(auth.isAuthenticated).toBe(false)
    await auth.loginWithApiKey('key')
    expect(auth.isAuthenticated).toBe(true)
  })

  it('logout clears both admin and API key sessions', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({ token: 'tok' }))
    const auth = useAuthStore()
    await auth.login('user', 'pass')
    auth.logout()
    expect(auth.isAdminSession).toBe(false)
    expect(auth.isApiKeySession).toBe(false)
    expect(auth.isAuthenticated).toBe(false)
  })

  // The key-session JWT is no longer persisted (injected script could read
  // it); a copy stored by an older version is ignored and removed.
  it('ignores and removes an apiKeyToken persisted by older versions', () => {
    sessionStorageMock['apiKeyToken'] = 'persisted-jwt'
    const auth = useAuthStore()
    expect(auth.apiKeyToken).toBeNull()
    expect(sessionStorageStub.removeItem).toHaveBeenCalledWith('apiKeyToken')
  })

  it('freeApiKeyEnabled defaults to false', () => {
    const auth = useAuthStore()
    expect(auth.freeApiKeyEnabled).toBe(false)
  })

  it('checkSetupRequired populates freeApiKeyEnabled from response', async () => {
    const fetchMock = mockFetchSuccess({ setup_required: false, free_api_key_enabled: true })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.checkSetupRequired()

    expect(auth.freeApiKeyEnabled).toBe(true)
  })

  it('checkSetupRequired sets freeApiKeyEnabled false when not in response', async () => {
    const fetchMock = mockFetchSuccess({ setup_required: false })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.checkSetupRequired()

    expect(auth.freeApiKeyEnabled).toBe(false)
  })

  it('disablePublicPages defaults to false', () => {
    const auth = useAuthStore()
    expect(auth.disablePublicPages).toBe(false)
  })

  it('checkSetupRequired populates disablePublicPages from response', async () => {
    const fetchMock = mockFetchSuccess({ setup_required: false, disable_public_pages: true })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.checkSetupRequired()

    expect(auth.disablePublicPages).toBe(true)
  })

  it('checkSetupRequired sets disablePublicPages false when not in response', async () => {
    const fetchMock = mockFetchSuccess({ setup_required: false })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.checkSetupRequired()

    expect(auth.disablePublicPages).toBe(false)
  })
})

describe('auth store - API key session', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.keys(localStorageMock).forEach((k) => delete localStorageMock[k])
    Object.keys(sessionStorageMock).forEach((k) => delete sessionStorageMock[k])
    vi.restoreAllMocks()
    vi.stubGlobal('localStorage', localStorageStub)
    vi.stubGlobal('sessionStorage', sessionStorageStub)
  })

  it('loginWithApiKey stores the JWT (in memory) on success', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt-session-token', name: 'my-key', key_prefix: 'abcd1234' }),
    )
    const auth = useAuthStore()

    const result = await auth.loginWithApiKey('the-full-api-key')

    expect(result).toEqual({ ok: true })
    expect(auth.apiKeyToken).toBe('jwt-session-token')
    expect(sessionStorageStub.setItem).not.toHaveBeenCalled()
    expect(auth.apiKeyInfo).toEqual({ name: 'my-key', key_prefix: 'abcd1234' })
  })

  it('loginWithApiKey returns false on failure', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({}, false))
    const auth = useAuthStore()

    const result = await auth.loginWithApiKey('bad-key')

    expect(result).toEqual({ ok: false, reason: 'unauthorized' })
    expect(auth.apiKeyToken).toBeNull()
    expect(auth.apiKeyInfo).toBeNull()
  })

  it('isApiKeySession is true when apiKeyToken is set', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt-tok', name: 'k', key_prefix: 'ab' }),
    )
    const auth = useAuthStore()
    await auth.loginWithApiKey('key123')

    expect(auth.isApiKeySession).toBe(true)
    expect(auth.isAdminSession).toBe(false)
    expect(auth.isAuthenticated).toBe(true)
  })

  it('isAdminSession is true when JWT token is set', async () => {
    vi.stubGlobal('fetch', mockFetchSuccess({ token: 'jwt-tok' }))
    const auth = useAuthStore()
    await auth.login('user', 'pass')

    expect(auth.isAdminSession).toBe(true)
    expect(auth.isApiKeySession).toBe(false)
    expect(auth.isAuthenticated).toBe(true)
  })

  it('logoutApiKey clears API key state', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt-tok', name: 'k', key_prefix: 'ab' }),
    )
    const auth = useAuthStore()
    await auth.loginWithApiKey('key123')

    auth.logoutApiKey()

    expect(auth.apiKeyToken).toBeNull()
    expect(auth.apiKeyInfo).toBeNull()
    expect(auth.isApiKeySession).toBe(false)
    expect(auth.isAuthenticated).toBe(false)
    expect(sessionStorageStub.removeItem).toHaveBeenCalledWith('apiKeyToken')
  })

  it('logout clears both admin and API key state', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchSuccess({ token: 'jwt-tok', name: 'k', key_prefix: 'ab' }),
    )
    const auth = useAuthStore()
    await auth.loginWithApiKey('key123')

    auth.logout()

    expect(auth.apiKeyToken).toBeNull()
    expect(auth.token).toBeNull()
    expect(auth.isAuthenticated).toBe(false)
  })

  it('loginWithApiKey sends correct endpoint', async () => {
    const fetchMock = mockFetchSuccess({ name: 'k', key_prefix: 'ab' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()

    await auth.loginWithApiKey('my-key')

    const [url, options] = fetchMock.mock.calls[0]!
    expect(url).toContain('/api/auth/key-login')
    expect(options.method).toBe('POST')
    expect(JSON.parse(options.body)).toEqual({ api_key: 'my-key' })
  })

  // Concurrent 401s share one refresh (each used to send its own; the server
  // rotated on the first and the rest logged the user out).
  it('concurrent refresh calls share one request', async () => {
    let resolve: (v: unknown) => void = () => {}
    const fetchMock = vi.fn().mockImplementation(() => new Promise((r) => { resolve = r }))
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()
    const calls = [auth.refresh(), auth.refresh(), auth.refresh()]
    resolve({ ok: true, status: 200, headers: new Headers(), json: () => Promise.resolve({ token: 'new-token' }) })
    expect(await Promise.all(calls)).toEqual(['refreshed', 'refreshed', 'refreshed'])
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(auth.token).toBe('new-token')
    // a later refresh starts a new request
    fetchMock.mockResolvedValue({ ok: true, status: 200, headers: new Headers(), json: () => Promise.resolve({ token: 't2' }) })
    await auth.refresh()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  // Logout revokes the refresh token server-side via the cookie-path endpoint.
  it('logout calls the server logout endpoint', async () => {
    const fetchMock = mockFetchSuccess({ token: 'tok' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()
    await auth.login('admin', 'pw')
    fetchMock.mockClear()
    auth.logout()
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/auth/refresh/logout'),
      expect.objectContaining({ method: 'POST', credentials: 'include' }),
    )
    expect(auth.token).toBeNull()
  })
})

function mockFetchStatus(
  status: number,
  body: Record<string, unknown> = {},
  headers: Record<string, string> = {},
) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(headers),
    json: () => Promise.resolve(body),
  })
}

// Login, key login and setup tell wrong credentials apart from rate limits,
// a busy server, server errors and network failures, so the views can say so.
describe('auth store - failure results', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
    vi.stubGlobal('localStorage', localStorageStub)
    vi.stubGlobal('sessionStorage', sessionStorageStub)
  })

  it('login: 429 is rate-limited with Retry-After seconds', async () => {
    vi.stubGlobal('fetch', mockFetchStatus(429, { error: 'rate limit exceeded' }, { 'Retry-After': '42' }))
    const auth = useAuthStore()
    expect(await auth.login('u', 'p')).toEqual({ ok: false, reason: 'rate-limited', retryAfter: 42 })
  })

  it('login: 503 is busy with Retry-After seconds', async () => {
    vi.stubGlobal('fetch', mockFetchStatus(503, { error: 'server busy, retry shortly' }, { 'Retry-After': '5' }))
    const auth = useAuthStore()
    expect(await auth.login('u', 'p')).toEqual({ ok: false, reason: 'busy', retryAfter: 5 })
  })

  it('login: 503 without a usable Retry-After has no retryAfter', async () => {
    vi.stubGlobal('fetch', mockFetchStatus(503, {}, { 'Retry-After': 'soon' }))
    const auth = useAuthStore()
    expect(await auth.login('u', 'p')).toEqual({ ok: false, reason: 'busy' })
  })

  it('login: 500 is a server error', async () => {
    vi.stubGlobal('fetch', mockFetchStatus(500, { error: 'Authentication failed' }))
    const auth = useAuthStore()
    expect(await auth.login('u', 'p')).toEqual({ ok: false, reason: 'server' })
  })

  it('login: a network failure is reported, not thrown', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    const auth = useAuthStore()
    expect(await auth.login('u', 'p')).toEqual({ ok: false, reason: 'network' })
    expect(auth.token).toBeNull()
  })

  it('key login: 403 is forbidden with the server message', async () => {
    vi.stubGlobal('fetch', mockFetchStatus(403, { error: 'Free API key is disabled' }))
    const auth = useAuthStore()
    expect(await auth.loginWithApiKey('t0-free-rpdb')).toEqual({
      ok: false,
      reason: 'forbidden',
      message: 'Free API key is disabled',
    })
    expect(auth.apiKeyToken).toBeNull()
  })

  it('setup: 400 is invalid with the validation message', async () => {
    const msg = 'Invalid username: must be 1-128 characters and not contain whitespace/control characters'
    vi.stubGlobal('fetch', mockFetchStatus(400, { error: msg }))
    const auth = useAuthStore()
    expect(await auth.setup('a b', 'password123')).toEqual({ ok: false, reason: 'invalid', message: msg })
    expect(auth.setupRequired).toBeNull()
  })

  it('setup: 403 means setup was already completed', async () => {
    vi.stubGlobal('fetch', mockFetchStatus(403, { error: 'Setup already completed' }))
    const auth = useAuthStore()
    expect(await auth.setup('admin', 'password123')).toEqual({
      ok: false,
      reason: 'forbidden',
      message: 'Setup already completed',
    })
  })

  it('setup: 429 is rate-limited', async () => {
    vi.stubGlobal('fetch', mockFetchStatus(429, {}, { 'Retry-After': '10' }))
    const auth = useAuthStore()
    expect(await auth.setup('admin', 'password123')).toEqual({
      ok: false,
      reason: 'rate-limited',
      retryAfter: 10,
    })
  })
})

// Only a 401 from the refresh endpoint ends the session; a 500, 503, 429 or a
// network error leaves the refresh cookie alone and is retried later.
describe('auth store - refresh outcomes', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
    vi.stubGlobal('localStorage', localStorageStub)
    vi.stubGlobal('sessionStorage', sessionStorageStub)
  })

  it.each([500, 503, 429])('refresh: %i is unavailable, not unauthorized', async (status) => {
    vi.stubGlobal('fetch', mockFetchStatus(status))
    const auth = useAuthStore()
    expect(await auth.refresh()).toBe('unavailable')
  })

  it('refresh: a network error is unavailable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    const auth = useAuthStore()
    expect(await auth.refresh()).toBe('unavailable')
  })

  it('restoreSession retries on the next navigation after a 503', async () => {
    const fetchMock = mockFetchStatus(503, {}, { 'Retry-After': '5' })
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()
    expect(await auth.restoreSession()).toBe(false)
    fetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers(),
      json: () => Promise.resolve({ token: 'back' }),
    })
    expect(await auth.restoreSession()).toBe(true)
    expect(auth.token).toBe('back')
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('restoreSession retries after a network error and never calls logout', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()
    expect(await auth.restoreSession()).toBe(false)
    expect(await auth.restoreSession()).toBe(false)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    for (const [url] of fetchMock.mock.calls) {
      expect(url).not.toContain('/logout')
    }
  })

  it('restoreSession settles a 401 for the page load', async () => {
    const fetchMock = mockFetchStatus(401)
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()
    expect(await auth.restoreSession()).toBe(false)
    expect(await auth.restoreSession()).toBe(false)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
