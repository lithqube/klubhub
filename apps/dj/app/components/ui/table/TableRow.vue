<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <!-- Alternating rows via CSS: odd=surface-container, even=surface-container-low. No border-b dividers. -->
  <tr
    :class="cn(
      'transition-colors duration-100',
      'odd:bg-surface-container even:bg-surface-container-low',
      'hover:bg-surface-bright',
      'data-[selected=true]:row-selected',
      props.class,
    )"
  >
    <slot />
  </tr>
</template>
