<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { CalendarRange } from 'lucide-vue-next'
import { useEventStore } from '~/stores/event'
import { useOrgStore } from '~/stores/org'
import { dayLabel } from '~/utils/datetime'

useHead({ title: 'Dashboard' })

const orgStore = useOrgStore()
const { org } = storeToRefs(orgStore)
await callOnce('promoter-org', () => orgStore.fetchOrg())

const events = useEventStore()
const { lists, loading, error } = storeToRefs(events)
const { items, load } = useAttention()
await useAsyncData('dashboard-events', () => load())

// Minute clock for the countdown; SSR renders with the request time.
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  timer = setInterval(() => (now.value = Date.now()), 60_000)
})
onBeforeUnmount(() => clearInterval(timer))

const upcoming = computed(() => lists.value.upcoming.filter(e => e.status !== 'cancelled' && Date.parse(e.ends_at) > now.value))
const next = computed(() => upcoming.value[0] ?? null)
const later = computed(() => upcoming.value.slice(1, 5))
const empty = computed(() => !upcoming.value.length && !lists.value.drafts.length)
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">DASHBOARD</h1>
        <div class="page-sub">
          <template v-if="org">{{ org.name }} · {{ org.timezone }}</template>
          <template v-else>WHAT NEEDS YOU TODAY</template>
        </div>
      </div>
      <NuxtLink to="/events/new" class="btn-hud btn-hud-cta" style="min-height:44px;">+ NEW EVENT</NuxtLink>
    </div>

    <div class="page-body space-y-4">
      <p v-if="error" role="alert" class="glass accent-bar-failed" style="padding:12px 16px;">
        COULDN'T LOAD YOUR NIGHTS. <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="load()">RETRY</button>
      </p>
      <div v-else-if="loading && empty" class="dash-grid" aria-busy="true">
        <div class="glass" style="height:220px;opacity:.5;" />
        <div class="glass" style="height:220px;opacity:.5;" />
      </div>
      <KhEmptyState
        v-else-if="empty" :icon="CalendarRange" title="NO EVENTS YET"
        hint="Title, date and a place is enough to start. Lineup, timetable and export come next."
        action-label="NEW EVENT" action-to="/events/new"
      />
      <div v-else class="dash-grid">
        <DashboardNextEventCard v-if="next" :event="next" :now="now" />
        <p v-else class="glass" style="padding:18px;margin:0;font-size:13px;">Nothing announced yet. Your drafts are below.</p>
        <section class="glass" style="padding:16px;" aria-labelledby="upnext-h">
          <h2 id="upnext-h" class="section-lbl" style="margin:0 0 8px;">UP NEXT</h2>
          <p v-if="!later.length" style="margin:0;font-size:13px;color:var(--color-on-surface-variant);">No other upcoming events.</p>
          <ul v-else style="list-style:none;margin:0;padding:0;">
            <li v-for="e in later" :key="e.id" style="border-bottom:1px dashed var(--color-outline-variant);">
              <NuxtLink :to="`/events/${e.id}/details`" style="display:flex;gap:10px;align-items:center;min-height:44px;text-decoration:none;color:inherit;font-size:13px;">
                <span class="data-frag" style="font-size:8px;white-space:nowrap;">{{ dayLabel(e.starts_at, e.timezone) }}</span>
                <span style="text-transform:uppercase;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ e.title }}</span>
              </NuxtLink>
            </li>
          </ul>
          <NuxtLink to="/events" class="data-frag" style="display:inline-flex;align-items:center;min-height:44px;font-size:9px;color:var(--color-primary);">ALL EVENTS →</NuxtLink>
        </section>
      </div>

      <DashboardAttentionQueue :items="items" />
    </div>
  </div>
</template>

<style scoped>
.dash-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
}
@media (min-width: 1024px) {
  .dash-grid { grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); }
}
</style>
