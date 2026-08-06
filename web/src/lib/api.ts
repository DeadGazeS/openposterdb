import { useAuthStore } from '@/stores/auth'
import type { PreviewKind, PreviewParams, SaveSettingsPayload, SourceColors } from '@/lib/settings'

const BASE_URL = import.meta.env.VITE_API_URL || ''

let _onAuthFailure: (() => void) | null = null

export function setOnAuthFailure(callback: () => void) {
  _onAuthFailure = callback
}

async function request(path: string, options: RequestInit = {}): Promise<Response> {
  const auth = useAuthStore()
  const headers = new Headers(options.headers)

  if (auth.token) {
    headers.set('Authorization', `Bearer ${auth.token}`)
  }

  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(`${BASE_URL}${path}`, { ...options, headers, credentials: 'include' })

  if (res.status === 401 && auth.token) {
    // Try refreshing the token
    const refreshed = await auth.refresh()
    if (refreshed) {
      const retryHeaders = new Headers(options.headers)
      retryHeaders.set('Authorization', `Bearer ${auth.token}`)
      if (options.body && !retryHeaders.has('Content-Type')) {
        retryHeaders.set('Content-Type', 'application/json')
      }
      return fetch(`${BASE_URL}${path}`, { ...options, headers: retryHeaders, credentials: 'include' })
    }
    // Refresh failed — clear credentials and redirect without retrying
    auth.logout()
    _onAuthFailure?.()
    return res
  }

  return res
}

export async function get(path: string): Promise<Response> {
  return request(path)
}

