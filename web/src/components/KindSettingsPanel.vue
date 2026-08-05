<script setup lang="ts">
import { computed } from 'vue'
import { Loader2 } from 'lucide-vue-next'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import LayoutEditor from '@/components/LayoutEditor.vue'
import { POSTER_FIT_DESCRIPTIONS } from '@/lib/constants'
import type { ImageLayout } from '@/lib/layout'

interface PreviewState {
  src: string
  loading: boolean
  error: boolean
  size: { w: number; h: number } | null
  generation: number
}

const props = defineProps<{
  kind: 'poster' | 'logo' | 'backdrop' | 'episode'
  title: string
  preview: PreviewState
  uid?: string
}>()

const editLayout = defineModel<ImageLayout>('editLayout', { required: true })
const badgeStyle = defineModel<string>('badgeStyle', { required: true })
const labelStyle = defineModel<string>('labelStyle', { required: true })
const shape = defineModel<string>('shape', { required: true })
const alpha = defineModel<number>('alpha', { required: true })
const textSize = defineModel<number>('textSize', { required: true })
const badgeWidth = defineModel<number>('badgeWidth', { required: true })
const badgeHeight = defineModel<number>('badgeHeight', { required: true })
const logoSize = defineModel<number>('logoSize', { required: true })
const fit = defineModel<string>('fit', { default: 'native' })
const textless = defineModel<boolean>('textless', { default: false })
const edgeInsetX = defineModel<number>('edgeInsetX', { default: 0 })
const edgeInsetY = defineModel<number>('edgeInsetY', { default: 0 })
const blur = defineModel<boolean>('blur', { default: false })

const inputId = (name: string) => (props.uid ? `${name}-${props.uid}` : name)

const previewStyle = computed(() =>
  props.preview.size
    ? { aspectRatio: `${props.preview.size.w} / ${props.preview.size.h}` }
    : undefined,
)

function onPreviewLoad(e: Event) {
  const img = e.target as HTMLImageElement
  if (img.naturalWidth && img.naturalHeight) {
    props.preview.size = { w: img.naturalWidth, h: img.naturalHeight }
  }
  props.preview.loading = false
  props.preview.error = false
}

function gcd(a: number, b: number): number {
  return b === 0 ? a : gcd(b, a % b)
}

function aspectRatioLabel(size: { w: number; h: number }): string {
  const { w, h } = size
  if (!w || !h) return '—'
  const d = gcd(w, h)
  return `${w / d}:${h / d} · ${w}×${h}`
}
</script>

