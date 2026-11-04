import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import LayoutEditor from '@/components/LayoutEditor.vue'
import type { ImageLayout } from '@/lib/layout'

const layout = (): ImageLayout =>
  ({
    top: { per_row: 0, rows: 0, start: 'c' },
    right: { per_row: 0, rows: 0, start: 'c' },
    bottom: { per_row: 3, rows: 1, start: 'c' },
    left: { per_row: 0, rows: 0, start: 'c' },
    order: ['bottom'],
  }) as unknown as ImageLayout

describe('LayoutEditor', () => {
  // Two editors are mounted at once (one per kind tab); their label/input
  // ids used to collide, so a label focused the other editor's input.
  it('gives each instance unique input ids', () => {
    const Two = defineComponent({
      setup() {
        const a = ref(layout())
        const b = ref(layout())
        return () => [
          h(LayoutEditor, { modelValue: a.value, kind: 'poster', testPrefix: 'poster' }),
          h(LayoutEditor, { modelValue: b.value, kind: 'logo', testPrefix: 'logo' }),
        ]
      },
    })
    const w = mount(Two)
    const ids = w.findAll('input').map((i) => i.attributes('id'))
    expect(ids.length).toBeGreaterThan(0)
    expect(new Set(ids).size).toBe(ids.length)
  })

  // An emptied number box used to store '' for an int field, and the save
  // then failed with a 400.
  it('stores whole numbers in 0-10 for emptied or out-of-range boxes', async () => {
    const model = ref(layout())
    const w = mount(LayoutEditor, {
      props: { modelValue: model.value, kind: 'poster', testPrefix: 'poster', 'onUpdate:modelValue': (v: ImageLayout) => (model.value = v) },
    })
    await w.find('[data-testid="poster-bottom-per-row"]').setValue('')
    expect(model.value.bottom.per_row).toBe(0)
    await w.find('[data-testid="poster-bottom-rows"]').setValue('42')
    expect(model.value.bottom.rows).toBe(10)
  })
})
