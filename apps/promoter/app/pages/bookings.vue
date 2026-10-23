<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useBookingsStore } from '~/stores/bookings'
import { useEventStore } from '~/stores/event'
import { useOrgStore } from '~/stores/org'
import { useSessionStore } from '~/stores/session'
import type { OpenSlot, TalentContact, TalentMatch } from '~/types/bookings'

/**
 * Bookings (04.10-05): fill open slots with the DJs you know. Four tabs —
 * SLOTS (event → open slots → matched talent → compose), TALENT (the
 * contact book), TEMPLATES (offer templates) and SENT (the delivery log
 * plus the do-not-contact list). talent.* and booking.* are owner/admin/
 * booker actions; the API enforces the same split through OPA.
 */
useHead({ title: 'Bookings' })

const session = useSessionStore()
const { me } = storeToRefs(session)
const events = useEventStore()
const org = useOrgStore()
const store = useBookingsStore()
const {
  talent, slots, matches, offers, suppressions,
  talentSec, slotsSec, matchesSec, offersSec, suppressionsSec,
} = storeToRefs(store)

const BOOKING_ROLES = new Set(['owner', 'admin', 'booker'])
const canBook = computed(() => !!me.value?.roles.some(r => BOOKING_ROLES.has(r)))

type Tab = 'slots' | 'talent' | 'templates' | 'sent'
const TABS: { id: Tab, label: string }[] = [
  { id: 'slots', label: 'SLOTS' },
  { id: 'talent', label: 'TALENT' },
  { id: 'templates', label: 'TEMPLATES' },
  { id: 'sent', label: 'SENT' },
]
const tab = ref<Tab>('slots')

const eventId = ref<string | null>(null)
const selectedSlot = ref<OpenSlot | null>(null)
const composerContact = ref<TalentContact | null>(null)
const editingContact = ref<TalentContact | 'new' | null>(null)

const upcoming = computed(() => events.lists.upcoming)
const selectedEvent = computed(() => upcoming.value.find(e => e.id === eventId.value) ?? null)

onMounted(async () => {
  if (!canBook.value) return
  await Promise.allSettled([
    events.fetchList('upcoming'),
    org.fetchOrg(),
    store.loadTalent(),
    store.loadTemplates(),
    store.loadOffers(),
    store.loadSuppressions(),
  ])
})

watch(eventId, async (id) => {
  selectedSlot.value = null
  composerContact.value = null
  store.clearMatches()
  if (id) await store.loadSlots(id).catch(() => { /* slotsSec.error shows it */ })
})

function onSelectSlot(slot: OpenSlot) {
  selectedSlot.value = slot
  composerContact.value = null
  void store.loadMatches(slot.id).catch(() => { /* matchesSec.error shows it */ })
}

function onCompose(m: TalentMatch) {
  composerContact.value = m.contact
}

function startAddContact() {
  editingContact.value = 'new'
}

function startEditContact(c: TalentContact) {
  editingContact.value = c
}

function closeContactEditor() {
  editingContact.value = null
}

async function removeSuppression(id: string) {
  if (!window.confirm('Remove this entry from the do-not-contact list?')) return
  await store.deleteSuppression(id).catch(() => { /* suppressionsSec.error shows it */ })
}
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">BOOKINGS</h1>
        <p class="page-sub">Fill your open slots: your events × the DJs you know.</p>
      </div>
    </div>
    <div class="page-body space-y-4">
      <section v-if="!canBook" class="glass" style="padding:14px;display:grid;gap:8px;" data-testid="bookings-forbidden">
        <h2 class="section-lbl" style="margin:0;">BOOKINGS NEED A BOOKING ROLE</h2>
        <p class="data-frag" style="font-size:13px;margin:0;">
          Open slots, the talent book and offers are owner, admin and booker actions. Ask an owner for a role, or sign in as one.
        </p>
      </section>

      <template v-else>
        <div role="tablist" aria-label="Bookings sections" style="display:flex;gap:4px;flex-wrap:wrap;">
          <button
            v-for="t in TABS" :key="t.id" type="button" role="tab" :aria-selected="tab === t.id"
            class="btn-hud btn-hud-sm" :class="tab === t.id ? 'btn-hud-cta' : 'btn-hud-ghost'"
            style="min-height:40px;" :data-testid="`tab-${t.id}`" @click="tab = t.id"
          >
            {{ t.label }}
          </button>
        </div>

        <div v-if="tab === 'slots'" class="bookings-grid" data-testid="tab-slots">
          <BookingSlotList
            :event-id="eventId" :events="upcoming" :slots="slots" :loading="slotsSec.loading"
            @update:event-id="eventId = $event" @select="onSelectSlot"
          />
          <div style="display:grid;gap:16px;align-content:start;">
            <p v-if="slotsSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
              COULDN'T LOAD THE SLOTS.
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="eventId && store.loadSlots(eventId)">RETRY</button>
            </p>
            <BookingMatchList :loading="matchesSec.loading" :matches="matches" :open-slot="selectedSlot" @compose="onCompose" />
            <p v-if="matchesSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
              COULDN'T LOAD THE MATCHES.
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="selectedSlot && store.loadMatches(selectedSlot.id)">RETRY</button>
            </p>
            <BookingComposer :contact="composerContact" :event="selectedEvent" :open-slot="selectedSlot" />
          </div>
        </div>

        <div v-else-if="tab === 'talent'" class="space-y-4" data-testid="tab-talent">
          <TalentContactEditor
            v-if="editingContact" :contact="editingContact === 'new' ? null : editingContact"
            @saved="closeContactEditor" @cancel="closeContactEditor"
          />
          <p v-if="talentSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
            COULDN'T LOAD THE TALENT BOOK.
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadTalent()">RETRY</button>
          </p>
          <TalentContactList :contacts="talent" :loading="talentSec.loading" @edit="startEditContact" @add="startAddContact" />
        </div>

        <div v-else-if="tab === 'templates'" data-testid="tab-templates">
          <BookingTemplateEditor />
        </div>

        <div v-else class="space-y-4" data-testid="tab-sent">
          <p v-if="offersSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
            COULDN'T LOAD THE SENT LOG.
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadOffers()">RETRY</button>
          </p>
          <BookingSentLog :offers="offers" :loading="offersSec.loading" />

          <section aria-labelledby="suppressions-h" class="space-y-2">
            <h2 id="suppressions-h" class="section-lbl">DO-NOT-CONTACT LIST</h2>
            <p class="data-frag" style="font-size:11px;margin:0;">These addresses are never matched or sent to. The list stores a blind index only — no addresses are shown.</p>
            <p v-if="suppressionsSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
              COULDN'T LOAD THE DO-NOT-CONTACT LIST.
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadSuppressions()">RETRY</button>
            </p>
            <KhEmptyState v-else-if="!suppressions.length" title="NO SUPPRESSIONS" hint="Use DO NOT CONTACT on a sent offer or a talent row to add one." />
            <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
              <li v-for="s in suppressions" :key="s.id" class="hud-card" style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;" data-testid="suppression-row">
                <span class="data-frag" style="font-size:12px;">{{ s.reason }} · {{ new Date(s.created_at).toLocaleString() }}</span>
                <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" data-testid="suppression-remove" @click="removeSuppression(s.id)">REMOVE</button>
              </li>
            </ul>
          </section>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.bookings-grid {
  display: grid;
  grid-template-columns: minmax(280px, 1fr) minmax(320px, 1.4fr);
  gap: 16px;
  align-items: start;
}
@media (max-width: 900px) {
  .bookings-grid { grid-template-columns: 1fr; }
}
</style>
