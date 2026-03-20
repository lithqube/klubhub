<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <thead :class="cn('bg-surface-container-lowest', props.class)">
    <slot />
  </thead>
</template>
