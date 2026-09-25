<script setup lang="ts">
import { cn } from '#kui/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <tfoot :class="cn('bg-surface-container font-medium text-on-surface', props.class)">
    <slot />
  </tfoot>
</template>
