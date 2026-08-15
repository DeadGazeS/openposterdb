// Per-id-type example values for the Fetch modals in the image admin and
// free-key card. Each id system has its own value format:
//
//   imdb  → "tt1234567" (7-digit zero-padded numeric, lowercase "tt" prefix)
//   tmdb  → "movie-27205" / "series-1399" / "episode-1399-S1E1"
//   tvdb  → "81189" (bare numeric)
//   kitsu → "1" or "fairy-tail-2018" (numeric id OR Kitsu slug)
//   mal   → "1" (bare numeric)
//
// Rendered into the ID Value input's placeholder so the admin sees a
// format-appropriate example per selection. Mirrored across ImageListView.vue
// (admin Fetch modal) and FreeApiKeyCard.vue (free-key preview form) —
// keep the two in sync if you add a new id type.
export interface IdTypeExample {
  value: string
  label: string
}

const EXAMPLES: Record<string, IdTypeExample> = {
  imdb: { value: 'tt1234567', label: 'IMDb id' },
  tmdb: { value: 'movie-27205', label: 'TMDb id (movie-…, series-…, episode-…)' },
  tvdb: { value: '81189', label: 'TVDb id (numeric)' },
  kitsu: { value: '1 or fairy-tail-2018', label: 'Kitsu anime id (numeric or slug)' },
  mal: { value: '1', label: 'MAL anime id (numeric)' },
}

export const DEFAULT_ID_EXAMPLE: IdTypeExample = {
  value: 'tt1234567',
  label: 'id value',
}

// idTypeExample returns the placeholder example for the given id_type
// (the lowercase string form used in URL paths — "imdb", "tmdb", "tvdb",
// "kitsu", "mal"). Falls back to DEFAULT_ID_EXAMPLE for unknown types so
// the input never shows a bare "value" placeholder.
export function idTypeExample(idType: string): IdTypeExample {
  return EXAMPLES[idType] ?? DEFAULT_ID_EXAMPLE
}

// The full set of supported id types. Order matters — it drives the
// dropdown order in both Fetch modals (ImageListView.vue admin +
// FreeApiKeyCard.vue preview).
export const ALL_ID_TYPES = ['imdb', 'tmdb', 'tvdb', 'kitsu', 'mal'] as const

// Kitsu/MAL resolvers carry poster + backdrop artwork only (no logo or
// per-episode stills). Hide them from the dropdown when the requested kind
// isn't supported — otherwise the user picks kitsu/mal for a logo, hits Fetch,
// and gets the new "Kitsu / MAL don't carry logo artwork" error. Better to
// just not show the option.
const ID_TYPES_WITHOUT_KITSU_MAL = new Set(['logo', 'episode'])

// availableIdTypesForKind returns the id-type keys the dropdown should
// expose for the given image kind. Use this as the v-for source in both
// Fetch modals (ImageListView.vue + FreeApiKeyCard.vue).
export function availableIdTypesForKind(kind: string): readonly string[] {
  if (ID_TYPES_WITHOUT_KITSU_MAL.has(kind)) {
    return ['imdb', 'tmdb', 'tvdb']
  }
  return ALL_ID_TYPES
}

// idTypeLabel returns the human-readable label for an id-type key. The
// Fetch modals iterate over availableIdTypesForKind and pair each key
// with idTypeLabel for the SelectItem display.
export function idTypeLabel(idType: string): string {
  switch (idType) {
    case 'imdb':  return 'IMDb'
    case 'tmdb':  return 'TMDb'
    case 'tvdb':  return 'TVDB'
    case 'kitsu': return 'Kitsu'
    case 'mal':   return 'MAL'
  }
  return idType
}
