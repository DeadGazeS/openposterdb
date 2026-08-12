import type { ClassValue } from "clsx"
import { clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Extract the logical title id from a rendered cache value.
 *
 * A cache value is `{idValue}{variant}{suffix}`, where `variant` (when present)
 * starts with `_` and `suffix` always starts with `@`. Legitimate title ids
 * never contain `_` or `@`, so the title id is everything up to the first such
 * delimiter — e.g. `tt123_t_de@imc` and `movie-99@i` yield `tt123` / `movie-99`.
 */
export function titleIdFromCacheValue(cacheValue: string): string {
  const match = cacheValue.match(/^[^_@]*/)
  return match && match[0] ? match[0] : cacheValue
}

/**
 * maskKey mirrors the backend MaskKey at
 * api-go/internal/services/servicekeys.go: show first min(len/4, 4) +
 * "..." + last min(len/4, 4) chars. Always hides at least half of the key
 * (for keys ≥ 8 chars). For keys < 8 chars, hides what it can and falls back
 * to "****" for empty/undefined input. Used by both source API keys and
 * api_keys chips so both surfaces obfuscate identically.
 *
 * Defensive against `undefined`/`null`/empty: when the backend omits a field
 * (e.g. legacy api_keys rows where encrypted_key is null), JS receives
 * `undefined` and the chip would otherwise throw at .length and crash the
 * surrounding render (Vue falls back to a <!----> placeholder). Masking as
 * "****" is the right behavior — there's nothing to reveal anyway.
 */
export function maskKey(key: string | null | undefined): string {
  if (!key) return '****'
  let n = Math.floor(key.length / 4)
  if (n > 4) n = 4
  if (n === 0) return '****'
  return key.slice(0, n) + '...' + key.slice(-n)
}
