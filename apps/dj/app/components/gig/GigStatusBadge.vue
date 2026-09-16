<script setup lang="ts">
import type { GigStatus } from '../../types/gig'

const props = defineProps<{
  status: GigStatus
}>()

const badgeClass = computed(() => {
  switch (props.status) {
    case 'confirmed':
      return 'badge-ready'
    case 'inquiry':
    case 'advanced':
      return 'badge-draft'
    case 'played':
      return 'badge-archived'
    case 'cancelled':
      return 'badge-failed'
    default:
      return 'badge-archived'
  }
})

const badgeLabel = computed(() => {
  switch (props.status) {
    case 'confirmed':
      return 'CONFIRMED'
    case 'inquiry':
      return 'INQUIRY'
    case 'advanced':
      return 'ADVANCED'
    case 'played':
      return 'PAST'
    case 'cancelled':
      return 'CANCELLED'
    default:
      return props.status.toUpperCase()
  }
})
</script>

<template>
  <span class="badge-hud" :class="badgeClass">{{ badgeLabel }}</span>
</template>
