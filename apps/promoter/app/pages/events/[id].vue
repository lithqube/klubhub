<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import type { ApiError, EventStatus } from '~/types/event'
import { dayLabel, rangeLabel, zoneAbbrev } from '~/utils/datetime'

const route = useRoute()
const store = useEventStore()
const { current, error } = storeToRefs(store)
const id = computed(() => String(route.params.id))
await useAsyncData(() => `event-${id.value}`, () => store.fetchEvent(id.value), { watch: [id] })
useHead(() => ({ title: current.value?.title ?? 'Event' }))

const tabs = [
  { to: 'details', label: 'DETAILS' }, { to: 'lineup', label: 'LINEUP & TIMETABLE' }, { to: 'export', label: 'EXPORT' },
]

const past = computed(() => !!current.value && Date.parse(current.value.ends_at) <= Date.now())
const actions = computed<{ to: EventStatus, label: string, danger?: boolean }[]>(() => {
  const s = current.value?.status
  if (!s || past.value) return []
  return ({
    draft: [{ to: 'published', label: 'PUBLISH NOW' }, { to: 'scheduled', label: 'SCHEDULE' }, { to: 'cancelled', label: 'CANCEL', danger: true }],
    scheduled: [{ to: 'published', label: 'PUBLISH NOW' }, { to: 'draft', label: 'BACK TO DRAFT' }, { to: 'postponed', label: 'POSTPONE' }, { to: 'cancelled', label: 'CANCEL', danger: true }],
    published: [{ to: 'postponed', label: 'POSTPONE' }, { to: 'cancelled', label: 'CANCEL', danger: true }],
    postponed: [{ to: 'published', label: 'REPUBLISH' }, { to: 'cancelled', label: 'CANCEL', danger: true }],
    cancelled: [],
  } as Record<EventStatus, { to: EventStatus, label: string, danger?: boolean }[]>)[s]
})

const statusError = ref<string | null>(null)
async function change(to: EventStatus) {
  if (!current.value) return
  if (to === 'cancelled' && !window.confirm(`Cancel ${current.value.title}? This can't be undone.`)) return
  statusError.value = null
  try {
    await store.changeStatus(current.value.id, current.value.version, to)
  } catch (e) {
    const err = e as ApiError
    statusError.value = ({
      timetable_errors: 'The timetable has problems. Fix them in LINEUP & TIMETABLE, then publish.',
      publish_at_required: 'Set a future GO LIVE date under RELEASE to schedule.',
      version_conflict: 'Someone else changed this event. Reload to see their changes.',
      bad_transition: 'That status change is not possible from here.',
    } as Record<string, string>)[err.error] ?? 'Could not change the status.'
  }
}
</script>

<template>
  <div v-if="current">
    <div class="page-header" style="flex-wrap:wrap;gap:10px;">
      <div style="min-width:0;">
        <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;">
          <h1 class="page-title" style="margin:0;">{{ current.title }}</h1>
          <EventStatusBadge :status="current.status" :past="past" />
          <span class="data-frag" style="font-size:8px;">{{ current.visibility.toUpperCase() }}</span>
        </div>
        <div class="page-sub">
          {{ dayLabel(current.starts_at, current.timezone) }} · {{ rangeLabel(current.starts_at, current.ends_at, current.timezone) }} ·
          {{ current.timezone.toUpperCase() }} ({{ zoneAbbrev(current.starts_at, current.timezone) }})
        </div>
      </div>
      <div style="display:flex;flex-wrap:wrap;gap:6px;">
        <button
          v-for="a in actions" :key="a.to" type="button" class="btn-hud"
          :class="a.danger ? 'btn-hud-ghost' : a.to === 'published' ? 'btn-hud-cta' : 'btn-hud-ghost'"
          :style="a.danger ? 'color:var(--color-error);min-height:44px;' : 'min-height:44px;'"
          @click="change(a.to)"
        >
          {{ a.label }}
        </button>
      </div>
    </div>
    <div class="page-body space-y-3">
      <p v-if="statusError" role="alert" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-error);font-size:13px;">
        {{ statusError }}
        <NuxtLink v-if="statusError.includes('LINEUP')" :to="`/events/${current.id}/lineup`" style="color:var(--color-primary);">Open timetable →</NuxtLink>
      </p>
      <nav class="tabs-bar" aria-label="Event sections">
        <NuxtLink
          v-for="t in tabs" :key="t.to" :to="`/events/${current.id}/${t.to}`" class="tab-item"
          active-class="active" style="min-height:44px;display:inline-flex;align-items:center;"
        >
          {{ t.label }}
        </NuxtLink>
      </nav>
      <NuxtPage />
    </div>
  </div>
  <div v-else class="page-body">
    <KhEmptyState
      :title="error?.status === 404 ? 'EVENT NOT FOUND' : 'COULD NOT LOAD THIS EVENT'"
      hint="It may have been removed, or it belongs to another collective." action-label="ALL EVENTS" action-to="/events"
    />
  </div>
</template>
