import { test, expect, type Locator, type Page } from '@playwright/test'
import { resetGlobalSettings } from './helpers'

test.describe('api keys', () => {
  test.beforeEach(async ({ page, request }) => {
    // Ensure admin exists and login
    await request.post('/api/auth/setup', {
      data: { username: 'admin', password: 'testpassword123' },
    })

    const loginRes = await request.post('/api/auth/login', {
      data: { username: 'admin', password: 'testpassword123' },
    })
    const { token } = await loginRes.json()

    // Reset global settings to defaults so tests start from a known state
    // (settings.spec.ts changes e.g. the poster label style, which new keys
    // inherit).
    await resetGlobalSettings(request, token)

    await page.goto('/login')
    await page.fill('#username', 'admin')
    await page.fill('#password', 'testpassword123')
    await page.click('button[type="submit"]')
    await expect(page).toHaveURL(/\/admin/)

    // API keys live under Settings → API, tab "API Keys".
    await page.goto('/admin/settings/api?tab=api-keys')
    await expect(page.locator('input[placeholder*="Key name"]')).toBeVisible()
  })

  async function createKey(page: Page, name: string) {
    await page.fill('input[placeholder*="Key name"]', name)
    await page.click('button:has-text("Create")')
    await expect(page.getByTestId('key-row').filter({ hasText: name })).toBeVisible()
  }

  test('create a new API key and see its value', async ({ page }) => {
    await createKey(page, 'test-key')

    // Key value banner should appear
    await expect(page.locator("text=Here's your API key")).toBeVisible()
    await expect(page.locator('code')).toBeVisible()
  })

  test('dismiss key banner', async ({ page }) => {
    await createKey(page, 'dismiss-test')

    await expect(page.locator("text=Here's your API key")).toBeVisible()
    await page.click('button:has-text("Dismiss")')
    await expect(page.locator("text=Here's your API key")).not.toBeVisible()
  })

  test('key appears in list with correct name', async ({ page }) => {
    await createKey(page, 'my-prod-key')
  })

  test('refresh button is visible and clickable', async ({ page }) => {
    const refreshButton = page.locator('button:has-text("Refresh")')
    await expect(refreshButton).toBeVisible()

    await refreshButton.click()

    // Button should still be present after refresh completes
    await expect(refreshButton).toBeVisible()
  })

  /**
   * Create a key with a unique name and expand its settings panel. Returns
   * the key's row; every lookup is scoped to it, since the free-key card on
   * the same page uses some of the same test ids.
   */
  let settingsKeyCounter = 0
  async function createKeyAndOpenSettings(page: Page): Promise<Locator> {
    const keyName = `settings-key-${++settingsKeyCounter}-${Date.now()}`
    await createKey(page, keyName)

    const dismissBtn = page.locator('button:has-text("Dismiss")')
    if (await dismissBtn.isVisible()) await dismissBtn.click()

    const row = page.getByTestId('key-row').filter({ hasText: keyName })
    await toggleSettings(row)
    await expect(row.getByTestId('form-tab-image-settings')).toBeVisible()
    return row
  }

  async function toggleSettings(row: Locator) {
    await row.locator('[data-testid^="key-settings-"]').click()
  }

  /** Per-key settings are saved with the page header's Save button. */
  async function saveAll(page: Page) {
    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })
  }

  test('per-key settings panel shows the rating order', async ({ page }) => {
    const row = await createKeyAndOpenSettings(page)

    await row.getByTestId('form-tab-rating-order').click()
    await expect(row.getByTestId('exclude-rt-eye')).toBeVisible()
  })

  test('per-key poster layout defaults to 3 ratings', async ({ page }) => {
    const row = await createKeyAndOpenSettings(page)

    await row.getByTestId('form-tab-poster').click()
    await expect(row.getByTestId('poster-layout-editor')).toContainText('3 ratings shown')
  })

  test('per-key layout persists after save', async ({ page }) => {
    const row = await createKeyAndOpenSettings(page)

    await row.getByTestId('form-tab-poster').click()
    await row.getByTestId('poster-top-per-row').fill('2')
    await row.getByTestId('poster-top-rows').fill('1')
    await saveAll(page)

    // Collapse and re-expand settings to verify persistence
    await toggleSettings(row)
    await expect(row.getByTestId('form-tab-image-settings')).not.toBeVisible()
    await toggleSettings(row)
    await row.getByTestId('form-tab-poster').click()

    await expect(row.getByTestId('poster-top-per-row')).toHaveValue('2')
    await expect(row.getByTestId('poster-top-rows')).toHaveValue('1')
  })

  test('per-key label style dropdowns are visible with default official', async ({ page }) => {
    const row = await createKeyAndOpenSettings(page)

    for (const kind of ['poster', 'logo', 'backdrop'] as const) {
      await row.getByTestId(`form-tab-${kind}`).click()
      const select = row.getByTestId(`${kind}-label-style-select`)
      await expect(select).toBeVisible()
      await expect(select).toContainText('Official')
    }
  })

  test('per-key label style persists after save', async ({ page }) => {
    const row = await createKeyAndOpenSettings(page)

    await row.getByTestId('form-tab-poster').click()
    await row.getByTestId('poster-label-style-select').click()
    await page.getByRole('option', { name: 'Text', exact: true }).click()
    await saveAll(page)

    // Collapse and re-expand settings to verify persistence
    await toggleSettings(row)
    await expect(row.getByTestId('form-tab-image-settings')).not.toBeVisible()
    await toggleSettings(row)
    await row.getByTestId('form-tab-poster').click()

    await expect(row.getByTestId('poster-label-style-select')).toContainText('Text')
  })

  test('delete a key removes it from list', async ({ page }) => {
    await createKey(page, 'to-delete')

    const dismissBtn = page.locator('button:has-text("Dismiss")')
    if (await dismissBtn.isVisible()) await dismissBtn.click()

    // Accept the confirm dialog
    page.on('dialog', (dialog) => dialog.accept())

    const row = page.getByTestId('key-row').filter({ hasText: 'to-delete' })
    await row.locator('button:has-text("Delete")').click()

    await expect(row).toHaveCount(0)
  })
})
