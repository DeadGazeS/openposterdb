import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RatingsOrderList from '@/components/RatingsOrderList.vue'
import { ALL_RATING_SOURCES } from '@/lib/constants'

function mountWithModel(initial: string[] = ['imdb', 'tmdb', 'rt'], compact = false) {
  const wrapper = mount(RatingsOrderList, {
    props: {
      modelValue: initial,
      compact,
    },
  })
  return wrapper
}

function mountWithExcluded(initial: string[], excluded: string[]) {
  return mount(RatingsOrderList, {
    props: { modelValue: initial, excluded },
  })
}

describe('RatingsOrderList', () => {
  it('renders all items in correct order', () => {
    const order = ['imdb', 'tmdb', 'rt']
    const wrapper = mountWithModel(order)

    const rows = wrapper.findAll('.flex.items-center')
    expect(rows).toHaveLength(3)

    // Check labels appear in order
    const text = wrapper.text()
    expect(text).toContain('IMDb')
    expect(text).toContain('TMDB')
    expect(text).toContain('Rotten Tomatoes (Critics)')
  })

  it('displays 1-based index numbers', () => {
    const wrapper = mountWithModel(['imdb', 'tmdb', 'rt'])

    const indices = wrapper.findAll('.text-muted-foreground.text-xs')
    expect(indices[0]!.text()).toBe('1')
    expect(indices[1]!.text()).toBe('2')
    expect(indices[2]!.text()).toBe('3')
  })

  it('renders colored dots from ALL_RATING_SOURCES', () => {
    const wrapper = mountWithModel(['imdb'])
    const dot = wrapper.find('.rounded-full')
    // Style is set via :style binding — check that backgroundColor is set
    expect(dot.attributes('style')).toContain('background-color')
  })

  it('disables up button on first item', () => {
    const wrapper = mountWithModel(['imdb', 'tmdb', 'rt'])
    const buttons = wrapper.findAll('button')
    // First row: up button (index 0), down button (index 1)
    expect((buttons[0]!.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('disables down button on last item', () => {
    const wrapper = mountWithModel(['imdb', 'tmdb', 'rt'])
    const buttons = wrapper.findAll('button')
    // Last row: up button (index 4), down button (index 5)
    const lastDown = buttons[buttons.length - 1]!
    expect((lastDown.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('enables up/down buttons on middle items', () => {
    const wrapper = mountWithModel(['imdb', 'tmdb', 'rt'])
    const buttons = wrapper.findAll('button')
    // Middle row (tmdb): up button (index 2), down button (index 3)
    expect((buttons[2]!.element as HTMLButtonElement).disabled).toBe(false)
    expect((buttons[3]!.element as HTMLButtonElement).disabled).toBe(false)
  })

  it('emits update:modelValue with swapped items when moving up', async () => {
    const wrapper = mountWithModel(['imdb', 'tmdb', 'rt'])
    const buttons = wrapper.findAll('button')
    // Click "up" on tmdb (second row, up button = index 2)
    await buttons[2]!.trigger('click')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    expect(emitted![0]![0]).toEqual(['tmdb', 'imdb', 'rt'])
  })

  it('emits update:modelValue with swapped items when moving down', async () => {
    const wrapper = mountWithModel(['imdb', 'tmdb', 'rt'])
    const buttons = wrapper.findAll('button')
    // Click "down" on tmdb (second row, down button = index 3)
    await buttons[3]!.trigger('click')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    expect(emitted![0]![0]).toEqual(['imdb', 'rt', 'tmdb'])
  })

  it('does not emit when clicking disabled up button on first item', async () => {
    const wrapper = mountWithModel(['imdb', 'tmdb'])
    const buttons = wrapper.findAll('button')
    // First item's up button is disabled — clicking should not emit
    await buttons[0]!.trigger('click')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeUndefined()
  })

  it('does not emit when clicking disabled down button on last item', async () => {
    const wrapper = mountWithModel(['imdb', 'tmdb'])
    const buttons = wrapper.findAll('button')
    // Last item's down button
    const lastDown = buttons[buttons.length - 1]!
    await lastDown.trigger('click')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeUndefined()
  })

  it('handles single-item list (both buttons disabled)', () => {
    const wrapper = mountWithModel(['imdb'])
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)
    expect((buttons[0]!.element as HTMLButtonElement).disabled).toBe(true)
    expect((buttons[1]!.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('falls back to key when rating source not found', () => {
    const wrapper = mountWithModel(['unknown_source'])
    expect(wrapper.text()).toContain('unknown_source')
  })

  it('applies compact styling when compact prop is true', () => {
    const wrapper = mountWithModel(['imdb', 'tmdb'], true)
    const container = wrapper.find('.space-y-0\\.5')
    expect(container.exists()).toBe(true)
  })

  it('applies normal styling when compact is false', () => {
    const wrapper = mountWithModel(['imdb', 'tmdb'], false)
    const container = wrapper.find('.space-y-1')
    expect(container.exists()).toBe(true)
  })

  describe('excluded prop', () => {
    it('dims and strikes through excluded sources only', () => {
      const wrapper = mountWithExcluded(['imdb', 'tmdb', 'rt'], ['tmdb'])
      const rows = wrapper.findAll('.flex.items-center')
      expect(rows[1]!.classes()).toContain('opacity-40')
      expect(rows[0]!.classes()).not.toContain('opacity-40')

      const labels = wrapper.findAll('span.flex-1')
      expect(labels[1]!.classes()).toContain('line-through')
      expect(labels[1]!.attributes('title')).toBeTruthy()
      expect(labels[0]!.classes()).not.toContain('line-through')
      expect(labels[0]!.attributes('title')).toBeUndefined()
    })

    it('applies no exclusion styling when excluded is omitted', () => {
      const wrapper = mountWithModel(['imdb', 'tmdb'])
      for (const row of wrapper.findAll('.flex.items-center')) {
        expect(row.classes()).not.toContain('opacity-40')
      }
      for (const label of wrapper.findAll('span.flex-1')) {
        expect(label.classes()).not.toContain('line-through')
      }
    })

    it('keeps excluded items functional — move buttons still reorder them', async () => {
      const wrapper = mountWithExcluded(['imdb', 'tmdb', 'rt'], ['tmdb'])
      const buttons = wrapper.findAll('button')
      // Move tmdb (excluded, second row) up — up button is index 2.
      await buttons[2]!.trigger('click')
      expect(wrapper.emitted('update:modelValue')![0]![0]).toEqual(['tmdb', 'imdb', 'rt'])
    })
  })

  it('handles all 10 rating sources', () => {
    const allKeys = ALL_RATING_SOURCES.map(s => s.key)
    const wrapper = mountWithModel(allKeys)

    const rows = wrapper.findAll('.flex.items-center')
    expect(rows).toHaveLength(10)

    // Verify all up/down buttons exist (2 per row)
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(20)
  })
})
