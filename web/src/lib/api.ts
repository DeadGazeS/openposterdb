import { useAuthStore } from '@/stores/auth'
import type { ImageLayout } from '@/lib/layout'

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

/** Per-source color overrides. Empty string means "use the default". Accent and
 * value are the badge section colors (default black); border and text round out
 * the badge. The badge alpha slider controls shared background opacity. */
export interface SourceColors {
  accent?: string
  value?: string
  border?: string
  text?: string
}

export interface SaveSettingsPayload {
  image_source: string
  lang: string
  textless: boolean
  ratings_limit?: number
  ratings_order: string
  ratings_exclude: string
  poster_layout: ImageLayout
  logo_ratings_limit?: number
  backdrop_ratings_limit?: number
  poster_badge_style: string
  logo_badge_style: string
  backdrop_badge_style: string
  poster_label_style: string
  logo_label_style: string
  backdrop_label_style: string
  // Direction fields are optional: when omitted the backend keeps the stored value.
  poster_badge_direction?: string
  poster_fit: string
  poster_text_size: number
  logo_text_size: number
  backdrop_text_size: number
  poster_badge_size?: number
  poster_badge_width?: number
  poster_badge_height?: number
  logo_badge_size?: number
  logo_badge_width?: number
  logo_badge_height?: number
  backdrop_badge_size?: number
  backdrop_badge_width?: number
  backdrop_badge_height?: number
  poster_logo_size: number
  logo_logo_size: number
  backdrop_logo_size: number
  logo_layout: ImageLayout
  backdrop_layout: ImageLayout
  backdrop_badge_direction?: string
  backdrop_edge_inset_x: number
  backdrop_edge_inset_y: number
  episode_ratings_limit?: number
  episode_badge_style: string
  episode_label_style: string
  episode_text_size: number
  episode_badge_size?: number
  episode_badge_width?: number
  episode_badge_height?: number
  episode_logo_size: number
  episode_layout: ImageLayout
  episode_badge_direction?: string
  episode_blur: boolean
  poster_badge_shape: string
  logo_badge_shape: string
  backdrop_badge_shape: string
  episode_badge_shape: string
  poster_badge_alpha: number
  logo_badge_alpha: number
  backdrop_badge_alpha: number
  episode_badge_alpha: number
  colors?: Record<string, SourceColors>
}

/** Build a URL path with query parameters, omitting entries with nullish values. */
function buildUrl(path: string, params: Record<string, string | number | undefined>): string {
  const qs = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') {
      qs.set(key, String(value))
    }
  }
  const query = qs.toString()
  return query ? `${path}?${query}` : path
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
  previewPoster: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, badgeDirection?: string, textSize?: number, ratingsExclude?: string, layout?: string, badgeShape?: string, badgeAlpha?: number, posterFit?: string, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    get(buildUrl('/api/admin/preview/poster', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, badge_direction: badgeDirection, text_size: textSize, layout: layout, badge_shape: badgeShape, badge_alpha: badgeAlpha, fit: posterFit, badge_size: badgeSize, logo_size: logoSize, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
  previewLogo: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, badgeSize?: number, logoSize?: number, layout?: string, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    get(buildUrl('/api/admin/preview/logo', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, text_size: textSize, badge_shape: badgeShape, badge_alpha: badgeAlpha, badge_size: badgeSize, logo_size: logoSize, layout: layout, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
  previewBackdrop: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, badgeDirection?: string, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, edgeInsetX?: number, edgeInsetY?: number, layout?: string, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    get(buildUrl('/api/admin/preview/backdrop', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, text_size: textSize, badge_direction: badgeDirection, badge_shape: badgeShape, badge_alpha: badgeAlpha, edge_inset_x: edgeInsetX, edge_inset_y: edgeInsetY, layout: layout, badge_size: badgeSize, logo_size: logoSize, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
  previewEpisode: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, badgeDirection?: string, blur?: boolean, layout?: string, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    get(buildUrl('/api/admin/preview/episode', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, text_size: textSize, badge_direction: badgeDirection, blur: blur ? 'true' : undefined, layout: layout, badge_shape: badgeShape, badge_alpha: badgeAlpha, badge_size: badgeSize, logo_size: logoSize, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
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
  previewPoster: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, badgeDirection?: string, textSize?: number, ratingsExclude?: string, layout?: string, badgeShape?: string, badgeAlpha?: number, posterFit?: string, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    keyRequest(buildUrl('/api/key/me/preview/poster', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, badge_direction: badgeDirection, text_size: textSize, layout: layout, badge_shape: badgeShape, badge_alpha: badgeAlpha, fit: posterFit, badge_size: badgeSize, logo_size: logoSize, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
  previewLogo: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, badgeSize?: number, logoSize?: number, layout?: string, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    keyRequest(buildUrl('/api/key/me/preview/logo', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, text_size: textSize, badge_shape: badgeShape, badge_alpha: badgeAlpha, badge_size: badgeSize, logo_size: logoSize, layout: layout, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
  previewBackdrop: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, badgeDirection?: string, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, edgeInsetX?: number, edgeInsetY?: number, layout?: string, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    keyRequest(buildUrl('/api/key/me/preview/backdrop', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, text_size: textSize, badge_direction: badgeDirection, badge_shape: badgeShape, badge_alpha: badgeAlpha, edge_inset_x: edgeInsetX, edge_inset_y: edgeInsetY, layout: layout, badge_size: badgeSize, logo_size: logoSize, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
  previewEpisode: (ratingsLimit: number, ratingsOrder: string, badgeStyle?: string, labelStyle?: string, textSize?: number, badgeDirection?: string, blur?: boolean, layout?: string, ratingsExclude?: string, badgeShape?: string, badgeAlpha?: number, badgeSize?: number, logoSize?: number, colors?: Record<string, SourceColors>, badgeWidth?: number, badgeHeight?: number): Promise<Response> =>
    keyRequest(buildUrl('/api/key/me/preview/episode', { ratings_limit: ratingsLimit, ratings_order: ratingsOrder, ratings_exclude: ratingsExclude, badge_style: badgeStyle, label_style: labelStyle, text_size: textSize, badge_direction: badgeDirection, blur: blur ? 'true' : undefined, layout: layout, badge_shape: badgeShape, badge_alpha: badgeAlpha, badge_size: badgeSize, logo_size: logoSize, colors: colorsQuery(colors), badge_width: badgeWidth, badge_height: badgeHeight })),
}

export const keysApi = {
  list: (): Promise<Response> => get('/api/keys'),
  create: (name: string): Promise<Response> => post('/api/keys', { name }),
  delete: (id: number): Promise<Response> => del(`/api/keys/${id}`),
  getSettings: (id: number): Promise<Response> => get(`/api/keys/${id}/settings`),
  updateSettings: (id: number, settings: SaveSettingsPayload): Promise<Response> => put(`/api/keys/${id}/settings`, settings),
  deleteSettings: (id: number): Promise<Response> => del(`/api/keys/${id}/settings`),
}
