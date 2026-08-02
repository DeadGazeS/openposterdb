export const FREE_API_KEY = 't0-free-rpdb'

export const LANGUAGES = [
  { code: 'ar', name: 'Arabic' },
  { code: 'bg', name: 'Bulgarian' },
  { code: 'ca', name: 'Catalan' },
  { code: 'zh', name: 'Chinese' },
  { code: 'zh-CN', name: 'Chinese (Simplified)' },
  { code: 'zh-TW', name: 'Chinese (Traditional)' },
  { code: 'hr', name: 'Croatian' },
  { code: 'cs', name: 'Czech' },
  { code: 'da', name: 'Danish' },
  { code: 'nl', name: 'Dutch' },
  { code: 'en', name: 'English' },
  { code: 'en-US', name: 'English (US)' },
  { code: 'en-GB', name: 'English (UK)' },
  { code: 'fi', name: 'Finnish' },
  { code: 'fr', name: 'French' },
  { code: 'fr-CA', name: 'French (Canada)' },
  { code: 'de', name: 'German' },
  { code: 'el', name: 'Greek' },
  { code: 'he', name: 'Hebrew' },
  { code: 'hi', name: 'Hindi' },
  { code: 'hu', name: 'Hungarian' },
  { code: 'id', name: 'Indonesian' },
  { code: 'it', name: 'Italian' },
  { code: 'ja', name: 'Japanese' },
  { code: 'ko', name: 'Korean' },
  { code: 'ms', name: 'Malay' },
  { code: 'no', name: 'Norwegian' },
  { code: 'fa', name: 'Persian' },
  { code: 'pl', name: 'Polish' },
  { code: 'pt', name: 'Portuguese' },
  { code: 'pt-BR', name: 'Portuguese (Brazil)' },
  { code: 'pt-PT', name: 'Portuguese (Portugal)' },
  { code: 'ro', name: 'Romanian' },
  { code: 'ru', name: 'Russian' },
  { code: 'sr', name: 'Serbian' },
  { code: 'sk', name: 'Slovak' },
  { code: 'sl', name: 'Slovenian' },
  { code: 'es', name: 'Spanish' },
  { code: 'es-MX', name: 'Spanish (Mexico)' },
  { code: 'es-AR', name: 'Spanish (Argentina)' },
  { code: 'sv', name: 'Swedish' },
  { code: 'th', name: 'Thai' },
  { code: 'tr', name: 'Turkish' },
  { code: 'uk', name: 'Ukrainian' },
  { code: 'vi', name: 'Vietnamese' },
] as const

export const ALL_RATING_SOURCES = [
  { key: 'imdb', label: 'IMDb', color: '#b4910f' },
  { key: 'tmdb', label: 'TMDB', color: '#019b58' },
  { key: 'rt', label: 'Rotten Tomatoes (Critics)', color: '#b92308' },
  { key: 'rta', label: 'Rotten Tomatoes (Audience)', color: '#b92308' },
  { key: 'mc', label: 'Metacritic', color: '#4b9626' },
  { key: 'trakt', label: 'Trakt', color: '#af0f2d' },
  { key: 'lb', label: 'Letterboxd', color: '#009b58' },
  { key: 'mal', label: 'MyAnimeList', color: '#223c78' },
  { key: 'mdblist', label: 'MDBList', color: '#4284CA' },
  { key: 'ebert', label: 'Roger Ebert', color: '#e8590c' },
] as const

export const DEFAULT_RATINGS_ORDER = 'mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert'

/** Sample values used to render a badge for each source in the settings UI.
 * Sources with multiple logos (Rotten Tomatoes) render one badge per variant —
 * the value also picks the logo, matching real behaviour. */
export const SOURCE_BADGE_SAMPLES: Record<string, { value: string; label: string }[]> = {
  imdb: [{ value: '8.7', label: 'IMDb' }],
  tmdb: [{ value: '92%', label: 'TMDB' }],
  rt: [
    { value: '90%', label: 'Critics · Certified Fresh' },
    { value: '65%', label: 'Critics · Fresh' },
    { value: '40%', label: 'Critics · Rotten' },
  ],
  rta: [
    { value: '90%', label: 'Audience · Verified Hot' },
    { value: '65%', label: 'Audience · Fresh' },
    { value: '40%', label: 'Audience · Rotten' },
  ],
  mc: [{ value: '86', label: 'Metacritic' }],
  trakt: [{ value: '97%', label: 'Trakt' }],
  lb: [{ value: '4.1', label: 'Letterboxd' }],
  mal: [{ value: '8.70', label: 'MyAnimeList' }],
  mdblist: [{ value: '95', label: 'MDBList' }],
  ebert: [{ value: '4.0', label: 'Roger Ebert' }],
}

