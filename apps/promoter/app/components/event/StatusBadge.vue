<script setup lang="ts">
import { Ban, CalendarClock, CheckCircle2, History, PauseCircle, PencilLine } from 'lucide-vue-next'
import type { EventStatus } from '~/types/event'

/**
 * Status badge (UX §5). POSTPONED is violet (decision 1) and always carries
 * its own icon and word, so it never differs from DRAFT by colour alone.
 */
const props = defineProps<{ status: EventStatus, past?: boolean }>()

const look = computed(() => {
  if (props.past && props.status !== 'cancelled') return { cls: 'badge-published', label: 'PAST', icon: History }
  return {
    draft: { cls: 'badge-draft', label: 'DRAFT', icon: PencilLine },
    scheduled: { cls: 'badge-scheduled', label: 'SCHEDULED', icon: CalendarClock },
    published: { cls: 'badge-ready', label: 'PUBLISHED', icon: CheckCircle2 },
    cancelled: { cls: 'badge-failed', label: 'CANCELLED', icon: Ban },
    postponed: { cls: 'badge-draft', label: 'POSTPONED', icon: PauseCircle },
  }[props.status]
})
</script>

<template>
  <span class="badge-hud" :class="look.cls" style="display:inline-flex;align-items:center;gap:4px;">
    <component :is="look.icon" style="width:10px;height:10px;" aria-hidden="true" />
    {{ look.label }}
  </span>
</template>
