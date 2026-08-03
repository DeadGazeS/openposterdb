import { test, expect, type APIRequestContext, type Page } from '@playwright/test'
import { selectOption, resetGlobalSettings } from './helpers'

test.describe('key settings (self-service)', () => {
  /** Ensure admin exists and return an admin JWT token. */
  async function ensureAdmin(request: APIRequestContext): Promise<string> {
    await request.post('/api/auth/setup', {
      data: { username: 'admin', password: 'testpassword123' },
    })
    const loginRes = await request.post('/api/auth/login', {
      data: { username: 'admin', password: 'testpassword123' },
    })
    const { token } = await loginRes.json()
    return token
  }

  /** Create admin + API key, login with key via UI. */
  async function loginWithApiKey(
    page: Page,
    request: APIRequestContext,
  ): Promise<string> {
    const token = await ensureAdmin(request)

    const keyRes = await request.post('/api/keys', {
      headers: { Authorization: `Bearer ${token}` },
      data: { name: 'settings-test-key' },
    })
    const keyData = await keyRes.json()
    const apiKey = keyData.key

    // Login via UI with API key
    await page.goto('/login')
    await page.click('text=Sign in with API key instead')
    await page.fill('#apikey', apiKey)
    await page.click('button[type="submit"]')
    await expect(page).toHaveURL(/\/key-settings/)

    return apiKey
  }

  test('displays settings form with defaults', async ({ page, request }) => {
    // Ensure global settings are at a known state (other tests may change them)
    const adminToken = await ensureAdmin(request)
    await resetGlobalSettings(request, adminToken)

    await loginWithApiKey(page, request)

    await expect(page.locator('h1')).toContainText('Image Settings')
    await expect(page.locator('text=settings-test-key')).toBeVisible()

    // Settings form should be present with fanart checkbox
    await expect(page.getByTestId('fanart-checkbox')).toBeVisible()
  })

  test('changes are only saved when Save is clicked', async ({ page, request }) => {
    await loginWithApiKey(page, request)

    // Change a setting — nothing persists until Save is clicked.
    await page.getByTestId('fanart-checkbox').check()
    await expect(page.getByTestId('discard-settings-button')).toBeVisible()
    await expect(page.locator('text=Saved')).toBeHidden()

    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })
  })

  test('language and textless options are always enabled', async ({ page, request }) => {
    await loginWithApiKey(page, request)

    await expect(page.getByTestId('lang-select')).toBeEnabled()
    await expect(page.getByTestId('textless-checkbox')).toBeEnabled()
  })

  test('settings persist after save and reload', async ({ page, request }) => {
    await loginWithApiKey(page, request)

    // Enable fanart
    await page.getByTestId('fanart-checkbox').check()
    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })

    // Reload
    await page.reload()
    await expect(page.locator('h1')).toContainText('Image Settings')

    // Settings should persist
    await expect(page.getByTestId('fanart-checkbox')).toBeChecked()
  })

  test('rating display section is visible', async ({ page, request }) => {
    await loginWithApiKey(page, request)
    await page.getByTestId('form-tab-ratings').click()

    await expect(page.locator('text=Rating Display')).toBeVisible()
    await expect(page.locator('text=Rating order')).toBeVisible()
  })

  test('ratings count is driven by the layout grid', async ({ page, request }) => {
    await loginWithApiKey(page, request)
    await page.getByTestId('form-tab-poster').click()

    // The poster layout defaults to 3 badges across one bottom row.
    const layoutEditor = page.getByTestId('poster-layout-editor')
    await expect(layoutEditor).toBeVisible()
    await expect(layoutEditor).toContainText('3 ratings shown')

    await page.getByTestId('poster-top-per-row').fill('2')
    await page.getByTestId('poster-top-rows').fill('1')
    await expect(layoutEditor).toContainText('5 ratings shown')
  })

  test('rating order list has an eye toggle per source', async ({ page, request }) => {
    await loginWithApiKey(page, request)
    await page.getByTestId('form-tab-ratings').click()

    await expect(page.locator('text=Rating order')).toBeVisible()
    await expect(page.getByTestId('exclude-rt-eye')).toBeVisible()
  })

  test('reset to defaults works', async ({ page, request }) => {
    // Ensure global settings are at a known state before testing reset.
    // Other tests (e.g. settings.spec.ts) may change global poster_source,
    // which would make the post-reset value unpredictable.
    const adminToken = await ensureAdmin(request)
    await resetGlobalSettings(request, adminToken)

    await loginWithApiKey(page, request)

    // Global is now "tmdb", key has no overrides → fanart should be unchecked
    await expect(page.getByTestId('fanart-checkbox')).not.toBeChecked()

    // Enable fanart and save explicitly.
    await page.getByTestId('fanart-checkbox').check()
    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })

    // Wait for "Using defaults" badge to disappear (confirms custom settings saved)
    await expect(page.locator('text=Using defaults')).toBeHidden()

    // Reset to defaults
    await page.locator('button:has-text("Reset to defaults")').click()

    // Should be back to global default (fanart unchecked) and not dirty.
    await expect(page.locator('text=Using defaults')).toBeVisible({ timeout: 10000 })
    await expect(page.getByTestId('fanart-checkbox')).not.toBeChecked()
    await expect(page.getByTestId('discard-settings-button')).toBeHidden()
  })

  test('label style dropdowns are visible', async ({ page, request }) => {
    await loginWithApiKey(page, request)

    // Check poster, logo, and backdrop label style selects
    for (const [testId, tab] of [
      ['poster-label-style-select', 'poster'],
      ['logo-label-style-select', 'logo'],
      ['backdrop-label-style-select', 'backdrop'],
    ] as const) {
      await page.getByTestId(`form-tab-${tab}`).click()
      const select = page.getByTestId(testId)
      await expect(select).toBeVisible()
      await expect(select).toContainText('Official')
    }
  })

  test('label style persists after change and reload', async ({ page, request }) => {
    await loginWithApiKey(page, request)
    await page.getByTestId('form-tab-poster').click()

    // Change poster label style to Text
    const labelSelect = page.getByTestId('poster-label-style-select')
    await selectOption(page, labelSelect, 'Text')

    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })

    // Reload and verify persistence
    await page.reload()
    await expect(page.locator('h1')).toContainText('Image Settings')
    await page.getByTestId('form-tab-poster').click()

    await expect(page.getByTestId('poster-label-style-select')).toContainText('Text')
  })
})
