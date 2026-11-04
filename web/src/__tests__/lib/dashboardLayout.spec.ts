import { describe, it, expect } from 'vitest'
import {
  defaultLayout,
  moveBy,
  moveTo,
  parseLayout,
  serializeLayout,
  sortedAZ,
  toggleHidden,
} from '@/lib/dashboardLayout'

const tiles = [
  { key: 'total_images', label: 'Total Images' },
  { key: 'uptime', label: 'Uptime' },
  { key: 'api_keys', label: 'API Keys' },
  { key: 'database', label: 'Database (MB)' },
]

describe('dashboardLayout', () => {
  it('defaults to A–Z, everything visible, three columns', () => {
    expect(defaultLayout(tiles)).toEqual({
      order: ['api_keys', 'database', 'total_images', 'uptime'],
      hidden: [],
      columns: 3,
    })
  })

  it('round-trips a saved layout', () => {
    const layout = { order: ['uptime', 'api_keys', 'database', 'total_images'], hidden: ['database'], columns: 4 as const }
    expect(parseLayout(serializeLayout(layout), tiles)).toEqual(layout)
  })

  it('drops unknown tiles and appends new ones A–Z', () => {
    const raw = JSON.stringify({ order: ['uptime', 'removed_tile', 'uptime'], hidden: ['removed_tile', 'api_keys'], columns: 2 })
    expect(parseLayout(raw, tiles)).toEqual({
      order: ['uptime', 'api_keys', 'database', 'total_images'],
      hidden: ['api_keys'],
      columns: 2,
    })
  })

  it('falls back to the default for missing or broken values', () => {
    for (const raw of [undefined, '', 'not json', '42', 'null', JSON.stringify({ columns: 7 })]) {
      expect(parseLayout(raw, tiles)).toEqual(defaultLayout(tiles))
    }
  })

  it('moves tiles by drag target and by one step', () => {
    const order = ['a', 'b', 'c', 'd']
    expect(moveTo(order, 'a', 'c')).toEqual(['b', 'c', 'a', 'd'])
    expect(moveTo(order, 'd', 'b')).toEqual(['a', 'd', 'b', 'c'])
    expect(moveBy(order, 'b', -1)).toEqual(['b', 'a', 'c', 'd'])
    expect(moveBy(order, 'a', -1)).toEqual(order)
    expect(moveBy(order, 'd', 1)).toEqual(order)
  })

  it('toggles a hidden tile and sorts any subset A–Z', () => {
    expect(toggleHidden(['a'], 'b')).toEqual(['a', 'b'])
    expect(toggleHidden(['a', 'b'], 'a')).toEqual(['b'])
    expect(sortedAZ(tiles, ['uptime', 'api_keys'])).toEqual(['api_keys', 'uptime'])
  })
})