/** The six Rotten Tomatoes logo variants, each with its own badge color
 * settings (keys match the backend `RatingColorKey`). */
export const RT_COLOR_VARIANTS = [
  { key: 'rt_cf', label: 'Rotten Tomatoes (Critics) · Certified Fresh', color: '#b92308' },
  { key: 'rt_pos', label: 'Rotten Tomatoes (Critics) · Fresh', color: '#b92308' },
  { key: 'rt_rot', label: 'Rotten Tomatoes (Critics) · Rotten', color: '#b92308' },
  { key: 'rta_hot', label: 'Rotten Tomatoes (Audience) · Verified Hot', color: '#b92308' },
  { key: 'rta_pos', label: 'Rotten Tomatoes (Audience) · Fresh', color: '#b92308' },
  { key: 'rta_neg', label: 'Rotten Tomatoes (Audience) · Rotten', color: '#b92308' },
] as const

/** Rows for the "Rating Colors" section: the non-Rotten-Tomatoes sources plus
 * one row per Rotten Tomatoes logo variant, in the natural source order. */
export const RATING_COLOR_ROWS = [
  { key: 'imdb', label: 'IMDb', color: '#b4910f' },
  { key: 'tmdb', label: 'TMDB', color: '#019b58' },
  ...RT_COLOR_VARIANTS,
  { key: 'mc', label: 'Metacritic', color: '#4b9626' },
  { key: 'trakt', label: 'Trakt', color: '#af0f2d' },
  { key: 'lb', label: 'Letterboxd', color: '#009b58' },
  { key: 'mal', label: 'MyAnimeList', color: '#223c78' },
  { key: 'mdblist', label: 'MDBList', color: '#4284CA' },
  { key: 'ebert', label: 'Roger Ebert', color: '#e8590c' },
]

// Maps from the short enum codes used by the image query params / settings API
// to human-readable labels. Used to show what a server "default" resolves to in
// the free-key "Try it out" form. `d` (style/direction) means "auto" — resolved
// at render time from the badge position/direction.
export const BADGE_STYLE_LABELS: Record<string, string> = { h: 'Horizontal', v: 'Vertical', d: 'Auto' }
export const BADGE_DIRECTION_LABELS: Record<string, string> = { h: 'Horizontal', v: 'Vertical', d: 'Auto' }
export const LABEL_STYLE_LABELS: Record<string, string> = { t: 'Text', i: 'White', o: 'Official', h: 'High Res' }
export const BADGE_SHAPE_LABELS: Record<string, string> = { r: 'Rounded', p: 'Pill' }
export const BADGE_BACKGROUND_LABELS: Record<string, string> = { d: 'Default', k: 'Dark', t: 'Transparent', n: 'None' }
export const IMAGE_SOURCE_LABELS: Record<string, string> = { t: 'TMDB', f: 'Fanart.tv' }
export const POSTER_FIT_LABELS: Record<string, string> = { native: 'Native', cover: 'Crop to 2:3', blur: 'Blur fill', pad: 'Letterbox' }
export const POSITION_LABELS: Record<string, string> = {
  bc: 'Bottom Center', tc: 'Top Center', l: 'Left', r: 'Right',
  tl: 'Top Left', tr: 'Top Right', bl: 'Bottom Left', br: 'Bottom Right',
}

export function parseRatingsOrder(order: string): string[] {
  const keys = order ? order.split(',').map(k => k.trim()).filter(Boolean) : []
  const allKeys = ALL_RATING_SOURCES.map(s => s.key)
  for (const k of allKeys) {
    if (!keys.includes(k)) keys.push(k)
  }
  return keys
}

/**
 * Parse the comma-separated `ratings_exclude` string into an array of source
 * keys. Unlike {@link parseRatingsOrder}, this does NOT append missing sources —
 * it keeps only the keys the user has explicitly chosen to exclude. Unknown keys
 * are dropped so a stale value can't poison the form.
 */
export function parseRatingsExclude(exclude: string): string[] {
  if (!exclude) return []
  const known = new Set<string>(ALL_RATING_SOURCES.map(s => s.key))
  return exclude.split(',').map(k => k.trim()).filter(k => known.has(k))
}
