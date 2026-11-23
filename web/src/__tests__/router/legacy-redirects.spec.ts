import { describe, it, expect, vi } from 'vitest'
import type { RouteLocationRaw } from 'vue-router'
import router from '@/router/index'

vi.stubGlobal('sessionStorage', {
  getItem: vi.fn(() => null),
  setItem: vi.fn(),
  removeItem: vi.fn(),
})

// Rust-era bookmarks keep working: the old paths redirect to the pages that
// replaced them (checked against the real route table, not a test copy).
function redirectTarget(path: string) {
  const record = router.getRoutes().find((r) => r.path === path)
  expect(record, `no route for ${path}`).toBeTruthy()
  return router.resolve(record!.redirect as RouteLocationRaw)
}

describe('legacy route redirects', () => {
  it('/key-settings goes to the individual image settings page', () => {
    expect(redirectTarget('/key-settings').fullPath).toBe('/individual-image-settings')
  })

  it('/admin/keys opens the API keys tab', () => {
    const to = redirectTarget('/admin/keys')
    expect(to.name).toBe('settings-api')
    expect(to.query.tab).toBe('api-keys')
  })
})
