<script setup lang="ts">
import { CheckCircle2, Clock, Hourglass, Mail, XCircle } from 'lucide-vue-next'
import type { GuestStatus } from '~/types/guest'
import { STATUS_LABEL } from '~/utils/guests'

/** Guest status: word + icon + tone, never colour alone. */
const props = defineProps<{ status: GuestStatus }>()
const look = computed(() => ({
  going: { cls: 'badge-ready', icon: CheckCircle2 },
  pending: { cls: 'badge-draft', icon: Clock },
  waitlist: { cls: 'badge-archived', icon: Hourglass },
  invited: { cls: 'badge-published', icon: Mail },
  declined: { cls: 'badge-published', icon: XCircle },
}[props.status]))
</script>

<template>
  <span class="badge-hud" :class="look.cls" style="display:inline-flex;align-items:center;gap:4px;white-space:nowrap;">
    <component :is="look.icon" style="width:10px;height:10px;" aria-hidden="true" />
    {{ STATUS_LABEL[status] }}
  </span>
</template>
