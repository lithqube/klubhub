<script setup lang="ts">
import { cn } from '#kui/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <tr
    :class="cn(
      'transition-colors duration-100 border-b border-outline-variant/15',
      'hover:bg-primary/[0.02]',
      'data-[selected=true]:row-selected',
      props.class,
    )"
  >
    <slot />
  </tr>
</template>
