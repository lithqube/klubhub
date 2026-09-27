<script setup lang="ts">
import { Ban, CheckCircle2, Clock, Undo2 } from 'lucide-vue-next'
import type { TicketStatus } from '~/types/guest'
import { TICKET_STATUS_LABEL } from '~/utils/attendeeImport'

/** Imported ticket status: word + icon + tone, never colour alone. */
const props = defineProps<{ status: TicketStatus }>()
const look = computed(() => ({
  valid: { cls: 'badge-ready', icon: CheckCircle2 },
  pending: { cls: 'badge-draft', icon: Clock },
  cancelled: { cls: 'badge-published', icon: Ban },
  refunded: { cls: 'badge-archived', icon: Undo2 },
}[props.status]))
</script>

<template>
  <span class="badge-hud" :class="look.cls" style="display:inline-flex;align-items:center;gap:4px;white-space:nowrap;">
    <component :is="look.icon" style="width:10px;height:10px;" aria-hidden="true" />
    {{ TICKET_STATUS_LABEL[status] }}
  </span>
</template>
