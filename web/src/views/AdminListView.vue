<script setup lang="ts">
import ImageListView from '@/components/ImageListView.vue'
import { imageListConfig, type ImageListKind } from '@/lib/api'

// AdminListView replaces the 4 per-kind stub views (Posters/Logos/
// Backdrops/Episodes). The kind prop is supplied by the router so the
// admin gallery is driven from a single component + a single config
// table in api.ts (`imageListConfig`).
defineProps<{ kind: ImageListKind }>()
</script>

<template>
  <ImageListView
    :kind="kind"
    v-bind="imageListConfig(kind)"
  />
</template>
