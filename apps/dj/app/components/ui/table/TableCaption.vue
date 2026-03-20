<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <caption :class="cn('mt-4 text-xs text-tertiary font-terminal tracking-terminal', props.class)">
    <slot />
  </caption>
</template>
