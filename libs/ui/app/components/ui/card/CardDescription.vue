<script setup lang="ts">
import { cn } from '#kui/lib/utils'
interface Props { class?: string }
const props = defineProps<Props>()
</script>
<template>
  <p :class="cn('text-xs text-tertiary font-data', props.class)"><slot /></p>
</template>