export async function post(path: string, body?: unknown): Promise<Response> {
  return request(path, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function put(path: string, body?: unknown): Promise<Response> {
  return request(path, {
    method: 'PUT',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function del(path: string): Promise<Response> {
  return request(path, { method: 'DELETE' })
}

/** Convert a params object to a query string (with leading "?" if any entries,
 *  empty string otherwise). Entries with undefined values are omitted (the
 *  caller means "don't set this param"); empty-string values are KEPT
 *  because URLSearchParams.set(key, '') emits `key=`, which is the
 *  convention for clearing a server-side default. Numbers are stringified.
 *  Shared by buildUrl (api client) and the FreeApiKeyCard preview-URL
 *  builder. */
export function toQueryParams(params: Record<string, string | number | undefined>): string {
  const qs = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) {
      qs.set(key, String(value))
    }
  }
  const query = qs.toString()
  return query ? `?${query}` : ''
}

/** Build a URL path with query parameters, omitting entries with nullish values. */
function buildUrl(path: string, params: Record<string, string | number | undefined>): string {
  return `${path}${toQueryParams(params)}`
}

// --- Typed API service layer ---

/** Purge scope: a whole logical title (all variants) or a single rendered variant. */
export type PurgeScope = 'title' | 'variant'

/**
 * Build a per-title / per-variant purge URL. For `title` scope `idValue` is the
 * bare title id; for `variant` scope it's the full cache value (one row's key).
 */
function purgeUrl(kind: string, idType: string, idValue: string, scope: PurgeScope): string {
  const path = `/api/admin/${kind}/${idType}/${encodeURIComponent(idValue)}`
  return scope === 'variant' ? `${path}?scope=variant` : path
}

/** Serialize non-default color overrides into a compact JSON query value. */
function colorsQuery(colors?: Record<string, SourceColors>): string | undefined {
  if (!colors) return undefined
  const compact: Record<string, SourceColors> = {}
  for (const [k, c] of Object.entries(colors)) {
    if (c.accent || c.value || c.border || c.text) compact[k] = c
  }
  return Object.keys(compact).length ? JSON.stringify(compact) : undefined
}

/** Map camelCase preview params to the snake_case query string. */
function previewQuery(p: PreviewParams): Record<string, string | number | undefined> {
  return {
    ratings_limit: p.ratingsLimit,
    ratings_order: p.ratingsOrder,
    ratings_exclude: p.ratingsExclude,
    badge_style: p.badgeStyle,
    label_style: p.labelStyle,
    badge_direction: p.badgeDirection,
    text_size: p.textSize,
    layout: p.layout,
    badge_shape: p.badgeShape,
    badge_alpha: p.badgeAlpha,
    fit: p.posterFit,
    badge_size: p.badgeSize,
    logo_size: p.logoSize,
    badge_width: p.badgeWidth,
    badge_height: p.badgeHeight,
    edge_inset_x: p.edgeInsetX,
    edge_inset_y: p.edgeInsetY,
    blur: p.blur ? 'true' : undefined,
    colors: colorsQuery(p.colors),
  }
}

export const adminApi = {
  getStats: (): Promise<Response> => get('/api/admin/stats'),
  purgeAll: (): Promise<Response> => post('/api/admin/cache/purge'),
  clearPosters: (): Promise<Response> => del('/api/admin/posters'),
  clearLogos: (): Promise<Response> => del('/api/admin/logos'),
  clearBackdrops: (): Promise<Response> => del('/api/admin/backdrops'),
  clearEpisodes: (): Promise<Response> => del('/api/admin/episodes'),
  getPosters: (page: number, pageSize: number): Promise<Response> =>
    get(`/api/admin/posters?page=${page}&page_size=${pageSize}`),
  getPosterImage: (key: string): Promise<Response> =>
    get(`/api/admin/posters/${key}/image`),
  getSettings: (): Promise<Response> => get('/api/admin/settings'),
  updateSettings: (settings: Partial<SaveSettingsPayload> & { image_source: string; free_api_key_enabled?: boolean }): Promise<Response> => put('/api/admin/settings', settings),
  getPrefs: (): Promise<Response> => get('/api/admin/prefs'),
  updatePrefs: (prefs: Record<string, string>): Promise<Response> => put('/api/admin/prefs', prefs),
  exportSettings: (includeServiceKeys: boolean, includeAPIKeys: boolean): Promise<Response> =>
    get(`/api/admin/settings/export?include_service_keys=${includeServiceKeys ? '1' : '0'}&include_api_keys=${includeAPIKeys ? '1' : '0'}`),
  importSettings: (payload: unknown): Promise<Response> =>
    post('/api/admin/settings/import', payload),
  getServiceKeys: (): Promise<Response> => get('/api/admin/settings/services'),
  updateServiceKeys: (keys: Record<string, string | null>): Promise<Response> => put('/api/admin/settings/services', keys),
  fetchPoster: (idType: string, idValue: string): Promise<Response> =>
    post(`/api/admin/posters/${idType}/${idValue}/fetch`),
  purgePoster: (idType: string, idValue: string, scope: PurgeScope = 'title'): Promise<Response> =>
    del(purgeUrl('posters', idType, idValue, scope)),
  getLogos: (page: number, pageSize: number): Promise<Response> =>
    get(`/api/admin/logos?page=${page}&page_size=${pageSize}`),
  getLogoImage: (key: string): Promise<Response> =>
    get(`/api/admin/logos/${key}`),
  fetchLogo: (idType: string, idValue: string): Promise<Response> =>
    post(`/api/admin/logos/${idType}/${idValue}/fetch`),
  purgeLogo: (idType: string, idValue: string, scope: PurgeScope = 'title'): Promise<Response> =>
    del(purgeUrl('logos', idType, idValue, scope)),
  getBackdrops: (page: number, pageSize: number): Promise<Response> =>
    get(`/api/admin/backdrops?page=${page}&page_size=${pageSize}`),
  getBackdropImage: (key: string): Promise<Response> =>
    get(`/api/admin/backdrops/${key}`),
  fetchBackdrop: (idType: string, idValue: string): Promise<Response> =>
    post(`/api/admin/backdrops/${idType}/${idValue}/fetch`),
  purgeBackdrop: (idType: string, idValue: string, scope: PurgeScope = 'title'): Promise<Response> =>
    del(purgeUrl('backdrops', idType, idValue, scope)),
  getEpisodes: (page: number, pageSize: number): Promise<Response> =>
    get(`/api/admin/episodes?page=${page}&page_size=${pageSize}`),
  getEpisodeImage: (key: string): Promise<Response> =>
    get(`/api/admin/episodes/${key}/image`),
  fetchEpisode: (idType: string, idValue: string): Promise<Response> =>
    post(`/api/admin/episodes/${idType}/${idValue}/fetch`),
  purgeEpisode: (idType: string, idValue: string, scope: PurgeScope = 'title'): Promise<Response> =>
    del(purgeUrl('episodes', idType, idValue, scope)),
  preview: (kind: PreviewKind, params: PreviewParams): Promise<Response> =>
    get(buildUrl(`/api/admin/preview/${kind}`, previewQuery(params))),
}

// --- Self-service API (API key session JWT auth) ---

const KEY_BASE_URL = import.meta.env.VITE_API_URL || ''

function keyRequest(path: string, options: RequestInit = {}): Promise<Response> {
  const auth = useAuthStore()
  const headers = new Headers(options.headers)

  if (auth.apiKeyToken) {
    headers.set('Authorization', `Bearer ${auth.apiKeyToken}`)
  }

  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  return fetch(`${KEY_BASE_URL}${path}`, { ...options, headers })
}

export const selfApi = {
  getInfo: (): Promise<Response> => keyRequest('/api/key/me'),
  getSettings: (): Promise<Response> => keyRequest('/api/key/me/settings'),
  updateSettings: (settings: SaveSettingsPayload): Promise<Response> =>
    keyRequest('/api/key/me/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),
  resetSettings: (): Promise<Response> =>
    keyRequest('/api/key/me/settings', { method: 'DELETE' }),
  preview: (kind: PreviewKind, params: PreviewParams): Promise<Response> =>
    keyRequest(buildUrl(`/api/key/me/preview/${kind}`, previewQuery(params))),
}

export const keysApi = {
  list: (): Promise<Response> => get('/api/keys'),
  create: (name: string): Promise<Response> => post('/api/keys', { name }),
  delete: (id: number): Promise<Response> => del(`/api/keys/${id}`),
  getSettings: (id: number): Promise<Response> => get(`/api/keys/${id}/settings`),
  updateSettings: (id: number, settings: SaveSettingsPayload): Promise<Response> => put(`/api/keys/${id}/settings`, settings),
  deleteSettings: (id: number): Promise<Response> => del(`/api/keys/${id}/settings`),
}
