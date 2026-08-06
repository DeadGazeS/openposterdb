import type { FreeKeyDefaults } from '@/lib/settings'

const BASE_URL = import.meta.env.VITE_API_URL || ''

// authFetch is the shared helper for every auth-endpoint request: joins
// BASE_URL, sets Content-Type when a body is present and the caller hasn't,
// and forwards cookies (so the refresh cookie round-trips). Returns the raw
// Response — callers parse the body (the auth endpoints return heterogeneous
// shapes).
async function authFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  return fetch(`${BASE_URL}${path}`, { ...init, headers, credentials: 'include' })
}

export const authApi = {
  status: (): Promise<Response> => authFetch('/api/auth/status'),
  freeKeySettings: (): Promise<Response> => authFetch('/api/free-key/settings'),
  setup: (username: string, password: string): Promise<Response> =>
    authFetch('/api/auth/setup', { method: 'POST', body: JSON.stringify({ username, password }) }),
  login: (username: string, password: string): Promise<Response> =>
    authFetch('/api/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  refresh: (): Promise<Response> => authFetch('/api/auth/refresh', { method: 'POST' }),
  keyLogin: (apiKey: string): Promise<Response> =>
    authFetch('/api/auth/key-login', { method: 'POST', body: JSON.stringify({ api_key: apiKey }) }),
}
