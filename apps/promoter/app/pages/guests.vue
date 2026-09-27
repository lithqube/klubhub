<script setup lang="ts">
import { ListPlus } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useGuestStore } from '~/stores/guest'
import type { ApiError } from '~/types/event'
import type { ListInput, StandingList } from '~/types/guest'
import { dayLabel, timeLabel } from '~/utils/datetime'
import { guestErrorText, LIST_TYPES } from '~/utils/guests'

useHead({ title: 'Guests' })

const store = useGuestStore()
const { overview, standing, loading, error } = storeToRefs(store)
await useAsyncData('guests-overview', () => Promise.all([store.fetchOverview(), store.fetchStanding()]))

const editing = ref<StandingList | 'new' | null>(null)
const formError = ref<ApiError | null>(null)
const saving = ref(false)
const notice = ref('')

const typeLabel = (t: string) => LIST_TYPES.find(x => x.id === t)?.label ?? t

async function save(input: ListInput) {
  saving.value = true
  formError.value = null
  try {
    const cur = editing.value
    const s = await store.saveStanding(input, cur && cur !== 'new' ? cur.id : undefined)
    notice.value = cur === 'new' ? `${s.name} will be added to every new event.` : `${s.name} saved. Events that already have it keep their copy.`
    editing.value = null
  } catch (e) {
    formError.value = e as ApiError
  } finally {
    saving.value = false
  }
}

async function remove(s: StandingList) {
  if (!window.confirm(`Stop adding ${s.name} to new events? Events that already have it keep their list.`)) return
  try {
    await store.deleteStanding(s.id)
    notice.value = `${s.name} removed from standing lists.`
  } catch (e) {
    notice.value = guestErrorText(e as ApiError)
  }
}

function edit(s: StandingList | 'new') {
  formError.value = null
  notice.value = ''
  editing.value = s
}
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">GUESTS</h1>
        <p class="page-sub">Guest lists across your upcoming events, and the lists every new event starts with.</p>
      </div>
    </div>
    <div class="page-body space-y-4">
      <section aria-labelledby="upcoming-h" class="space-y-2">
        <h2 id="upcoming-h" class="section-lbl">UPCOMING</h2>
        <p v-if="error" role="alert" class="glass" style="padding:12px 14px;border-left:3px solid var(--color-error);font-size:13px;">
          COULD NOT LOAD GUEST LISTS.
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="store.fetchOverview()">RETRY</button>
        </p>
        <p v-else-if="loading && !overview.length" role="status" class="data-frag" style="font-size:9px;">LOADING…</p>
        <KhEmptyState v-else-if="!overview.length" title="NO UPCOMING EVENTS" hint="Guest lists live on each event." action-label="NEW EVENT" action-to="/events/new" />
        <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
          <li v-for="o in overview" :key="o.event_id">
            <NuxtLink
              :to="`/events/${o.event_id}/guests`" class="hud-card overview-row" :class="o.pending ? 'accent-bar-pending' : 'accent-bar-ready'"
              :aria-label="`${o.title}: ${o.going_heads} going heads, ${o.pending} to approve`"
            >
              <span style="min-width:0;">
                <span class="data-frag" style="font-size:9px;">{{ dayLabel(o.starts_at, o.timezone) }} · {{ timeLabel(o.starts_at, o.timezone) }}</span>
                <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ o.title }}</span>
                <span style="font-size:12px;color:var(--color-on-surface-variant);">
                  {{ o.lists }} {{ o.lists === 1 ? 'list' : 'lists' }} · {{ o.guests }} guests · {{ o.going_heads }} going heads<template v-if="o.capacity"> of {{ o.capacity }}</template><template v-if="o.tickets"> · {{ o.tickets }} tickets</template>
                </span>
              </span>
              <span style="display:grid;gap:4px;min-width:0;">
                <GuestQuotaBar v-if="o.quota" :used="o.used" :quota="o.quota" :label="`${o.title} allocations`" />
                <span v-else class="data-frag" style="font-size:8px;">NO ALLOCATIONS</span>
                <span v-if="o.pending" class="badge-hud badge-draft" style="justify-self:start;">{{ o.pending }} TO APPROVE</span>
              </span>
            </NuxtLink>
          </li>
        </ul>
      </section>

      <section aria-labelledby="standing-h" class="space-y-2">
        <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;">
          <div>
            <h2 id="standing-h" class="section-lbl" style="margin:0;">STANDING LISTS</h2>
            <p style="margin:4px 0 0;font-size:12px;color:var(--color-on-surface-variant);">
              Copied into every new event. Changing one here doesn't touch events that already have it.
            </p>
          </div>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :aria-expanded="editing === 'new'" @click="edit('new')">
            <ListPlus style="width:14px;height:14px;" aria-hidden="true" /> NEW STANDING LIST
          </button>
        </div>
        <p v-if="notice" role="status" class="data-frag" style="font-size:9px;">{{ notice }}</p>
        <GuestListForm v-if="editing === 'new'" template :error="formError" :saving="saving" submit-label="CREATE STANDING LIST" @save="save" @cancel="editing = null" />
        <p v-if="!standing.length && editing !== 'new'" class="glass" style="padding:14px;font-size:13px;">
          None yet. Residents, crew or a regular comp list are good candidates.
        </p>
        <div v-for="s in standing" :key="s.id">
          <GuestListForm v-if="editing !== 'new' && editing?.id === s.id" :initial="s" template :error="formError" :saving="saving" @save="save" @cancel="editing = null" />
          <div v-else class="hud-card" style="padding:10px 14px;display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;">
            <span style="min-width:0;display:flex;flex-wrap:wrap;align-items:center;gap:6px;">
              <strong style="font-size:14px;">{{ s.name }}</strong>
              <span class="badge-hud badge-published">{{ typeLabel(s.type) }}</span>
              <span class="badge-hud badge-published">{{ s.collect_contact ? 'CONTACTS' : 'NAME-ONLY' }}</span>
              <span class="data-frag" style="font-size:9px;">
                {{ s.entry_terms.price_mode === 'reduced' ? `REDUCED · ${s.entry_terms.reduced_price_text}` : 'FREE' }}<template v-if="s.entry_terms.cutoff_local"> · CUTOFF {{ s.entry_terms.cutoff_local }}</template>
              </span>
            </span>
            <span style="display:flex;gap:4px;">
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Edit standing list ${s.name}`" @click="edit(s)">EDIT</button>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Remove standing list ${s.name}`" style="color:var(--color-error);" @click="remove(s)">REMOVE</button>
            </span>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.overview-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 8px;
  padding: 10px 14px;
  text-decoration: none;
  color: inherit;
  min-height: 44px;
}
@media (min-width: 768px) {
  .overview-row { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); align-items: center; }
}
</style>
