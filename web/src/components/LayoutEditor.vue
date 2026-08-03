<script setup lang="ts">
import { computed } from 'vue'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ImageLayout, SideSlot } from '@/lib/layout'
import { layoutTotal } from '@/lib/layout'

const props = withDefaults(defineProps<{
  modelValue: ImageLayout
  kind: string
  testPrefix?: string
}>(), {
  testPrefix: '',
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: ImageLayout): void
}>()

const total = computed(() => layoutTotal(props.modelValue))

function setSlot(side: 'top' | 'right' | 'bottom' | 'left', slot: SideSlot) {
  emit('update:modelValue', { ...props.modelValue, [side]: slot })
}

function setField(side: 'top' | 'right' | 'bottom' | 'left', field: 'per_row' | 'rows' | 'start', value: number | string) {
  const cur = { ...props.modelValue[side] }
  ;(cur as Record<string, unknown>)[field] = value
  setSlot(side, cur)
}

function slotValue(side: 'top' | 'right' | 'bottom' | 'left'): SideSlot {
  return props.modelValue[side]
}

// start anchor options depend on the side axis.
const horizontalStartOptions = [
  { value: 'l', label: 'Left' },
  { value: 'c', label: 'Centre' },
  { value: 'r', label: 'Right' },
]
const verticalStartOptions = [
  { value: 't', label: 'Top' },
  { value: 'c', label: 'Centre' },
  { value: 'b', label: 'Bottom' },
]

function startLabel(side: string): string {
  return side === 'top' || side === 'bottom' ? 'Start (left/centre/right)' : 'Start (top/centre/bottom)'
}

function sideLabel(side: string): string {
  return side.charAt(0).toUpperCase() + side.slice(1)
}
</script>

<template>
  <div class="space-y-3" :data-testid="testPrefix ? `${testPrefix}-layout-editor` : undefined">
    <p class="text-xs text-muted-foreground">
      {{ total }} ratings shown. Set badges-per-row and row count for each side.
    </p>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <template v-for="side in ['top', 'bottom', 'left', 'right'] as const" :key="side">
        <div class="rounded-md border p-3 space-y-2">
          <p class="text-sm font-medium">{{ sideLabel(side) }}</p>
          <div class="grid grid-cols-2 gap-2">
            <div class="space-y-1">
              <Label :for="`${side}-per-row`">Badges per row</Label>
              <Input
                :id="`${side}-per-row`"
                :model-value="slotValue(side).per_row"
                type="number"
                min="0"
                max="10"
                class="w-full"
                :data-testid="`${testPrefix}-${side}-per-row`"
                @update:model-value="(v) => setField(side, 'per_row', v)"
              />
            </div>
            <div class="space-y-1">
              <Label :for="`${side}-rows`">Rows</Label>
              <Input
                :id="`${side}-rows`"
                :model-value="slotValue(side).rows"
                type="number"
                min="0"
                max="10"
                class="w-full"
                :data-testid="`${testPrefix}-${side}-rows`"
                @update:model-value="(v) => setField(side, 'rows', v)"
              />
            </div>
          </div>
          <div class="space-y-1">
            <Label :for="`${side}-start`">{{ startLabel(side) }}</Label>
            <Select
              :model-value="slotValue(side).start"
              @update:model-value="(v) => setField(side, 'start', String(v))"
            >
              <SelectTrigger :id="`${side}-start`" class="w-full max-w-xs" :data-testid="`${testPrefix}-${side}-start`">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="opt in (side === 'top' || side === 'bottom' ? horizontalStartOptions : verticalStartOptions)"
                  :key="opt.value"
                  :value="opt.value"
                >
                  {{ opt.label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
