<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <th
    :class="cn(
      'h-10 px-4 text-left align-middle font-terminal tracking-terminal text-xs text-tertiary uppercase',
      props.class,
    )"
  >
    <slot />
  </th>
</template>
