// Dashboard tile layout: order, hidden tiles and column count. Stored per
// admin user in the server-side prefs (key `dashboard_layout`, JSON string).
// Pure functions so the merge / move / sort rules are unit-tested.

export type DashboardColumns = 2 | 3 | 4

export interface DashboardLayout {
  order: string[]
  hidden: string[]
  columns: DashboardColumns
}

export const DASHBOARD_LAYOUT_PREF = 'dashboard_layout'
export const DEFAULT_COLUMNS: DashboardColumns = 3
const VALID_COLUMNS: DashboardColumns[] = [2, 3, 4]

type Tile = { key: string; label: string }

/** Tile keys sorted A–Z by label. */
export function sortedAZ(tiles: readonly Tile[], keys?: readonly string[]): string[] {
  const label = new Map(tiles.map((t) => [t.key, t.label]))
  return [...(keys ?? tiles.map((t) => t.key))].sort((a, b) =>
    (label.get(a) ?? a).localeCompare(label.get(b) ?? b, undefined, { sensitivity: 'base' }),
  )
}

export function defaultLayout(tiles: readonly Tile[]): DashboardLayout {
  return { order: sortedAZ(tiles), hidden: [], columns: DEFAULT_COLUMNS }
}

/**
 * Turns a stored value (possibly from an older version, or garbage) into a
 * layout for the current tiles: unknown keys are dropped, tiles added since
 * the layout was saved are appended (A–Z among themselves), an invalid
 * column count falls back to the default.
 */
export function parseLayout(raw: string | undefined | null, tiles: readonly Tile[]): DashboardLayout {
  const fallback = defaultLayout(tiles)
  if (!raw) return fallback
  let data: unknown
  try {
    data = JSON.parse(raw)
  } catch {
    return fallback
  }
  if (!data || typeof data !== 'object') return fallback
  const d = data as Partial<Record<keyof DashboardLayout, unknown>>
  const known = new Set(tiles.map((t) => t.key))
  const strings = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [])

  const order = [...new Set(strings(d.order))].filter((k) => known.has(k))
  const missing = sortedAZ(tiles, tiles.map((t) => t.key).filter((k) => !order.includes(k)))
  const columns = VALID_COLUMNS.includes(d.columns as DashboardColumns) ? (d.columns as DashboardColumns) : DEFAULT_COLUMNS
  return {
    order: [...order, ...missing],
    hidden: [...new Set(strings(d.hidden))].filter((k) => known.has(k)),
    columns,
  }
}

export function serializeLayout(layout: DashboardLayout): string {
  return JSON.stringify(layout)
}

/** Moves `key` to the position currently held by `target` (drag & drop). */
export function moveTo(order: readonly string[], key: string, target: string): string[] {
  const from = order.indexOf(key)
  const to = order.indexOf(target)
  if (from < 0 || to < 0 || from === to) return [...order]
  const next = [...order]
  next.splice(from, 1)
  next.splice(to, 0, key)
  return next
}

/** Moves `key` one place earlier (-1) or later (+1); no-op at the ends. */
export function moveBy(order: readonly string[], key: string, delta: -1 | 1): string[] {
  const from = order.indexOf(key)
  const to = from + delta
  if (from < 0 || to < 0 || to >= order.length) return [...order]
  return moveTo(order, key, order[to]!)
}

export function toggleHidden(hidden: readonly string[], key: string): string[] {
  return hidden.includes(key) ? hidden.filter((k) => k !== key) : [...hidden, key]
}
