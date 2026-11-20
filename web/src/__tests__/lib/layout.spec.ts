import { describe, it, expect } from 'vitest'
import { parseLayout } from '@/lib/layout'

describe('parseLayout', () => {
  // The server sends unused sides as Go zero values (start ""). Kept as-is,
  // turning such a side on saved start "" and the server rejected it with
  // "invalid top start position ''".
  it('replaces an empty start on an unused side with a valid one', () => {
    const l = parseLayout(
      {
        top: { per_row: 0, rows: 0, start: '' },
        right: { per_row: 0, rows: 0, start: '' },
        bottom: { per_row: 3, rows: 1, start: 'c' },
        left: { per_row: 0, rows: 0, start: '' },
        order: ['bottom', 'top', 'left', 'right'],
      },
      'poster',
    )
    expect(l.top.start).toBe('c')
    expect(l.right.start).toBe('c')
    expect(l.left.start).toBe('c')
  })

  it('replaces a start that is invalid for the side', () => {
    const l = parseLayout({ top: { per_row: 2, rows: 1, start: 't' }, right: { per_row: 1, rows: 1, start: 'l' } }, 'poster')
    expect(l.top.start).toBe('c')
    expect(l.right.start).toBe('c')
  })

  it('keeps valid starts', () => {
    const l = parseLayout(
      {
        top: { per_row: 5, rows: 1, start: 'r' },
        right: { per_row: 1, rows: 1, start: 'b' },
        bottom: { per_row: 3, rows: 1, start: 'l' },
        left: { per_row: 1, rows: 1, start: 't' },
      },
      'poster',
    )
    expect([l.top.start, l.right.start, l.bottom.start, l.left.start]).toEqual(['r', 'b', 'l', 't'])
  })
})
