<script setup lang="ts">
import { Eye, EyeOff } from 'lucide-vue-next'
import { ALL_RATING_SOURCES } from '@/lib/constants'

const model = defineModel<string[]>({ required: true })

const props = withDefaults(defineProps<{
  compact?: boolean
  /** Source keys the user has excluded — shown dimmed/struck-through, not removed. */
  excluded?: string[]
  /**
   * Renders a per-row show/hide eye. When enabled, clicking a row's eye emits
   * `toggle-exclude` with the source key and the parent is expected to update
   * `excluded` accordingly. Left off by consumers that manage exclusion
   * through their own controls.
   */
  toggleable?: boolean
}>(), {
  toggleable: false,
})

const emit = defineEmits<{
  'toggle-exclude': [key: string]
}>()

function isExcluded(key: string) {
  return props.excluded?.includes(key) ?? false
}

function moveItem(from: number, to: number) {
  if (to < 0 || to >= model.value.length) return
  const arr = [...model.value]
  const removed = arr.splice(from, 1)
  arr.splice(to, 0, ...removed)
  model.value = arr
}

function getRatingSource(key: string) {
  return ALL_RATING_SOURCES.find(s => s.key === key)
}
</script>

<template>
  <div :class="compact ? 'space-y-0.5' : 'space-y-1'" class="max-w-sm">
    <div
      v-for="(key, index) in model"
      :key="key"
      class="flex items-center gap-2 rounded border bg-background"
      :class="[compact ? 'px-1.5 py-1' : 'px-2 py-1.5', { 'opacity-40': isExcluded(key) }]"
    >
      <span class="text-muted-foreground text-xs select-none w-4 text-right">{{ index + 1 }}</span>
      <span
        class="inline-block w-2.5 h-2.5 rounded-full shrink-0"
        :style="{ backgroundColor: getRatingSource(key)?.color }"
      ></span>
      <span
        class="text-sm flex-1"
        :class="{ 'line-through': isExcluded(key) }"
      >{{ getRatingSource(key)?.label || key }}</span>
      <button
        v-if="toggleable"
        type="button"
        :data-testid="`exclude-${key}-eye`"
        :title="isExcluded(key) ? 'Show rating' : 'Hide rating'"
        :aria-label="`${isExcluded(key) ? 'Show' : 'Hide'} ${getRatingSource(key)?.label || key}`"
        :aria-pressed="isExcluded(key)"
        class="inline-flex items-center justify-center rounded border text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        :class="compact ? 'w-6 h-6 text-xs' : 'w-8 h-8'"
        @click="emit('toggle-exclude', key)"
      >
        <Eye v-if="!isExcluded(key)" class="size-4" aria-hidden="true" />
        <EyeOff v-else class="size-4" aria-hidden="true" />
      </button>
      <button
        type="button"
        class="inline-flex items-center justify-center rounded border text-muted-foreground hover:text-foreground hover:bg-muted disabled:opacity-30 disabled:pointer-events-none"
        :class="compact ? 'w-6 h-6 text-xs' : 'w-8 h-8'"
        :disabled="index === 0"
        @click="moveItem(index, index - 1)"
        title="Move up"
      >&uarr;</button>
      <button
        type="button"
        class="inline-flex items-center justify-center rounded border text-muted-foreground hover:text-foreground hover:bg-muted disabled:opacity-30 disabled:pointer-events-none"
        :class="compact ? 'w-6 h-6 text-xs' : 'w-8 h-8'"
        :disabled="index === model.length - 1"
        @click="moveItem(index, index + 1)"
        title="Move down"
      >&darr;</button>
    </div>
  </div>
</template>
