<script setup lang="ts">
import { computed } from 'vue'
import {
  RELEASE_STATUS_LABELS,
  RELEASE_STATUS_ORDER,
  type ReleaseStatus,
} from '~/types/release'

// The workflow rail of REL-02. A release moves one step forward or one step
// back; 'released' is terminal. Only the reachable steps are offered.
const props = defineProps<{
  status: ReleaseStatus
  busy?: boolean
}>()

const emit = defineEmits<{ change: [status: ReleaseStatus] }>()

const currentIndex = computed(() => RELEASE_STATUS_ORDER.indexOf(props.status))

function canMoveTo(index: number): boolean {
  if (props.busy) return false
  const distance = index - currentIndex.value
  return distance === 1 || distance === -1
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-1" data-testid="release-status-stepper">
    <button
      v-for="(step, index) in RELEASE_STATUS_ORDER"
      :key="step"
      type="button"
      class="px-2 py-1 text-[9px] uppercase tracking-terminal border transition-colors"
      :class="[
        index === currentIndex
          ? 'border-primary text-primary'
          : 'border-outline text-tertiary',
        canMoveTo(index) ? 'hover:text-primary hover:border-primary cursor-pointer' : 'cursor-default',
      ]"
      :disabled="!canMoveTo(index)"
      :aria-current="index === currentIndex ? 'step' : undefined"
      @click="canMoveTo(index) && emit('change', step)"
    >
      {{ RELEASE_STATUS_LABELS[step] }}
    </button>
  </div>
</template>
