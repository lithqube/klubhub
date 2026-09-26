<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { CalendarRange } from 'lucide-vue-next'
import { useEventStore } from '~/stores/event'
import type { EventView } from '~/types/event'

useHead({ title: 'Events' })

const route = useRoute()
const store = useEventStore()
const { lists, loading, error } = storeToRefs(store)

const views: { id: EventView, label: string }[] = [
  { id: 'upcoming', label: 'UPCOMING' }, { id: 'drafts', label: 'DRAFTS' }, { id: 'past', label: 'PAST' },
]
const view = computed<EventView>(() => (['upcoming', 'drafts', 'past'].includes(String(route.query.view)) ? route.query.view as EventView : 'upcoming'))
const search = ref('')
const rows = computed(() => {
  const q = search.value.trim().toLowerCase()
  return lists.value[view.value].filter(e => !q || e.title.toLowerCase().includes(q) || (e.venue_name ?? '').toLowerCase().includes(q) || e.city.toLowerCase().includes(q))
})

await useAsyncData('events-lists', () => Promise.all(views.map(v => store.fetchList(v.id))))

const empty = computed(() => ({
  upcoming: { title: 'NO EVENTS YET', hint: 'Title, date and a place is enough to start. The rest can wait.', cta: true },
  drafts: { title: 'NO DRAFTS', hint: 'Drafts you start land here until you publish them.', cta: false },
  past: { title: 'NOTHING PAST YET', hint: 'Your past nights land here.', cta: false },
}[view.value]))
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">EVENTS</h1>
        <div class="page-sub">PLAN · RELEASE · RUN</div>
      </div>
      <div style="display:flex;gap:8px;align-items:center;">
        <label class="sr-only" for="event-search">Search events</label>
        <input id="event-search" v-model="search" class="hud-input" type="search" placeholder="Search events…" style="width:200px;height:40px;">
        <NuxtLink to="/events/new" class="btn-hud btn-hud-cta" style="min-height:44px;">+ NEW EVENT</NuxtLink>
      </div>
    </div>
    <div class="page-body space-y-4">
      <nav class="tabs-bar" aria-label="Event views">
        <NuxtLink
          v-for="v in views" :key="v.id" :to="{ query: { view: v.id } }" class="tab-item"
          :class="{ active: view === v.id }" :aria-current="view === v.id ? 'page' : undefined"
          style="min-height:44px;display:inline-flex;align-items:center;"
        >
          {{ v.label }} {{ lists[v.id].length }}
        </NuxtLink>
      </nav>

      <p v-if="error" role="alert" class="glass accent-bar-failed" style="padding:12px 16px;">
        COULDN'T LOAD YOUR NIGHTS. <button type="button" class="btn-hud btn-hud-ghost" @click="store.fetchList(view)">RETRY</button>
      </p>
      <div v-else-if="loading && !rows.length" class="space-y-2" aria-busy="true">
        <div v-for="i in 3" :key="i" class="glass" style="height:64px;opacity:.5;" />
      </div>
      <KhEmptyState
        v-else-if="!rows.length && !search"
        :icon="CalendarRange" :title="empty.title" :hint="empty.hint"
        :action-label="empty.cta ? 'NEW EVENT' : undefined" :action-to="empty.cta ? '/events/new' : undefined"
      />
      <p v-else-if="!rows.length" class="glass" style="padding:16px;font-size:13px;">
        No events match “{{ search }}”. <button type="button" class="btn-hud btn-hud-ghost" @click="search = ''">CLEAR</button>
      </p>
      <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:8px;">
        <li v-for="e in rows" :key="e.id"><EventRow :event="e" /></li>
      </ul>
    </div>
  </div>
</template>
