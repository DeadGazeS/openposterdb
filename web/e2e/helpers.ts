import type { APIRequestContext, Locator, Page } from '@playwright/test'

/** Helper to select a value in a shadcn-vue Select component by its trigger locator. */
export async function selectOption(page: Page, trigger: Locator, optionName: string) {
  await trigger.click()
  await page.getByRole('option', { name: optionName, exact: true }).click()
}

/**
 * Reset the global render settings that the settings specs depend on to their
 * server defaults. Runs as a partial PUT, so unrelated settings (e.g. the free
 * API key flag) are left untouched. The layout fields are sent as objects (the
 * admin PUT expects objects — the GET payload returns them as JSON strings).
 */
export async function resetGlobalSettings(request: APIRequestContext, token: string): Promise<void> {
  const res = await request.put('/api/admin/settings', {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      image_source: 't',
      lang: 'en',
      textless: false,
      ratings_order: 'mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert',
      ratings_exclude: '',
      poster_layout: { bottom: { per_row: 3, rows: 1, start: 'c' } },
      logo_layout: { bottom: { per_row: 5, rows: 1, start: 'c' } },
      backdrop_layout: { top: { per_row: 5, rows: 1, start: 'r' } },
      episode_layout: { right: { per_row: 1, rows: 1, start: 't' } },
      poster_label_style: 'o',
      logo_label_style: 'o',
      backdrop_label_style: 'o',
      episode_label_style: 'o',
      poster_fit: 'native',
      poster_badge_shape: 'r',
      logo_badge_shape: 'r',
      backdrop_badge_shape: 'r',
      episode_badge_shape: 'r',
    },
  })
  if (!res.ok()) throw new Error(`Failed to reset global settings: ${res.status()}`)
}
