import { useAuthStore } from '@/stores/auth'
import type { PreviewKind, PreviewParams, SaveSettingsPayload, SourceColors } from '@/lib/settings'

const BASE_URL = import.meta.env.VITE_API_URL || ''

let _onAuthFailure: (() => void) | null = null

export function setOnAuthFailure(callback: () => void) {
  _onAuthFailure = callback
}

// authRequest is the shared fetcher for both the admin (JWT cookie) and
// self (API-key) callers. Sets Authorization Bearer when a token is
// provided, sets Content-Type when a body is present, and forwards
// `credentials: 'include'` only when `withCredentials` is true
// (cookie-based JWT auth) — API-key auth doesn't need cookies.
async function authRequest(
  path: string,
  options: RequestInit,
  token: string | null | undefined,
  withCredentials: boolean,
): Promise<Response> {
  const headers = new Headers(options.headers)
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  return fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
    ...(withCredentials ? { credentials: 'include' as RequestCredentials } : {}),
  })
}

// Admin (JWT cookie-based) — wraps authRequest and adds a 401-refresh-retry
// loop. On 401, tries to refresh the token; if that succeeds, re-sends the
// request with the new token; if not, clears credentials and returns the
// original 401.
async function request(path: string, options: RequestInit = {}): Promise<Response> {
  const auth = useAuthStore()
  const token = auth.token
  let res = await authRequest(path, options, token, true)

  if (res.status === 401 && token) {
    const refreshed = await auth.refresh()
    if (refreshed) {
      res = await authRequest(path, options, auth.token, true)
    } else {
      // Refresh failed — clear credentials and redirect without retrying
      auth.logout()
      _onAuthFailure?.()
    }
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
export function colorsQuery(colors?: Record<string, SourceColors>): string | undefined {
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

// Per-kind endpoint config — the only thing that varies between kinds is
// the URL path (plural form) and whether the image endpoint appends
// "/image" (poster + episode) or not (logo + backdrop).
interface AdminImageEndpoints {
  plural: string         // 'posters' | 'logos' | 'backdrops' | 'episodes'
  imageSuffix: string    // '/image' for poster/episode, '' for logo/backdrop
}

const ADMIN_IMAGE_ENDPOINTS: Record<ImageListKind, AdminImageEndpoints> = {
  poster:   { plural: 'posters',   imageSuffix: '/image' },
  logo:     { plural: 'logos',     imageSuffix: '' },
  backdrop: { plural: 'backdrops', imageSuffix: '' },
  episode:  { plural: 'episodes',  imageSuffix: '/image' },
}

interface AdminImageApi {
  clear: () => Promise<Response>
  list: (page: number, pageSize: number, sortBy?: string, sortDir?: string) => Promise<Response>
  image: (key: string) => Promise<Response>
  fetch: (idType: string, idValue: string) => Promise<Response>
  purge: (idType: string, idValue: string, scope?: PurgeScope) => Promise<Response>
}

// makeAdminImageApi builds the 5 per-kind endpoints from the per-kind path
// config. Adding a new ImageListKind is a 1-line ADMIN_IMAGE_ENDPOINTS
// entry + a makeAdminImageApi(...) call.
function makeAdminImageApi(cfg: AdminImageEndpoints): AdminImageApi {
  return {
    clear: () => del(`/api/admin/${cfg.plural}`),
    list: (page, pageSize, sortBy, sortDir) =>
      get(buildUrl(`/api/admin/${cfg.plural}`, { page, page_size: pageSize, sort_by: sortBy, sort_dir: sortDir })),
    image: (key) => get(`/api/admin/${cfg.plural}/${key}${cfg.imageSuffix}`),
    fetch: (idType, idValue) => post(`/api/admin/${cfg.plural}/${idType}/${idValue}/fetch`),
    purge: (idType, idValue, scope = 'title') => del(purgeUrl(cfg.plural, idType, idValue, scope)),
  }
}

const POSTER_API = makeAdminImageApi(ADMIN_IMAGE_ENDPOINTS.poster)
const LOGO_API = makeAdminImageApi(ADMIN_IMAGE_ENDPOINTS.logo)
const BACKDROP_API = makeAdminImageApi(ADMIN_IMAGE_ENDPOINTS.backdrop)
const EPISODE_API = makeAdminImageApi(ADMIN_IMAGE_ENDPOINTS.episode)

export const adminApi = {
  getStats: (): Promise<Response> => get('/api/admin/stats'),
  purgeAll: (): Promise<Response> => post('/api/admin/cache/purge'),
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
  preview: (kind: PreviewKind, params: PreviewParams): Promise<Response> =>
    get(buildUrl(`/api/admin/preview/${kind}`, previewQuery(params))),

  // Per-kind endpoints — generated by makeAdminImageApi from
  // ADMIN_IMAGE_ENDPOINTS. Adding a new kind is a 1-line entry there.
  clearPosters: POSTER_API.clear,
  getPosters: POSTER_API.list,
  getPosterImage: POSTER_API.image,
  fetchPoster: POSTER_API.fetch,
  purgePoster: POSTER_API.purge,
  clearLogos: LOGO_API.clear,
  getLogos: LOGO_API.list,
  getLogoImage: LOGO_API.image,
  fetchLogo: LOGO_API.fetch,
  purgeLogo: LOGO_API.purge,
  clearBackdrops: BACKDROP_API.clear,
  getBackdrops: BACKDROP_API.list,
  getBackdropImage: BACKDROP_API.image,
  fetchBackdrop: BACKDROP_API.fetch,
  purgeBackdrop: BACKDROP_API.purge,
  clearEpisodes: EPISODE_API.clear,
  getEpisodes: EPISODE_API.list,
  getEpisodeImage: EPISODE_API.image,
  fetchEpisode: EPISODE_API.fetch,
  purgeEpisode: EPISODE_API.purge,
}

// --- Self-service API (API key session JWT auth) ---

// Self (API-key based) — no retry, no cookies, just an Authorization
// header from the apiKeyToken.
async function keyRequest(path: string, options: RequestInit = {}): Promise<Response> {
  const auth = useAuthStore()
  return authRequest(path, options, auth.apiKeyToken, false)
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

/** Kinds supported by ImageListView — single source of truth (admin gallery). */
export type ImageListKind = 'poster' | 'logo' | 'backdrop' | 'episode'

// buildImageListConfig re-projects an AdminImageApi into the prop shape
// the ImageListView expects (clearAllFn instead of clear, deleteFn
// instead of purge). One call per kind — no parallel function list to
// drift.
function buildImageListConfig(api: AdminImageApi) {
  return {
    listFn: api.list,
    imageFn: api.image,
    fetchFn: api.fetch,
    deleteFn: api.purge,
    clearAllFn: api.clear,
  }
}

const IMAGE_LIST_CONFIGS: Record<ImageListKind, ReturnType<typeof buildImageListConfig>> = {
  poster: buildImageListConfig(POSTER_API),
  logo: buildImageListConfig(LOGO_API),
  backdrop: buildImageListConfig(BACKDROP_API),
  episode: buildImageListConfig(EPISODE_API),
}

/** Per-kind ImageListView props lookup — lets a single wrapper render the
 *  right admin functions without 4 near-identical view stubs. */
export function imageListConfig(kind: ImageListKind) {
  return IMAGE_LIST_CONFIGS[kind]
}

export const IMAGE_LIST_KINDS = Object.keys(IMAGE_LIST_CONFIGS) as ImageListKind[]

/** Display title for each kind (capitalised). */
export const IMAGE_LIST_TITLES: Record<ImageListKind, string> = {
  poster: 'Posters',
  logo: 'Logos',
  backdrop: 'Backdrops',
  episode: 'Episodes',
}
