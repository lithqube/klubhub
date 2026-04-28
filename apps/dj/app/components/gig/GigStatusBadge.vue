<script setup lang="ts">
import type { GigStatus } from '../../types/gig'

const props = defineProps<{
  status: GigStatus
}>()

const badgeClass = computed(() => {
  switch (props.status) {
    case 'confirmed':
    case 'inquiry':
      return 'badge-ready'
    case 'advanced':
      return 'badge-draft'
    case 'played':
    case 'cancelled':
      return 'badge-archived'
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
