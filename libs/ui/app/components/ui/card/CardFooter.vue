<script setup lang="ts">
import { cn } from '#kui/lib/utils'
interface Props { class?: string }
const props = defineProps<Props>()
</script>
<template>
  <div :class="cn('flex items-center p-4 pt-0', props.class)"><slot /></div>
</template>
