<script setup lang="ts">
import type { TabsContentProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { TabsContent, useForwardProps } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<TabsContentProps & { class?: HTMLAttributes["class"] }>()

const delegatedProps = reactiveOmit(props, "class")
const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <TabsContent
    v-bind="forwardedProps"
    :class="
      cn(
        'ring-offset-background focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2',
        props.class,
      )
    "
  >
    <slot />
  </TabsContent>
</template>
