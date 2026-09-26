<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '#kui/lib/utils'

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
    <div
      class="h-full rounded-none transition-all duration-300 progress-bar-fill"
      :style="{ width: `${percentage}%` }"
    />
  </div>
</template>