<template>
  <div class="rounded-md border p-4 space-y-3">
    <p class="text-sm font-semibold">{{ title }}</p>
    <div class="flex flex-col md:flex-row gap-4 items-start">
      <div class="flex flex-col items-center gap-1.5 shrink-0">
        <div class="relative w-[320px] shrink-0" :style="previewStyle">
          <img
            v-show="preview.src && !preview.error"
            :src="preview.src"
            :alt="`${title} preview`"
            class="rounded border w-full"
            :class="kind === 'logo' ? 'bg-neutral-900' : undefined"
            @load="onPreviewLoad"
            @error="preview.loading = false; preview.error = true"
          />
          <p v-if="preview.error && !preview.loading" class="text-sm text-muted-foreground py-4">Failed</p>
          <div v-if="preview.loading" class="absolute inset-0 flex items-center justify-center rounded">
            <Loader2 class="size-5 animate-spin text-white drop-shadow-md" />
          </div>
        </div>
        <p v-if="preview.size" class="text-xs text-muted-foreground tabular-nums" :data-testid="`${kind}-aspect-ratio`">
          {{ aspectRatioLabel(preview.size) }}
        </p>
      </div>
      <LayoutEditor v-model="editLayout" :kind="kind" :test-prefix="kind" class="flex-1 min-w-0" />
    </div>
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-lg">
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 sm:col-span-2">
        <div class="space-y-2 min-w-0">
          <Label :for="inputId(`${kind}-badge-style`)">Badge style</Label>
          <Select v-model="badgeStyle" :disabled="shape === 'p'">
            <SelectTrigger :id="inputId(`${kind}-badge-style`)" class="min-w-0 w-full" :data-testid="`${kind}-badge-style-select`">
              <SelectValue placeholder="Select style" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="lr">Logo left · Number right</SelectItem>
              <SelectItem value="rl">Number left · logo right</SelectItem>
              <SelectItem value="tb">Logo top · Number bottom</SelectItem>
              <SelectItem value="bt">Number top · logo bottom</SelectItem>
            </SelectContent>
          </Select>
          <p v-if="shape === 'p'" class="text-xs text-muted-foreground">Pills always render horizontally.</p>
        </div>
        <div class="space-y-2 min-w-0">
          <Label :for="inputId(`${kind}-label-style`)">Label style</Label>
          <Select v-model="labelStyle">
            <SelectTrigger :id="inputId(`${kind}-label-style`)" class="min-w-0 w-full" :data-testid="`${kind}-label-style-select`">
              <SelectValue placeholder="Select style" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="t">Text</SelectItem>
              <SelectItem value="i">White</SelectItem>
              <SelectItem value="o">Official</SelectItem>
              <SelectItem value="h">High Res</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2 min-w-0">
          <Label :for="inputId(`${kind}-badge-shape`)">Badge shape</Label>
          <Select v-model="shape">
            <SelectTrigger :id="inputId(`${kind}-badge-shape`)" class="min-w-0 w-full" :data-testid="`${kind}-badge-shape-select`">
              <SelectValue placeholder="Select shape" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="r">Rounded</SelectItem>
              <SelectItem value="p">Pill</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <div class="space-y-2">
        <div class="flex items-center gap-2">
          <Label :for="inputId(`${kind}-badge-width`)">Badge width</Label>
          <span class="text-xs text-muted-foreground w-10 text-right">{{ badgeWidth }}%</span>
        </div>
        <input
          :id="inputId(`${kind}-badge-width`)"
          v-model.number="badgeWidth"
          type="range"
          min="50"
          max="400"
          step="1"
          :data-testid="`${kind}-badge-width-slider`"
          class="w-full max-w-xs accent-primary"
        />
        <p class="text-xs text-muted-foreground">Badge width. 100% = default.</p>
      </div>
      <div class="space-y-2">
        <div class="flex items-center gap-2">
          <Label :for="inputId(`${kind}-badge-height`)">Badge height</Label>
          <span class="text-xs text-muted-foreground w-10 text-right">{{ badgeHeight }}%</span>
        </div>
        <input
          :id="inputId(`${kind}-badge-height`)"
          v-model.number="badgeHeight"
          type="range"
          min="50"
          max="400"
          step="1"
          :data-testid="`${kind}-badge-height-slider`"
          class="w-full max-w-xs accent-primary"
        />
        <p class="text-xs text-muted-foreground">Badge height. 100% = default.</p>
      </div>
      <div class="space-y-2">
        <div class="flex items-center gap-2">
          <Label :for="inputId(`${kind}-text-size`)">Text size</Label>
          <span class="text-xs text-muted-foreground w-10 text-right">{{ textSize }}%</span>
        </div>
        <input
          :id="inputId(`${kind}-text-size`)"
          v-model.number="textSize"
          type="range"
          min="50"
          max="400"
          step="1"
          :data-testid="`${kind}-text-size-slider`"
          class="w-full max-w-xs accent-primary"
        />
        <p class="text-xs text-muted-foreground">Rating text font size. 100% = default.</p>
      </div>
      <div class="space-y-2">
        <div class="flex items-center gap-2">
          <Label :for="inputId(`${kind}-logo-size`)">Logo size</Label>
          <span class="text-xs text-muted-foreground w-10 text-right">{{ logoSize }}%</span>
        </div>
        <input
          :id="inputId(`${kind}-logo-size`)"
          v-model.number="logoSize"
          type="range"
          min="50"
          max="400"
          step="1"
          :data-testid="`${kind}-logo-size-slider`"
          class="w-full max-w-xs accent-primary"
        />
        <p class="text-xs text-muted-foreground">Rating source logo size. The badge auto-sizes to fit, so nothing overflows.</p>
      </div>
      <div class="space-y-2">
        <div class="flex items-center gap-2">
          <Label :for="inputId(`${kind}-badge-alpha`)">Background opacity</Label>
          <span class="text-xs text-muted-foreground w-8 text-right">{{ alpha }}%</span>
        </div>
        <input
          :id="inputId(`${kind}-badge-alpha`)"
          v-model.number="alpha"
          type="range"
          min="0"
          max="100"
          step="5"
          :data-testid="`${kind}-badge-alpha-slider`"
          class="w-full max-w-xs accent-primary"
        />
        <p class="text-xs text-muted-foreground">0% = no background, 100% = opaque black</p>
      </div>
      <div v-if="kind === 'poster'" class="space-y-2">
        <Label :for="inputId('poster-fit')">Fit</Label>
        <Select v-model="fit">
          <SelectTrigger :id="inputId('poster-fit')" class="max-w-xs" data-testid="poster-fit-select">
            <SelectValue placeholder="Select fit" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="native">Native (source ratio)</SelectItem>
            <SelectItem value="cover">Crop to 2:3</SelectItem>
            <SelectItem value="blur">Blur fill to 2:3</SelectItem>
            <SelectItem value="pad">Letterbox to 2:3</SelectItem>
          </SelectContent>
        </Select>
        <p class="text-xs text-muted-foreground" data-testid="poster-fit-description">
          {{ POSTER_FIT_DESCRIPTIONS[fit] ?? 'How non-2:3 posters are fit to the standard 2:3 frame so clients don\'t crop them.' }}
        </p>
      </div>
      <div v-if="kind === 'poster'" class="flex items-center gap-2">
        <Checkbox v-model="textless" :id="inputId('textless')" data-testid="textless-checkbox" />
        <Label :for="inputId('textless')">Prefer textless posters</Label>
      </div>
      <div v-if="kind === 'backdrop'" class="space-y-2">
        <div class="flex items-center gap-4">
          <div class="space-y-1">
            <Label :for="inputId('backdrop-edge-inset-y')" class="text-xs font-normal text-muted-foreground">Space from edge</Label>
            <div class="flex items-center gap-1">
              <Input
                :id="inputId('backdrop-edge-inset-y')"
                v-model.number="edgeInsetY"
                type="number"
                :min="0"
                :max="50"
                class="w-[80px]"
                data-testid="backdrop-edge-inset-y"
              />
              <span class="text-xs text-muted-foreground">%</span>
            </div>
          </div>
          <div class="space-y-1">
            <Label :for="inputId('backdrop-edge-inset-x')" class="text-xs font-normal text-muted-foreground">Space from edge</Label>
            <div class="flex items-center gap-1">
              <Input
                :id="inputId('backdrop-edge-inset-x')"
                v-model.number="edgeInsetX"
                type="number"
                :min="0"
                :max="50"
                class="w-[80px]"
                data-testid="backdrop-edge-inset-x"
              />
              <span class="text-xs text-muted-foreground">%</span>
            </div>
          </div>
        </div>
        <p class="text-xs text-muted-foreground">Inset ratings from the image edge (% of size) — useful when a player crops the backdrop.</p>
      </div>
      <div v-if="kind === 'episode'" class="flex items-center gap-2">
        <Checkbox v-model="blur" :id="inputId('episode-blur')" data-testid="episode-blur-checkbox" />
        <Label :for="inputId('episode-blur')">Blur (spoiler protection)</Label>
      </div>
    </div>
  </div>
</template>
