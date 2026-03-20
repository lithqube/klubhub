<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <td :class="cn('px-4 py-2 align-middle text-sm text-on-surface-variant', props.class)">
    <slot />
  </td>
</template>
