<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { Lock, MapPin } from 'lucide-vue-next'
import { useSessionStore } from '~/stores/session'
import { useVenueStore } from '~/stores/venue'

useHead({ title: 'Venues' })

const store = useVenueStore()
const { venues, loading, error } = storeToRefs(store)
const { canEditEvents } = storeToRefs(useSessionStore())
await useAsyncData('venues', () => store.fetchVenues())

const search = ref('')
const rows = computed(() => {
  const q = search.value.trim().toLowerCase()
  return venues.value.filter(v => !q || v.name.toLowerCase().includes(q) || v.city.toLowerCase().includes(q))
})
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">VENUES</h1>
        <div class="page-sub">ROOMS · CURFEWS · PROTECTED CONTACTS</div>
      </div>
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
        <label class="sr-only" for="venue-search">Search venues</label>
        <input id="venue-search" v-model="search" class="hud-input" type="search" placeholder="Search venues…" style="width:200px;height:40px;">
        <NuxtLink v-if="canEditEvents" to="/venues/new" class="btn-hud btn-hud-cta" style="min-height:44px;">+ NEW VENUE</NuxtLink>
      </div>
    </div>
    <div class="page-body space-y-4">
      <p v-if="error" role="alert" class="glass accent-bar-failed" style="padding:12px 16px;">
        COULDN'T LOAD VENUES. <button type="button" class="btn-hud btn-hud-ghost" @click="store.fetchVenues()">RETRY</button>
      </p>
      <div v-else-if="loading && !rows.length" class="space-y-2" aria-busy="true">
        <div v-for="i in 3" :key="i" class="glass" style="height:64px;opacity:.5;" />
      </div>
      <KhEmptyState
        v-else-if="!rows.length && !search" :icon="MapPin" title="NO VENUES YET"
        hint="Save a venue once and reuse its rooms, curfew and contacts for every night there."
        :action-label="canEditEvents ? 'NEW VENUE' : undefined" :action-to="canEditEvents ? '/venues/new' : undefined"
      />
      <p v-else-if="!rows.length" class="glass" style="padding:16px;font-size:13px;">
        No venues match “{{ search }}”. <button type="button" class="btn-hud btn-hud-ghost" @click="search = ''">CLEAR</button>
      </p>
      <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:8px;">
        <li v-for="v in rows" :key="v.id">
          <NuxtLink :to="`/venues/${v.id}`" class="glass venue-row">
            <div style="min-width:0;">
              <div style="font-family:var(--font-command);font-weight:700;text-transform:uppercase;overflow-wrap:anywhere;">{{ v.name }}</div>
              <div style="font-size:12px;color:var(--color-on-surface-variant);">
                {{ [v.city, v.country].filter(Boolean).join(', ') || 'No city' }} · {{ v.timezone }}
              </div>
            </div>
            <div style="display:flex;flex-wrap:wrap;gap:6px;justify-content:flex-end;">
              <span class="data-frag" style="font-size:8px;">{{ v.rooms.length }} {{ v.rooms.length === 1 ? 'ROOM' : 'ROOMS' }}</span>
              <span v-if="v.capacity" class="data-frag" style="font-size:8px;">CAP {{ v.capacity }}</span>
              <span v-if="v.curfew_local" class="data-frag" style="font-size:8px;">CURFEW {{ v.curfew_local }}</span>
              <span v-if="v.protected.address || v.protected.contact" class="data-frag" style="font-size:8px;display:inline-flex;gap:4px;align-items:center;">
                <Lock :size="9" aria-hidden="true" />{{ [v.protected.address && 'ADDRESS', v.protected.contact && 'CONTACT'].filter(Boolean).join(' + ') }}
              </span>
            </div>
          </NuxtLink>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.venue-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 12px 16px;
  min-height: 64px;
  text-decoration: none;
  color: inherit;
}
.venue-row:hover, .venue-row:focus-visible { border-color: var(--color-primary); }
@media (max-width: 639px) {
  .venue-row { grid-template-columns: minmax(0, 1fr); }
  .venue-row > div:last-child { justify-content: flex-start; }
}
</style>
