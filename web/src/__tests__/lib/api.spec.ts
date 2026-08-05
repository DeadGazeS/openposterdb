import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mockAuthStore = {
  token: 'test-token',
  refresh: vi.fn(),
  logout: vi.fn(),
}

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => mockAuthStore,
}))

import { get, post, put, del, setOnAuthFailure, adminApi } from '@/lib/api'

const mockOnAuthFailure = vi.fn()
setOnAuthFailure(mockOnAuthFailure)

function makeFetchResponse(status: number, body: unknown = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  }
}

describe('api', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
    mockAuthStore.token = 'test-token'
    mockAuthStore.refresh = vi.fn()
    mockAuthStore.logout = vi.fn()
    mockOnAuthFailure.mockClear()
  })

  it('get adds Authorization header when token exists', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200, { data: 'ok' }))
    vi.stubGlobal('fetch', fetchMock)

    await get('/api/test')

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, options] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/test')
    expect(options.headers.get('Authorization')).toBe('Bearer test-token')
  })

  it('post sends JSON body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await post('/api/items', { name: 'test' })

    const [, options] = fetchMock.mock.calls[0]!!
    expect(options.method).toBe('POST')
    expect(options.headers.get('Content-Type')).toBe('application/json')
    expect(options.body).toBe(JSON.stringify({ name: 'test' }))
  })

  it('del uses DELETE method', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await del('/api/items/1')

    const [, options] = fetchMock.mock.calls[0]!!
    expect(options.method).toBe('DELETE')
  })

  it('401 handling: attempts refresh, retries request on success', async () => {
    mockAuthStore.refresh.mockResolvedValue(true)
    mockAuthStore.token = 'refreshed-token'

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(makeFetchResponse(401))
      .mockResolvedValueOnce(makeFetchResponse(200, { data: 'ok' }))
    vi.stubGlobal('fetch', fetchMock)

    await get('/api/protected')

    expect(mockAuthStore.refresh).toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('401 + failed refresh: calls logout', async () => {
    mockAuthStore.refresh.mockResolvedValue(false)

    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(401))
    vi.stubGlobal('fetch', fetchMock)

    await get('/api/protected')

    expect(mockAuthStore.refresh).toHaveBeenCalled()
    expect(mockAuthStore.logout).toHaveBeenCalled()
    expect(mockOnAuthFailure).toHaveBeenCalled()
  })

  it('non-401 error passes through without refresh', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(500, { error: 'server error' }))
    vi.stubGlobal('fetch', fetchMock)

    const res = await get('/api/broken')

    expect(res.status).toBe(500)
    expect(mockAuthStore.refresh).not.toHaveBeenCalled()
    expect(mockAuthStore.logout).not.toHaveBeenCalled()
  })

  it('includes credentials in fetch calls', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await get('/api/test')

    const [, options] = fetchMock.mock.calls[0]!!
    expect(options.credentials).toBe('include')
  })

  it('does not set Authorization header when token is null', async () => {
    mockAuthStore.token = null as unknown as string
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await get('/api/public')

    const [, options] = fetchMock.mock.calls[0]!!
    expect(options.headers.has('Authorization')).toBe(false)
  })

  it('network error propagates', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchMock)

    await expect(get('/api/test')).rejects.toThrow('Failed to fetch')
  })

  it('401 without token does not attempt refresh', async () => {
    mockAuthStore.token = null as unknown as string
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(401))
    vi.stubGlobal('fetch', fetchMock)

    const res = await get('/api/protected')

    expect(res.status).toBe(401)
    expect(mockAuthStore.refresh).not.toHaveBeenCalled()
  })

  it('put sends JSON body with PUT method', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await put('/api/settings', { image_source: 'f' })

    const [, options] = fetchMock.mock.calls[0]!!
    expect(options.method).toBe('PUT')
    expect(options.headers.get('Content-Type')).toBe('application/json')
    expect(options.body).toBe(JSON.stringify({ image_source: 'f' }))
  })

  it('post without body does not set Content-Type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await post('/api/action')

    const [, options] = fetchMock.mock.calls[0]!!
    expect(options.headers.has('Content-Type')).toBe(false)
    expect(options.body).toBeUndefined()
  })

  it('adminApi.fetchPoster calls POST with correct URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.fetchPoster('imdb', 'tt0111161')

    const [url, options] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/admin/posters/imdb/tt0111161/fetch')
    expect(options.method).toBe('POST')
  })

  it('adminApi.fetchPoster works with tmdb id type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.fetchPoster('tmdb', '550')

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/admin/posters/tmdb/550/fetch')
  })

  it('adminApi.purgeAll calls POST /api/admin/cache/purge', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.purgeAll()

    const [url, options] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/admin/cache/purge')
    expect(options.method).toBe('POST')
  })

  it('adminApi.clearPosters calls DELETE on the posters collection', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.clearPosters()

    const [url, options] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/admin/posters')
    expect(options.method).toBe('DELETE')
  })

  it('adminApi.purgePoster calls DELETE with the title id', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.purgePoster('imdb', 'tt0111161')

    const [url, options] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/admin/posters/imdb/tt0111161')
    expect(options.method).toBe('DELETE')
  })

  it('adminApi.purgeEpisode encodes the id value', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.purgeEpisode('tmdb', 'episode-1396-S1E1')

    const [url, options] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/admin/episodes/tmdb/episode-1396-S1E1')
    expect(options.method).toBe('DELETE')
  })

  it('adminApi.purgePoster with variant scope encodes the cache value and adds ?scope=variant', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.purgePoster('imdb', 'tt0111161_t_de@imc', 'variant')

    const [url, options] = fetchMock.mock.calls[0]!!
    expect(url).toBe('/api/admin/posters/imdb/tt0111161_t_de%40imc?scope=variant')
    expect(options.method).toBe('DELETE')
  })

  it('adminApi.preview calls GET with the kind path and params', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.preview('poster', { ratingsLimit: 3, ratingsOrder: 'imdb,rt,tmdb' })

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).toContain('/api/admin/preview/poster')
    expect(url).toContain('ratings_limit=3')
    expect(url).toContain('ratings_order=imdb%2Crt%2Ctmdb')
  })

  it('adminApi.preview includes layout when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.preview('poster', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', layout: '{"bottom":{"per_row":2}}' })

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).toContain('layout=%7B%22bottom%22%3A%7B%22per_row%22%3A2%7D%7D')
  })

  it('adminApi.preview includes label_style when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.preview('logo', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', badgeStyle: 'h', labelStyle: 'i' })

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).toContain('/api/admin/preview/logo')
    expect(url).toContain('label_style=i')
  })

  it('adminApi.preview includes badge_direction when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.preview('poster', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', badgeStyle: 'h', labelStyle: 'i', badgeDirection: 'v' })

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).toContain('badge_direction=v')
  })

  it('adminApi.preview episode includes blur=true when enabled', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.preview('episode', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', blur: true })

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).toContain('blur=true')
  })

  it('adminApi.preview episode omits blur when false', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.preview('episode', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', blur: false })

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).not.toContain('blur')
  })

  it('adminApi.preview backdrop includes edge insets when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(makeFetchResponse(200))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.preview('backdrop', { ratingsLimit: 3, ratingsOrder: 'imdb,rt', edgeInsetX: 8, edgeInsetY: 3 })

    const [url] = fetchMock.mock.calls[0]!!
    expect(url).toContain('edge_inset_x=8')
    expect(url).toContain('edge_inset_y=3')
  })
})
