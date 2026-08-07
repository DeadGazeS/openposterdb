import { describe, it, expect, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import AdminListView from '@/views/AdminListView.vue'
import ImageListView from '@/components/ImageListView.vue'
import { adminApi, imageListConfig } from '@/lib/api'

// AdminListView replaces the 4 per-kind stubs (Posters/Logos/Backdrops/
// Episodes) with a single kind-driven wrapper. The route supplies the
// kind prop; this spec covers the wiring for every supported kind.

vi.mock('@/lib/api', () => ({
  adminApi: {
    getPosters: vi.fn(),
    getPosterImage: vi.fn(),
    fetchPoster: vi.fn(),
    purgePoster: vi.fn(),
    clearPosters: vi.fn(),
    getLogos: vi.fn(),
    getLogoImage: vi.fn(),
    fetchLogo: vi.fn(),
    purgeLogo: vi.fn(),
    clearLogos: vi.fn(),
    getBackdrops: vi.fn(),
    getBackdropImage: vi.fn(),
    fetchBackdrop: vi.fn(),
    purgeBackdrop: vi.fn(),
    clearBackdrops: vi.fn(),
    getEpisodes: vi.fn(),
    getEpisodeImage: vi.fn(),
    fetchEpisode: vi.fn(),
    purgeEpisode: vi.fn(),
    clearEpisodes: vi.fn(),
  },
  imageListConfig: vi.fn(),
}))

const mockImageListConfig = vi.mocked(imageListConfig)

const kinds = ['poster', 'logo', 'backdrop', 'episode'] as const

describe('AdminListView', () => {
  for (const kind of kinds) {
    it(`renders ImageListView with kind="${kind}"`, () => {
      const wrapper = shallowMount(AdminListView, { props: { kind } })

      const imageList = wrapper.findComponent(ImageListView)
      expect(imageList.exists()).toBe(true)
      expect(imageList.props('kind')).toBe(kind)
    })

    it(`forwards the right adminApi functions for kind="${kind}"`, () => {
      mockImageListConfig.mockReturnValue({
        listFn: adminApi[`get${kind.charAt(0).toUpperCase() + kind.slice(1)}s` as keyof typeof adminApi] as never,
        imageFn: adminApi[`get${kind.charAt(0).toUpperCase() + kind.slice(1)}Image` as keyof typeof adminApi] as never,
        fetchFn: adminApi[`fetch${kind.charAt(0).toUpperCase() + kind.slice(1)}` as keyof typeof adminApi] as never,
        deleteFn: adminApi[`purge${kind.charAt(0).toUpperCase() + kind.slice(1)}` as keyof typeof adminApi] as never,
        clearAllFn: adminApi[`clear${kind.charAt(0).toUpperCase() + kind.slice(1)}s` as keyof typeof adminApi] as never,
      })

      const wrapper = shallowMount(AdminListView, { props: { kind } })

      const imageList = wrapper.findComponent(ImageListView)
      expect(imageList.props('listFn')).toBe(adminApi[`get${kind.charAt(0).toUpperCase() + kind.slice(1)}s` as keyof typeof adminApi])
      expect(imageList.props('imageFn')).toBe(adminApi[`get${kind.charAt(0).toUpperCase() + kind.slice(1)}Image` as keyof typeof adminApi])
      expect(imageList.props('fetchFn')).toBe(adminApi[`fetch${kind.charAt(0).toUpperCase() + kind.slice(1)}` as keyof typeof adminApi])
      expect(imageList.props('deleteFn')).toBe(adminApi[`purge${kind.charAt(0).toUpperCase() + kind.slice(1)}` as keyof typeof adminApi])
      expect(imageList.props('clearAllFn')).toBe(adminApi[`clear${kind.charAt(0).toUpperCase() + kind.slice(1)}s` as keyof typeof adminApi])

      expect(mockImageListConfig).toHaveBeenCalledWith(kind)
    })
  }
})
