<script setup lang="ts">
import { useEventStore } from '~/stores/event'
import { pickNow } from '~/utils/attention'

// Backstage shortcut (bottom nav NOW): the running night's timetable, else
// the next one within 7 days, else the events list.
const events = useEventStore()
await events.fetchList('upcoming')
const target = pickNow(events.lists.upcoming)
await navigateTo(target ? `/events/${target.id}/lineup` : '/events', { replace: true })
</script>

<template>
  <p role="status" class="data-frag" style="padding:24px;font-size:9px;">FINDING TONIGHT…</p>
</template>
