import { test, expect, type APIRequestContext, type Page } from '@playwright/test'
import { selectOption } from './helpers'

const TEST_IMDB_ID = 'tt0111161'

/** Get admin JWT via API. */
async function getAdminToken(request: APIRequestContext): Promise<string> {
  await request.post('/api/auth/setup', {
    data: { username: 'admin', password: 'testpassword123' },
  })
  const loginRes = await request.post('/api/auth/login', {
    data: { username: 'admin', password: 'testpassword123' },
  })
  const { token } = await loginRes.json()
  return token
}

/**
 * Set (or clear) the free API key flag without resetting other settings.
 *
 * Sends a minimal partial payload: the admin PUT accepts free_api_key_enabled
 * directly and treats the other fields as optional. Do NOT echo the GET payload
 * back — its layout fields arrive as JSON strings, which the PUT's object-typed
 * fields reject with a 400 (which silently left the flag untouched before).
 */
async function setFreeApiKey(request: APIRequestContext, enabled: boolean): Promise<string> {
  const token = await getAdminToken(request)
  const res = await request.put('/api/admin/settings', {
    headers: { Authorization: `Bearer ${token}` },
    data: { free_api_key_enabled: enabled },
  })
  expect(res.status()).toBe(200)
  return token
}

/** Check if real API keys are configured by attempting a poster fetch. */
async function hasRealKeys(request: APIRequestContext, token: string): Promise<boolean> {
  const keyRes = await request.post('/api/keys', {
    headers: { Authorization: `Bearer ${token}` },
    data: { name: 'key-check' },
  })
  const { key } = await keyRes.json()
  const res = await request.get(`/${key}/imdb/poster-default/${TEST_IMDB_ID}.jpg`, {
    timeout: 60_000,
  })
  return res.status() === 200
}

/** Login as admin and navigate to the General settings section. */
async function loginAndGoToSettings(page: Page, request: APIRequestContext) {
  await request.post('/api/auth/setup', {
    data: { username: 'admin', password: 'testpassword123' },
  })

  await page.goto('/login')
  await page.fill('#username', 'admin')
  await page.fill('#password', 'testpassword123')
  await page.click('button[type="submit"]')
  await expect(page).toHaveURL(/\/admin/)

  await page.click('text=Settings')
  await page.click('text=General')
  await expect(page).toHaveURL(/\/admin\/settings\/general/)
}

test.describe('free API key', () => {
  test('free API key toggle appears on settings page', async ({ page, request }) => {
    await loginAndGoToSettings(page, request)

    await expect(page.locator('text=Free API Key')).toBeVisible()
    await expect(page.locator('button[role="switch"]')).toBeVisible()
  })

  test('free API key toggle defaults to disabled', async ({ page, request }) => {
    await setFreeApiKey(request, false)

    await loginAndGoToSettings(page, request)

    const toggle = page.locator('button[role="switch"]')
    await expect(toggle).toHaveAttribute('aria-checked', 'false')
  })

  test('toggle free API key on and verify persistence', async ({ page, request }) => {
    await setFreeApiKey(request, false)

    await loginAndGoToSettings(page, request)

    const toggle = page.locator('button[role="switch"]')
    await expect(toggle).toHaveAttribute('aria-checked', 'false')

    // The switch only flips local state — persistence happens via the Save button.
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-checked', 'true')

    const putResponse = page.waitForResponse(
      (res) => res.url().includes('/api/admin/settings') && res.request().method() === 'PUT',
    )
    await page.getByTestId('save-settings-button').click()
    const response = await putResponse
    expect(response.status()).toBe(200)
    await expect(page.locator('text=Saved')).toBeVisible({ timeout: 5000 })

    // UI state has settled — reload and verify the flag persisted.
    await page.reload()
    await expect(page.locator('h1')).toContainText('General')
    await expect(page.locator('button[role="switch"]')).toHaveAttribute('aria-checked', 'true')
  })

  test('login page shows free key card when enabled', async ({ page, request }) => {
    await setFreeApiKey(request, true)

    // Visit login page (not logged in)
    await page.goto('/login')
    await expect(page.locator('text="Free API Key Available"')).toBeVisible()
    await expect(page.locator('code').filter({ hasText: /^t0-free-rpdb$/ })).toBeVisible()
  })

  test('login page hides free key card when disabled', async ({ page, request }) => {
    await setFreeApiKey(request, false)

    await page.goto('/login')
    await expect(page.locator('text=Free API Key Available')).toBeHidden()
  })

  test('key-login with free API key returns 401', async ({ request }) => {
    await setFreeApiKey(request, true)

    // Try to login with the free key — should fail (no self-service)
    const res = await request.post('/api/auth/key-login', {
      data: { api_key: 't0-free-rpdb' },
    })
    expect(res.status()).toBe(401)
  })

  test('poster endpoint with free key works when enabled', async ({ request }) => {
    await setFreeApiKey(request, true)

    // Request a poster — it may fail at TMDB fetch, but should not be 401
    const res = await request.get('/t0-free-rpdb/imdb/poster-default/tt0111161.jpg')
    expect(res.status()).not.toBe(401)
  })

  test('poster endpoint with free key returns 401 when disabled', async ({ request }) => {
    await setFreeApiKey(request, false)

    const res = await request.get('/t0-free-rpdb/imdb/poster-default/tt0111161.jpg')
    expect(res.status()).toBe(401)
  })
})

