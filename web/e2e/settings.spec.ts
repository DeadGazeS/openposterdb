import { test, expect, type Page } from '@playwright/test'
import { selectOption, resetGlobalSettings } from './helpers'

test.describe('settings', () => {
  test.beforeEach(async ({ page, request }) => {
    // Ensure admin exists and login
    await request.post('/api/auth/setup', {
      data: { username: 'admin', password: 'testpassword123' },
    })

    const loginRes = await request.post('/api/auth/login', {
      data: { username: 'admin', password: 'testpassword123' },
    })
    const { token } = await loginRes.json()

    // Reset settings to known defaults so tests start from a deterministic state
    await resetGlobalSettings(request, token)

    await page.goto('/login')
    await page.fill('#username', 'admin')
    await page.fill('#password', 'testpassword123')
    await page.click('button[type="submit"]')
    await expect(page).toHaveURL(/\/admin/)

    // Navigate to the Settings group (expand the sidebar collapsible)
    await page.click('text=Settings')
    await page.getByRole('link', { name: 'General', exact: true }).click()
    await expect(page).toHaveURL(/\/admin\/settings\/general/)
  })

  /** The settings form lives on the Image section. */
  async function goToImage(page: Page) {
    await page.getByRole('link', { name: 'Image', exact: true }).click()
    await expect(page).toHaveURL(/\/admin\/settings\/image/)
  }

  test('settings page loads with heading', async ({ page }) => {
    await expect(page.locator('h1')).toContainText('General')

    // The Image section holds the actual settings form.
    await goToImage(page)
    await expect(page.locator('h1')).toContainText('Image')
    await expect(page.locator('text=Global Image Settings')).toBeVisible()
  })

  test('fanart checkbox is visible', async ({ page }) => {
    await goToImage(page)
    await expect(page.getByTestId('fanart-checkbox')).toBeVisible()
  })

  test('language and textless options are always enabled', async ({ page }) => {
    await goToImage(page)
    await expect(page.getByTestId('lang-select')).toBeVisible()
    await expect(page.getByTestId('lang-select')).toBeEnabled()
    await expect(page.getByTestId('textless-checkbox')).toBeVisible()
    await expect(page.getByTestId('textless-checkbox')).toBeEnabled()
  })

  test('changes are only saved when Save is clicked', async ({ page }) => {
    await goToImage(page)

    await page.getByTestId('fanart-checkbox').check()

    // The form becomes dirty (Discard appears) but nothing persists on its own.
    await expect(page.getByTestId('discard-settings-button')).toBeVisible()
    await expect(page.locator('text=Saved')).toBeHidden()

    // Saving is explicit — the confirmation only appears after clicking Save.
    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })
  })

  test('fanart options persist after save and reload', async ({ page }) => {
    await goToImage(page)

    // Enable fanart and textless
    await page.getByTestId('fanart-checkbox').check()
    await page.getByTestId('textless-checkbox').check()
    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })

    // Reload page
    await page.reload()
    await expect(page.locator('h1')).toContainText('Image')

    // Fanart and textless should be checked
    await expect(page.getByTestId('fanart-checkbox')).toBeChecked()
    await expect(page.getByTestId('textless-checkbox')).toBeChecked()
  })

  test('refresh button is visible and clickable', async ({ page }) => {
    const refreshButton = page.locator('button:has-text("Refresh")')
    await expect(refreshButton).toBeVisible()

    await refreshButton.click()
    await expect(refreshButton).toBeVisible()
  })

  test('rating display section is visible', async ({ page }) => {
    await goToImage(page)
    await page.getByTestId('form-tab-ratings').click()
    await expect(page.locator('text=Rating Display')).toBeVisible()
    await expect(page.locator('text=Rating order')).toBeVisible()
  })

  test('ratings count is driven by the layout grid', async ({ page }) => {
    await goToImage(page)
    await page.getByTestId('form-tab-poster').click()

    // The default poster layout is 3 badges across one bottom row.
    const layoutEditor = page.getByTestId('poster-layout-editor')
    await expect(layoutEditor).toBeVisible()
    await expect(layoutEditor).toContainText('3 ratings shown')

    // Adding badges to another side increases the total.
    await page.getByTestId('poster-top-per-row').fill('2')
    await page.getByTestId('poster-top-rows').fill('1')
    await expect(layoutEditor).toContainText('5 ratings shown')
  })

  test('all rating sources are listed with an eye toggle', async ({ page }) => {
    await goToImage(page)
    await page.getByTestId('form-tab-ratings').click()

    await expect(page.locator('text=Rating order')).toBeVisible()
    for (const key of ['imdb', 'tmdb', 'rt', 'rta', 'mc', 'trakt', 'lb', 'mal', 'mdblist', 'ebert']) {
      await expect(page.getByTestId(`exclude-${key}-eye`)).toBeVisible()
    }
  })

  test('layout grid changes persist after save and reload', async ({ page }) => {
    await goToImage(page)
    await page.getByTestId('form-tab-poster').click()

    await page.getByTestId('poster-top-per-row').fill('2')
    await page.getByTestId('poster-top-rows').fill('1')
    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })

    // Reload
    await page.reload()
    await expect(page.locator('h1')).toContainText('Image')
    await page.getByTestId('form-tab-poster').click()

    // The layout should be preserved
    await expect(page.getByTestId('poster-top-per-row')).toHaveValue('2')
    await expect(page.getByTestId('poster-top-rows')).toHaveValue('1')
  })

  test('excluding a rating via the eye persists after save and reload', async ({ page }) => {
    await goToImage(page)
    await page.getByTestId('form-tab-ratings').click()

    const excludeRt = page.getByTestId('exclude-rt-eye')
    await expect(excludeRt).toHaveAttribute('aria-pressed', 'false')

    // Capture the save payload so we can verify ratings_exclude reaches the API.
    const captured: { payload: Record<string, unknown> | null } = { payload: null }
    await page.route('**/api/admin/settings', async (route) => {
      if (route.request().method() === 'PUT') {
        captured.payload = route.request().postDataJSON()
      }
      await route.continue()
    })

    // Exclude Rotten Tomatoes (Critics) — the scenario from issue #17
    await excludeRt.click()
    await expect(excludeRt).toHaveAttribute('aria-pressed', 'true')
    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })
    expect(captured.payload?.ratings_exclude).toBe('rt')

    await page.reload()
    await expect(page.locator('h1')).toContainText('Image')
    await page.getByTestId('form-tab-ratings').click()

    // Exclusion should be preserved across reload
    await expect(page.getByTestId('exclude-rt-eye')).toHaveAttribute('aria-pressed', 'true')
  })

  test('sidebar navigation to settings works', async ({ page }) => {
    // Navigate away
    await page.click('text=Dashboard')
    await expect(page).toHaveURL(/\/admin$/)

    // Navigate back via sidebar (expand the Settings group, then pick General)
    await page.click('text=Settings')
    await page.getByRole('link', { name: 'General', exact: true }).click()
    await expect(page).toHaveURL(/\/admin\/settings\/general/)
  })

  test('preview section is visible', async ({ page }) => {
    await goToImage(page)
    await page.getByTestId('form-tab-poster').click()

    await expect(page.locator('img[alt="Poster preview"]')).toHaveCount(1)
    await expect(page.getByTestId('poster-layout-editor')).toBeVisible()
  })

  test('preview updates when the layout grid changes', async ({ page }) => {
    // Track every poster preview request so we can assert the debounced refetch.
    const previewLayouts: (string | null)[] = []
    page.on('request', (req) => {
      if (req.url().includes('/api/admin/preview/poster')) {
        previewLayouts.push(new URL(req.url()).searchParams.get('layout'))
      }
    })

    await goToImage(page)

    // Change a per-row value → the preview refetches with the new layout.
    await page.getByTestId('form-tab-poster').click()
    await page.getByTestId('poster-top-per-row').fill('2')
    await page.getByTestId('poster-top-rows').fill('1')

    await expect.poll(() => previewLayouts[previewLayouts.length - 1], { timeout: 5000 }).toContain('"per_row":2')
  })

  test('label style dropdowns are visible with default official', async ({ page }) => {
    await goToImage(page)

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

  test('label style persists after change and reload', async ({ page }) => {
    await goToImage(page)
    await page.getByTestId('form-tab-poster').click()

    // Change poster label style to Text
    const labelSelect = page.getByTestId('poster-label-style-select')
    await selectOption(page, labelSelect, 'Text')

    await page.getByTestId('save-settings-button').click()
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })

    await page.reload()
    await expect(page.locator('h1')).toContainText('Image')
    await page.getByTestId('form-tab-poster').click()

    await expect(page.getByTestId('poster-label-style-select')).toContainText('Text')
  })
})
