import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mockAuthStore = {
  token: null as string | null,
  apiKeyToken: 'test-jwt-token',
  refresh: vi.fn(),
  logout: vi.fn(),
}

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => mockAuthStore,
}))

import { selfApi } from '@/lib/api'
import type { SaveSettingsPayload } from '@/lib/settings'

function makeFetchResponse(status: number, body: unknown = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  }
}

/** A complete SaveSettingsPayload fixture; override individual fields per test. */
function makePayload(overrides: Partial<SaveSettingsPayload> = {}): SaveSettingsPayload {
  const layout = { top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 0, rows: 0, start: 'c' }, bottom: { per_row: 3, rows: 1, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }
  const episodeLayout = { top: { per_row: 0, rows: 0, start: 'c' }, right: { per_row: 1, rows: 1, start: 't' }, bottom: { per_row: 0, rows: 0, start: 'c' }, left: { per_row: 0, rows: 0, start: 'c' }, order: ['bottom', 'top', 'left', 'right'] }
  return {
    image_source: 't',
    lang: 'en',
    textless: false,
    ratings_limit: 3,
    ratings_order: 'mal,imdb,trakt',
    ratings_exclude: '',
    poster_layout: layout,
    logo_ratings_limit: 3,
    backdrop_ratings_limit: 3,
    poster_badge_style: 'h',
    logo_badge_style: 'h',
    backdrop_badge_style: 'v',
    poster_label_style: 't',
    logo_label_style: 't',
    backdrop_label_style: 't',
    poster_badge_direction: 'd',
    poster_fit: 'native',
    poster_text_size: 100,
    logo_text_size: 100,
    backdrop_text_size: 100,
    poster_badge_size: 100,
    logo_badge_size: 100,
    backdrop_badge_size: 100,
    poster_logo_size: 100,
    logo_logo_size: 100,
    backdrop_logo_size: 100,
    logo_layout: layout,
    backdrop_layout: layout,
    backdrop_badge_direction: 'd',
    backdrop_edge_inset_x: 0,
    backdrop_edge_inset_y: 0,
    episode_ratings_limit: 1,
    episode_badge_style: 'v',
    episode_label_style: 'o',
    episode_text_size: 100,
    episode_badge_size: 100,
    episode_logo_size: 100,
    episode_layout: episodeLayout,
    episode_badge_direction: 'v',
    episode_blur: false,
    poster_badge_shape: 'r',
    logo_badge_shape: 'r',
    backdrop_badge_shape: 'r',
    episode_badge_shape: 'r',
    poster_badge_alpha: 80,
    logo_badge_alpha: 80,
    backdrop_badge_alpha: 80,
    episode_badge_alpha: 80,
    ...overrides,
  }
}