test.describe('free API key card', () => {
  test('card is hidden when free key is disabled', async ({ page, request }) => {
    await setFreeApiKey(request, false)

    await page.goto('/')
    await expect(page.locator('text=Free API Key Available')).toBeHidden()

    await page.goto('/login')
    await expect(page.locator('text=Free API Key Available')).toBeHidden()
  })

  test('card is visible on landing page when enabled', async ({ page, request }) => {
    await setFreeApiKey(request, true)

    await page.goto('/')
    await expect(page.locator('text="Free API Key Available"')).toBeVisible()
    await expect(page.locator('code').filter({ hasText: /^t0-free-rpdb$/ })).toBeVisible()
  })

  test('card is visible on login page when enabled', async ({ page, request }) => {
    await setFreeApiKey(request, true)

    await page.goto('/login')
    await expect(page.locator('text="Free API Key Available"')).toBeVisible()
    await expect(page.locator('code').filter({ hasText: /^t0-free-rpdb$/ })).toBeVisible()
  })

  test('try it out section is visible with form controls', async ({ page, request }) => {
    await setFreeApiKey(request, true)

    await page.goto('/')
    await expect(page.locator('text=Try it out')).toBeVisible()

    // Expand the collapsible section
    await page.locator('text=Try it out').click()

    // ID type select
    const idTypeSelect = page.locator('#free-id-type')
    await expect(idTypeSelect).toBeVisible()
    await expect(idTypeSelect).toContainText('IMDb')

    // Image type select
    const imageTypeSelect = page.locator('#free-image-type')
    await expect(imageTypeSelect).toBeVisible()
    await expect(imageTypeSelect).toContainText('Poster')

    // Image size select
    const imageSizeSelect = page.locator('#free-image-size')
    await expect(imageSizeSelect).toBeVisible()
    await expect(imageSizeSelect).toContainText('Size: default')

    // Language select
    const langSelect = page.locator('#free-lang')
    await expect(langSelect).toBeVisible()
    await expect(langSelect).toContainText('Language: any')

    // ID input pre-filled with Nosferatu
    const idInput = page.locator('#free-id-value')
    await expect(idInput).toBeVisible()
    await expect(idInput).toHaveValue('tt0013442')

    // Fetch button
    await expect(page.locator('button:has-text("Fetch")')).toBeVisible()
  })

  test('curl example updates when form values change', async ({ page, request }) => {
    await setFreeApiKey(request, true)

    await page.goto('/')

    // Expand the collapsible section
    await page.locator('text=Try it out').click()

    // Default curl should contain imdb and poster-default
    const curlBlock = page.locator('code:has-text("curl")')
    await expect(curlBlock).toContainText('imdb/poster-default/tt0013442.jpg')

    // Change ID type to TMDB
    await selectOption(page, page.locator('#free-id-type'), 'TMDb')
    await expect(curlBlock).toContainText('tmdb/poster-default/tt0013442.jpg')

    // Change image type to logo
    await selectOption(page, page.locator('#free-image-type'), 'Logo')
    await expect(curlBlock).toContainText('tmdb/logo-default/tt0013442.png')

    // Change ID value
    const idInput = page.locator('#free-id-value')
    await idInput.clear()
    await idInput.fill('movie-278')
    await expect(curlBlock).toContainText('tmdb/logo-default/movie-278.png')

    // Change image size
    await selectOption(page, page.locator('#free-image-size'), 'Large')
    await expect(curlBlock).toContainText('imageSize=large')

    // Set language
    await selectOption(page, page.locator('#free-lang'), 'en - English')
    await expect(curlBlock).toContainText('lang=en')
  })

  test('fetch poster shows image result', async ({ page, request }) => {
    const token = await setFreeApiKey(request, true)

    if (!(await hasRealKeys(request, token))) {
      test.skip(true, 'Real API keys not configured')
      return
    }

    await page.goto('/')
    await page.locator('text=Try it out').click()

    // Fill in a known IMDB ID
    const idInput = page.locator('#free-id-value')
    await idInput.clear()
    await idInput.fill(TEST_IMDB_ID)

    // Click Fetch
    await page.locator('button:has-text("Fetch")').click()

    // Result image should appear
    const resultImg = page.locator('img[alt="Fetched result"]')
    await expect(resultImg).toBeVisible({ timeout: 60_000 })
  })

  test('fetch with invalid ID shows error', async ({ page, request }) => {
    const token = await setFreeApiKey(request, true)

    if (!(await hasRealKeys(request, token))) {
      test.skip(true, 'Real API keys not configured')
      return
    }

    await page.goto('/')
    await page.locator('text=Try it out').click()

    const idInput = page.locator('#free-id-value')
    await idInput.clear()
    await idInput.fill('tt9999999')

    await page.locator('button:has-text("Fetch")').click()

    // Error message appears (either "Not found" for 404 or "Failed to fetch" for network errors)
    await expect(page.locator('.text-destructive')).toBeVisible({ timeout: 60_000 })
  })

  test('fetch poster on login page works too', async ({ page, request }) => {
    const token = await setFreeApiKey(request, true)

    if (!(await hasRealKeys(request, token))) {
      test.skip(true, 'Real API keys not configured')
      return
    }

    await page.goto('/login')

    // Scope to the free API key card to avoid matching the login form inputs
    const card = page.locator('.border-blue-500\\/30')
    await card.locator('text=Try it out').click()
    const idInput = card.locator('#free-id-value')
    await idInput.clear()
    await idInput.fill(TEST_IMDB_ID)

    await card.locator('button:has-text("Fetch")').click()

    const resultImg = page.locator('img[alt="Fetched result"]')
    await expect(resultImg).toBeVisible({ timeout: 60_000 })
  })

  test('switching image type and fetching shows correct result', async ({ page, request }) => {
    const token = await setFreeApiKey(request, true)

    if (!(await hasRealKeys(request, token))) {
      test.skip(true, 'Real API keys not configured')
      return
    }

    await page.goto('/')
    await page.locator('text=Try it out').click()

    const idInput = page.locator('#free-id-value')
    await idInput.clear()
    await idInput.fill(TEST_IMDB_ID)

    // Fetch poster first
    await page.locator('button:has-text("Fetch")').click()
    const resultImg = page.locator('img[alt="Fetched result"]')
    await expect(resultImg).toBeVisible({ timeout: 60_000 })

    // Switch to backdrop and fetch again
    await selectOption(page, page.locator('#free-image-type'), 'Backdrop')
    await page.locator('button:has-text("Fetch")').click()

    // Image should still be visible (old stays until new loads)
    await expect(resultImg).toBeVisible({ timeout: 60_000 })
  })

  test('card reflects the server ratings order and exposes the settings endpoint', async ({ page, request }) => {
    const token = await getAdminToken(request)
    // Minimal partial update — do not echo the GET payload back (layout fields
    // are JSON strings there but the PUT expects objects, which would 400).
    const updateRes = await request.put('/api/admin/settings', {
      headers: { Authorization: `Bearer ${token}` },
      data: { free_api_key_enabled: true, ratings_order: 'tmdb,imdb,rt' },
    })
    expect(updateRes.status()).toBe(200)

    // The public endpoint returns the operator's configured defaults.
    const pub = await request.get('/api/free-key/settings')
    expect(pub.status()).toBe(200)
    expect((await pub.json()).ratings_order).toBe('tmdb,imdb,rt')

    // The card's priority list reflects that order (TMDB first) — proving the
    // async load + seeding ran end-to-end against the real server.
    await page.goto('/')
    await page.locator('text=Try it out').click()
    const priorityLabels = page.locator('.border-blue-500\\/30 span.flex-1')
    await expect(priorityLabels.first()).toHaveText('TMDB')
  })

  test('settings endpoint returns 401 when the free key is disabled', async ({ request }) => {
    await setFreeApiKey(request, false)
    const res = await request.get('/api/free-key/settings')
    expect(res.status()).toBe(401)
  })

  test('excluding a rating source adds ratings_exclude to the curl example', async ({ page, request }) => {
    const token = await getAdminToken(request)
    const updateRes = await request.put('/api/admin/settings', {
      headers: { Authorization: `Bearer ${token}` },
      data: { free_api_key_enabled: true, ratings_exclude: '' },
    })
    expect(updateRes.status()).toBe(200)

    await page.goto('/')
    await page.locator('text=Try it out').click()

    const curlBlock = page.locator('code:has-text("curl")')
    await expect(curlBlock).not.toContainText('ratings_exclude')

    await page.locator('#free-exclude-rt').click()
    await expect(curlBlock).toContainText('ratings_exclude=rt')
  })
})
