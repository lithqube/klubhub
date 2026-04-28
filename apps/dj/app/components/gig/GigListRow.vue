<script setup lang="ts">
import type { Gig } from '../../types/gig'
import GigStatusBadge from './GigStatusBadge.vue'

const props = defineProps<{
  gig: Gig
}>()

const emit = defineEmits<{
  edit: [gig: Gig]
}>()

const accentClass = computed(() => {
  switch (props.gig.status) {
    case 'confirmed':
      return 'accent-bar-ready'
    case 'inquiry':
    case 'advanced':
      return 'accent-bar-draft'
    case 'played':
    case 'cancelled':
      return 'accent-bar-archived'
    default:
      return 'accent-bar-archived'
  }
})

function formatDate(dateStr: string) {
  const date = new Date(dateStr)
  const mon = date.toLocaleDateString('en-US', { month: 'short' }).toUpperCase()
  const day = date.getDate()
  return { mon, day }
}

function formatCurrency(amount: number, currency: string): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: currency || 'EUR',
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(amount)
}

const { mon, day } = formatDate(props.gig.date)
</script>

<template>
  <div
    class="gig-list-row"
    :class="accentClass"
    style="cursor:pointer;"
    @click="emit('edit', gig)"
  >
    <!-- Date block -->
    <div class="gig-date-block">
      <div class="gig-date-mon" :style="{ color: 'var(--color-primary)' }">
        {{ mon }}
      </div>
      <div class="gig-date-day">{{ day }}</div>
    </div>

    <!-- Info -->
    <div style="flex:1;min-width:0;">
      <div class="gig-venue">{{ gig.venue || gig.event_name }}</div>
      <div class="gig-meta">
        <span class="section-lbl">
          {{ [gig.city, gig.country].filter(Boolean).join(', ') }}
        </span>
      </div>
    </div>

    <!-- Fee -->
    <div
      class="gig-fee"
      :style="{
        color:
          gig.status === 'confirmed'
            ? 'var(--color-primary)'
            : gig.status === 'advanced'
              ? 'var(--color-secondary)'
              : 'var(--color-tertiary)',
      }"
    >
      {{ formatCurrency(gig.fee_amount, gig.fee_currency) }}
    </div>

    <!-- Status badge -->
    <GigStatusBadge :status="gig.status" />
  </div>
</template>
