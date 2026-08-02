export interface SideSlot {
  per_row: number
  rows: number
  start: string
}

export interface ImageLayout {
  top: SideSlot
  right: SideSlot
  bottom: SideSlot
  left: SideSlot
  order: string[]
}

export const DEFAULT_FILL_ORDER = ['bottom', 'top', 'left', 'right']

export function emptySlot(start = 'c'): SideSlot {
  return { per_row: 0, rows: 0, start }
}

export function defaultLayout(kind: string): ImageLayout {
  const order = [...DEFAULT_FILL_ORDER]
  switch (kind) {
    case 'poster':
      return { top: emptySlot(), right: emptySlot(), bottom: { per_row: 3, rows: 1, start: 'c' }, left: emptySlot(), order }
    case 'logo':
      return { top: emptySlot(), right: emptySlot(), bottom: { per_row: 5, rows: 1, start: 'c' }, left: emptySlot(), order }
    case 'backdrop':
      return { top: { per_row: 5, rows: 1, start: 'r' }, right: emptySlot(), bottom: emptySlot(), left: emptySlot(), order }
    case 'episode':
      return { top: emptySlot(), right: { per_row: 1, rows: 1, start: 't' }, bottom: emptySlot(), left: emptySlot(), order }
    default:
      return { top: emptySlot(), right: emptySlot(), bottom: emptySlot(), left: emptySlot(), order }
  }
}

export function layoutTotal(layout: ImageLayout): number {
  return (
    (layout.top?.per_row ?? 0) * (layout.top?.rows ?? 0) +
    (layout.right?.per_row ?? 0) * (layout.right?.rows ?? 0) +
    (layout.bottom?.per_row ?? 0) * (layout.bottom?.rows ?? 0) +
    (layout.left?.per_row ?? 0) * (layout.left?.rows ?? 0)
  )
}

export function parseLayout(raw: unknown, kind: string): ImageLayout {
  if (!raw) return defaultLayout(kind)
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return normalizeLayout(parsed, kind)
    } catch {
      return defaultLayout(kind)
    }
  }
  return normalizeLayout(raw, kind)
}

function normalizeLayout(l: unknown, kind: string): ImageLayout {
  const def = defaultLayout(kind)
  if (!l || typeof l !== 'object') return def
  const o = l as Record<string, unknown>
  return {
    top: normalizeSlot(o.top, def.top),
    right: normalizeSlot(o.right, def.right),
    bottom: normalizeSlot(o.bottom, def.bottom),
    left: normalizeSlot(o.left, def.left),
    order: Array.isArray(o.order) && (o.order as unknown[]).length ? (o.order as string[]) : [...DEFAULT_FILL_ORDER],
  }
}

function normalizeSlot(raw: unknown, def: SideSlot): SideSlot {
  if (!raw || typeof raw !== 'object') return { ...def }
  const o = raw as Record<string, unknown>
  const perRow = typeof o.per_row === 'number' ? o.per_row : def.per_row
  const rows = typeof o.rows === 'number' ? o.rows : def.rows
  return { per_row: perRow, rows, start: typeof o.start === 'string' ? o.start : def.start }
}

export function layoutToJSON(layout: ImageLayout): string {
  return JSON.stringify(layout)
}
