import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import MediaServerConnect from '@/components/MediaServerConnect.vue'

const RAW_KEY = 'opdb_abc123def4567890abcdef1234567890'

function mountConnect(props: Partial<{ apiKey: string; keyPrefix: string }> = {}) {
  return mount(MediaServerConnect, {
    props: {
      apiKey: RAW_KEY,
      keyPrefix: 'opdb_abc',
      ...props,
    },
  })
}

describe('MediaServerConnect', () => {
  beforeEach(() => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('renders AIOMetadata URL templates with the real base URL and API key substituted', () => {
    const wrapper = mountConnect()

    expect(wrapper.text()).toContain(`${window.location.origin}/${RAW_KEY}/imdb/poster-default/{imdb_id}.jpg`)
    expect(wrapper.text()).toContain(`${window.location.origin}/${RAW_KEY}/tmdb/backdrop-default/{type}-{tmdb_id}.jpg?imageSize=large`)
    expect(wrapper.text()).toContain(`${window.location.origin}/${RAW_KEY}/tmdb/logo-default/{type}-{tmdb_id}.png`)
    expect(wrapper.text()).toContain(`${window.location.origin}/${RAW_KEY}/imdb/episode-default/episode-{imdb_id}-S{season}E{episode}.jpg`)
  })

  it('leaves per-media-item placeholders untouched', () => {
    const wrapper = mountConnect()
    expect(wrapper.text()).toContain('{imdb_id}')
    expect(wrapper.text()).toContain('{type}')
    expect(wrapper.text()).toContain('{season}')
    expect(wrapper.text()).toContain('{episode}')
  })

  it('hides the TMDB/TVDB alternate episode-ID table until expanded', async () => {
    const wrapper = mountConnect()

    expect(wrapper.text()).not.toContain('{tvdb_id}')

    const trigger = wrapper.find('button')
    const altTrigger = wrapper.findAll('button').find((b) => b.text().includes('Alternate episode ID formats'))
    expect(altTrigger).toBeDefined()
    await altTrigger!.trigger('click')

    expect(wrapper.text()).toContain(`${window.location.origin}/${RAW_KEY}/tmdb/episode-default/episode-{tmdb_id}-S{season}E{episode}.jpg`)
    expect(wrapper.text()).toContain(`${window.location.origin}/${RAW_KEY}/tvdb/episode-default/episode-{tvdb_id}-S{season}E{episode}.jpg`)
    void trigger
  })

  it('renders the Jellyfin manifest URL and Base URL / API key fields', () => {
    const wrapper = mountConnect()

    expect(wrapper.text()).toContain('https://raw.githubusercontent.com/PNRxA/jellyfin-plugin-openposterdb/main/manifest.json')
    expect(wrapper.text()).toContain(window.location.origin)
    expect(wrapper.text()).toContain(RAW_KEY)
  })

  it('renders Plex provider URLs with the real API key and calls out the fixed hosted domain', () => {
    const wrapper = mountConnect()

    expect(wrapper.text()).toContain(`https://plex.openposterdb.com/${RAW_KEY}/movie`)
    expect(wrapper.text()).toContain(`https://plex.openposterdb.com/${RAW_KEY}/tv`)
    expect(wrapper.text()).toContain('not')
    expect(wrapper.text()).toContain(`https://plex.openposterdb.com/${RAW_KEY}/movie~badge_style=v&ratings_limit=5`)
  })

  it('re-renders all templated URLs when the Base URL input is edited', async () => {
    const wrapper = mountConnect()

    const input = wrapper.find('#connect-base-url')
    await input.setValue('https://example.com')

    expect(wrapper.text()).toContain(`https://example.com/${RAW_KEY}/imdb/poster-default/{imdb_id}.jpg`)
    // Plex URLs are unaffected by the base URL — they use the fixed hosted domain.
    expect(wrapper.text()).toContain(`https://plex.openposterdb.com/${RAW_KEY}/movie`)
  })

  it('shows a legacy-key banner and leaves {api_key} as a literal placeholder when apiKey is empty', () => {
    const wrapper = mountConnect({ apiKey: '' })

    expect(wrapper.find('[data-testid="connect-legacy-banner"]').exists()).toBe(true)
    expect(wrapper.text()).toContain(`${window.location.origin}/{api_key}/imdb/poster-default/{imdb_id}.jpg`)
  })

  it('copies a table row URL to the clipboard and flashes feedback', async () => {
    const wrapper = mountConnect()

    const copyButtons = wrapper.findAll('button[title="Copy Poster URL"]')
    expect(copyButtons.length).toBe(1)
    await copyButtons[0]!.trigger('click')

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      `${window.location.origin}/${RAW_KEY}/imdb/poster-default/{imdb_id}.jpg`,
    )
  })
})
