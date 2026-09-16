<script setup lang="ts">
import { cn } from '@/lib/utils'

import { computed } from 'vue'

interface Props {
  modelValue?: number
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: 0,
})

const percentage = computed(() => Math.min(100, Math.max(0, props.modelValue ?? 0)))
</script>

<template>
  <!-- Track -->
  <div
    :class="cn('relative h-1 w-full overflow-hidden rounded-none bg-surface-container-high', props.class)"
    role="progressbar"
    :aria-valuenow="percentage"
    aria-valuemin="0"
    aria-valuemax="100"
  >
    <!-- Fill — cyan gradient matching mockup -->
    <div
      class="h-full rounded-none transition-all duration-300"
      :style="{
        width: `${percentage}%`,
        background: 'linear-gradient(90deg, #96F8FF, #00F1FD)',
      }"
    />
  </div>
</template>
