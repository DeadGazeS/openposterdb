import type { ImageLayout } from '@/lib/layout'

/** Per-source color overrides. Empty string means "use the default". Accent and
 * value are the badge section colors (default black); border and text round out
 * the badge. The badge alpha slider controls shared background opacity. */
export interface SourceColors {
  accent?: string
  value?: string
  border?: string
  text?: string
}

/**
 * Canonical render-settings shape as returned by `GET /api/admin/settings`
 * (layouts serialized as JSON strings, defaults applied). Also the base for
 * the save payload and the free-key defaults.
 *
 * Layout fields are typed as `string | ImageLayout` because the per-key GET
 * response (`/api/key/{id}/settings`, `/api/key/me/settings`) actually
 * returns parsed `ImageLayout` objects, while the global GET returns
 * JSON-stringified layouts (the legacy wire format). Runtime callers go
 * through `parseLayout` (`web/src/lib/layout.ts`) which accepts both shapes
 * (#10.7). Save payloads (SaveSettingsPayload) always use the object form.
 */
export interface RenderSettings {
  image_source: string
  lang: string
  textless: boolean
  fanart_available: boolean
  is_default?: boolean
  ratings_limit: number
  ratings_order: string
  ratings_exclude: string
  poster_layout: string | ImageLayout
  logo_ratings_limit: number
  backdrop_ratings_limit: number
  poster_badge_style: string
  logo_badge_style: string
  backdrop_badge_style: string
  poster_label_style: string
  logo_label_style: string
  backdrop_label_style: string
  poster_badge_direction: string
  poster_fit: string
  poster_text_size: number
  logo_text_size: number
  backdrop_text_size: number
  poster_badge_size: number
  logo_badge_size: number
  backdrop_badge_size: number
  poster_badge_width: number
  poster_badge_height: number
  logo_badge_width: number
  logo_badge_height: number
  backdrop_badge_width: number
  backdrop_badge_height: number
  episode_badge_width: number
  episode_badge_height: number
  poster_logo_size: number
  logo_logo_size: number
  backdrop_logo_size: number
  logo_layout: string | ImageLayout
  backdrop_layout: string | ImageLayout
  backdrop_badge_direction: string
  backdrop_edge_inset_x: number
  backdrop_edge_inset_y: number
  episode_ratings_limit: number
  episode_badge_style: string
  episode_label_style: string
  episode_text_size: number
  episode_badge_size: number
  episode_logo_size: number
  episode_layout: string | ImageLayout
  episode_badge_direction: string
  episode_blur: boolean
  poster_badge_shape: string
  logo_badge_shape: string
  backdrop_badge_shape: string
  episode_badge_shape: string
  poster_badge_alpha: number
  logo_badge_alpha: number
  backdrop_badge_alpha: number
  episode_badge_alpha: number
  colors?: Record<string, SourceColors>
}

/**
 * PUT payload for the settings endpoints: layouts are sent as objects, and the
 * optional fields (ratings limits, badge sizes/directions) keep the stored
 * value on the backend when omitted.
 */
export type SaveSettingsPayload = Omit<
  RenderSettings,
  | 'fanart_available'
  | 'is_default'
  | 'ratings_limit'
  | 'logo_ratings_limit'
  | 'backdrop_ratings_limit'
  | 'episode_ratings_limit'
  | 'poster_layout'
  | 'logo_layout'
  | 'backdrop_layout'
  | 'episode_layout'
  | 'poster_badge_direction'
  | 'backdrop_badge_direction'
  | 'episode_badge_direction'
  | 'poster_badge_size'
  | 'logo_badge_size'
  | 'backdrop_badge_size'
  | 'episode_badge_size'
  | 'poster_badge_width'
  | 'poster_badge_height'
  | 'logo_badge_width'
  | 'logo_badge_height'
  | 'backdrop_badge_width'
  | 'backdrop_badge_height'
  | 'episode_badge_width'
  | 'episode_badge_height'
  | 'colors'
> & {
  ratings_limit?: number
  logo_ratings_limit?: number
  backdrop_ratings_limit?: number
  episode_ratings_limit?: number
  poster_layout: ImageLayout
  logo_layout: ImageLayout
  backdrop_layout: ImageLayout
  episode_layout: ImageLayout
  poster_badge_direction?: string
  backdrop_badge_direction?: string
  episode_badge_direction?: string
  poster_badge_size?: number
  logo_badge_size?: number
  backdrop_badge_size?: number
  episode_badge_size?: number
  poster_badge_width?: number
  poster_badge_height?: number
  logo_badge_width?: number
  logo_badge_height?: number
  backdrop_badge_width?: number
  backdrop_badge_height?: number
  episode_badge_width?: number
  episode_badge_height?: number
  colors?: Record<string, SourceColors>
}

/** Defaults served with the free API key (`GET /api/free-key/settings`). */
export type FreeKeyDefaults = Omit<
  RenderSettings,
  | 'fanart_available'
  | 'is_default'
  | 'colors'
  | 'poster_badge_width'
  | 'poster_badge_height'
  | 'logo_badge_width'
  | 'logo_badge_height'
  | 'backdrop_badge_width'
  | 'backdrop_badge_height'
  | 'episode_badge_width'
  | 'episode_badge_height'
>

/** The four image kinds the preview endpoints render. */
export type PreviewKind = 'poster' | 'logo' | 'backdrop' | 'episode'

/** Query parameters for the preview endpoints. Fields that do not apply to a
 * kind are ignored by the backend. */
export interface PreviewParams {
  ratingsLimit: number
  ratingsOrder: string
  ratingsExclude?: string
  badgeStyle?: string
  labelStyle?: string
  badgeDirection?: string
  textSize?: number
  layout?: string
  badgeShape?: string
  badgeAlpha?: number
  posterFit?: string
  badgeSize?: number
  logoSize?: number
  badgeWidth?: number
  badgeHeight?: number
  edgeInsetX?: number
  edgeInsetY?: number
  blur?: boolean
  colors?: Record<string, SourceColors>
}
