// useRenderSettingsForm — shared settings form helpers for the two views
// that drive RenderSettingsForm (SettingsView via adminApi, KeySettingsView
// via selfApi). The component already owns UI state (dirty/saving/error);
// this composable only owns the standard load/save/reset wiring: ok/!ok
// check, error-string parsing, and optional hooks for per-view side
// effects (e.g. onLoad to store the payload in a local ref, onSave to
// merge a transient flag into the payload before POST).
//
// Returns the same `loadSettings`/`saveSettings`/`resetSettings` shape
// RenderSettingsForm already accepts as props — no component change needed.

import type { RenderSettings } from '@/lib/settings'

export interface SettingsFormApi {
  load(): Promise<Response>
  save(payload: unknown): Promise<Response>
  reset?: () => Promise<Response>
}

export interface SettingsFormOptions<T = unknown> {
  api: SettingsFormApi
  /** Side effect after a successful load (e.g. mirror to a local ref). */
  onLoad?: (data: RenderSettings) => void
  /** Mutate the payload before POST (e.g. merge free_api_key_enabled). */
  onSave?: (payload: T) => T | Promise<T>
  /** Error string returned when the response body has no `.error` field. */
  saveErrorMessage?: string
}

export interface SettingsFormHelpers<T = unknown> {
  loadSettings: () => Promise<RenderSettings | null>
  saveSettings: (payload: T) => Promise<string | null>
  resetSettings: () => Promise<boolean>
}

export function useRenderSettingsForm<T = unknown>(
  opts: SettingsFormOptions<T>,
): SettingsFormHelpers<T> {
  return {
    async loadSettings(): Promise<RenderSettings | null> {
      try {
        const res = await opts.api.load()
        if (!res.ok) return null
        const data = (await res.json()) as RenderSettings
        opts.onLoad?.(data)
        return data
      } catch {
        return null
      }
    },
    async saveSettings(payload: T): Promise<string | null> {
      try {
        const finalPayload = opts.onSave ? await opts.onSave(payload) : payload
        const res = await opts.api.save(finalPayload)
        if (res.ok) return null
        const data = await res.json().catch(() => null)
        return (data?.error as string | undefined) || opts.saveErrorMessage || 'Failed to save'
      } catch {
        return opts.saveErrorMessage || 'Failed to save'
      }
    },
    async resetSettings(): Promise<boolean> {
      if (!opts.api.reset) return false
      const res = await opts.api.reset()
      return res.ok
    },
  }
}
