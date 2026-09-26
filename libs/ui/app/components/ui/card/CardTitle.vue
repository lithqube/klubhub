<script setup lang="ts">
import { cn } from '#kui/lib/utils'
interface Props { class?: string }
const props = defineProps<Props>()
</script>
<template>
  <h3 :class="cn('font-command tracking-command text-sm font-bold text-on-surface uppercase', props.class)"><slot /></h3>
</template>
