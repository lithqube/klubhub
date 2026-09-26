<script setup lang="ts">
import { cn } from '#kui/lib/utils'

interface Props {
  class?: string
  elevated?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  elevated: false,
})
</script>

<template>
  <div :class="cn('glass-panel rounded-none text-on-surface', elevated && 'luminous-threshold', props.class)">
    <slot />
  </div>
</template>
