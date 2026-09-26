<script setup lang="ts">
import { cn } from '#kui/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <tbody :class="cn(props.class)">
    <slot />
  </tbody>
</template>