describe('selfApi', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
    mockAuthStore.apiKeyToken = 'test-jwt-token'
    mockAuthStore.token = null
  })

  it('getInfo sends GET with JWT token as Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200, { name: 'k', key_prefix: 'ab' }))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.getInfo()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, options] = fetchMock.mock.calls[0]!
    expect(url).toContain('/api/key/me')
    expect(options.headers.get('Authorization')).toBe('Bearer test-jwt-token')
  })

  it('getSettings sends GET to /api/key/me/settings', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200, { image_source: 't' }))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.getSettings()

    const [url] = fetchMock.mock.calls[0]!
    expect(url).toContain('/api/key/me/settings')
  })

  it('updateSettings sends PUT with JSON body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200, { ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.updateSettings(
      makePayload({
        image_source: 'f',
        lang: 'de',
        textless: true,
        ratings_order: 'mal,imdb,trakt',
        ratings_exclude: 'rt',
        poster_text_size: 150,
        logo_text_size: 95,
        backdrop_text_size: 170,
      }),
    )

    const [url, options] = fetchMock.mock.calls[0]!
    expect(url).toContain('/api/key/me/settings')
    expect(options.method).toBe('PUT')
    expect(options.headers.get('Content-Type')).toBe('application/json')
    expect(JSON.parse(options.body)).toEqual(
      makePayload({
        image_source: 'f',
        lang: 'de',
        textless: true,
        ratings_order: 'mal,imdb,trakt',
        ratings_exclude: 'rt',
        poster_text_size: 150,
        logo_text_size: 95,
        backdrop_text_size: 170,
      }),
    )
  })

  it('resetSettings sends DELETE', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200, { ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.resetSettings()

    const [url, options] = fetchMock.mock.calls[0]!
    expect(url).toContain('/api/key/me/settings')
    expect(options.method).toBe('DELETE')
  })

  it('does not set Authorization when apiKeyToken is null', async () => {
    mockAuthStore.apiKeyToken = null as unknown as string
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.getInfo()

    const [, options] = fetchMock.mock.calls[0]!
    expect(options.headers.has('Authorization')).toBe(false)
  })

  it('does not include credentials (no cookie needed)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.getInfo()

    const [, options] = fetchMock.mock.calls[0]!
    // keyRequest does not set credentials: 'include'
    expect(options.credentials).toBeUndefined()
  })

  it('preview calls GET with the kind path and params', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.preview('poster', { ratingsLimit: 3, ratingsOrder: 'imdb,rt,tmdb' })

    const [url] = fetchMock.mock.calls[0]!
    expect(url).toContain('/api/key/me/preview/poster')
    expect(url).toContain('ratings_limit=3')
    expect(url).toContain('ratings_order=imdb%2Crt%2Ctmdb')
  })

  it('preview includes layout when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.preview('poster', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', layout: '{"bottom":{"per_row":2}}' })

    const [url] = fetchMock.mock.calls[0]!
    expect(url).toContain('layout=%7B%22bottom%22%3A%7B%22per_row%22%3A2%7D%7D')
  })

  it('preview includes label_style when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.preview('logo', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', badgeStyle: 'h', labelStyle: 'i' })

    const [url] = fetchMock.mock.calls[0]!
    expect(url).toContain('/api/key/me/preview/logo')
    expect(url).toContain('label_style=i')
  })

  it('preview includes badge_direction when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.preview('poster', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', badgeStyle: 'h', labelStyle: 'i', badgeDirection: 'v' })

    const [url] = fetchMock.mock.calls[0]!
    expect(url).toContain('badge_direction=v')
  })

  it('preview episode includes blur=true when enabled', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.preview('episode', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', blur: true })

    const [url] = fetchMock.mock.calls[0]!
    expect(url).toContain('blur=true')
  })

  it('preview episode omits blur when false', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.preview('episode', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', blur: false })

    const [url] = fetchMock.mock.calls[0]!
    expect(url).not.toContain('blur')
  })

  it('preview backdrop includes edge insets when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.preview('backdrop', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', edgeInsetX: 8, edgeInsetY: 3 })

    const [url] = fetchMock.mock.calls[0]!
    expect(url).toContain('edge_inset_x=8')
    expect(url).toContain('edge_inset_y=3')
  })
  it('updateSettings includes episode fields', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200, { ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await selfApi.updateSettings(
      makePayload({
        image_source: 't',
        lang: 'en',
        textless: false,
        ratings_order: 'imdb,rt,tmdb',
        poster_text_size: 150,
        logo_text_size: 95,
        backdrop_text_size: 170,
        episode_badge_style: 'v',
        episode_label_style: 'o',
        episode_text_size: 100,
        episode_badge_direction: 'v',
      }),
    )

    const [, options] = fetchMock.mock.calls[0]!
    const body = JSON.parse(options.body)
    expect(body.episode_ratings_limit).toBe(1)
    expect(body.episode_badge_style).toBe('v')
    expect(body.episode_label_style).toBe('o')
    expect(body.episode_text_size).toBe(100)
    expect(body.episode_layout.right.per_row).toBe(1)
    expect(body.episode_badge_direction).toBe('v')
    expect(body.episode_blur).toBe(false)
  })
})
