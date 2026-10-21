<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useOutreachStore } from '~/stores/outreach'
import { useSettingsStore } from '~/stores/settings'
import type { AvailabilitySlot, MatchedContact } from '~/types/outreach'

/**
 * Outreach (04.10-04): find gigs — your open dates × your promoter
 * contacts. Four tabs: AVAILABILITY (open slots), MATCHES (city-matched
 * promoter contacts → compose a booking request), TEMPLATES (the reusable
 * copy) and SENT (the delivery log plus the do-not-contact list).
 */
useHead({ title: 'Outreach — KlubHub DJ' })

const store = useOutreachStore()
const {
  slots, suppressions,
  slotsSec, matchesSec, templatesSec, requestsSec, suppressionsSec,
} = storeToRefs(store)
const settings = useSettingsStore()

type Tab = 'availability' | 'matches' | 'templates' | 'sent'
const TABS: { id: Tab, label: string }[] = [
  { id: 'availability', label: 'AVAILABILITY' },
  { id: 'matches', label: 'MATCHES' },
  { id: 'templates', label: 'TEMPLATES' },
  { id: 'sent', label: 'SENT' },
]
const tab = ref<Tab>('availability')

const slotFilter = ref<string>('')
const composerContact = ref<MatchedContact | null>(null)
const composerSlot = ref<AvailabilitySlot | null>(null)

onMounted(async () => {
  await Promise.allSettled([
    settings.loadFromApi(),
    store.loadSlots(),
    store.loadTemplates(),
    store.loadRequests(),
    store.loadSuppressions(),
    store.loadEpkExports(),
  ])
  await store.loadMatches().catch(() => { /* matchesSec.error shows it */ })
})

watch(slotFilter, async (id) => {
  composerContact.value = null
  composerSlot.value = null
  await store.loadMatches(id || undefined).catch(() => { /* matchesSec.error shows it */ })
})

function onCompose(payload: { contact: MatchedContact, slot: AvailabilitySlot }) {
  composerContact.value = payload.contact
  composerSlot.value = payload.slot
}

async function removeSuppression(id: string) {
  if (!window.confirm('Remove this entry from the do-not-contact list?')) return
  await store.removeSuppression(id).catch(() => { /* suppressionsSec.error shows it */ })
}
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">OUTREACH</div>
        <div class="page-sub">FIND GIGS · YOUR OPEN DATES × YOUR PROMOTER CONTACTS</div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="tabs-bar" role="tablist" aria-label="Outreach sections" data-testid="outreach-tabs">
      <button
        v-for="t in TABS" :key="t.id" type="button" role="tab" :aria-selected="tab === t.id"
        class="tab-item" :class="{ 'is-active': tab === t.id }"
        :data-testid="`tab-${t.id}`" @click="tab = t.id"
      >{{ t.label }}</button>
    </div>

    <!-- Scrollable body -->
    <div class="page-body space-y-4">

      <div v-if="tab === 'availability'" data-testid="tab-availability">
        <p v-if="slotsSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
          COULDN'T LOAD THE AVAILABILITY.
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadSlots()">RETRY</button>
        </p>
        <OutreachAvailabilityPanel />
      </div>

      <div v-else-if="tab === 'matches'" class="outreach-grid" data-testid="tab-matches">
        <div style="display:grid;gap:16px;align-content:start;">
          <label style="display:grid;gap:4px;">
            <span class="data-frag" style="font-size:10px;">SLOT</span>
            <select v-model="slotFilter" class="hud-input" data-testid="match-slot-picker">
              <option value="">ALL SLOTS</option>
              <option v-for="s in slots" :key="s.id" :value="s.id">{{ s.date }} · {{ s.city }}</option>
            </select>
          </label>
          <p v-if="matchesSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
            COULDN'T LOAD THE MATCHES.
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadMatches(slotFilter || undefined)">RETRY</button>
          </p>
          <OutreachMatchesList @compose="onCompose" />
        </div>
        <OutreachComposer :contact="composerContact" :avail-slot="composerSlot" />
      </div>

      <div v-else-if="tab === 'templates'" data-testid="tab-templates">
        <p v-if="templatesSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
          COULDN'T LOAD THE TEMPLATES.
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadTemplates()">RETRY</button>
        </p>
        <OutreachTemplateEditor />
      </div>

      <div v-else class="space-y-4" data-testid="tab-sent">
        <p v-if="requestsSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
          COULDN'T LOAD THE SENT LOG.
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadRequests()">RETRY</button>
        </p>
        <OutreachSentLog />

        <section aria-labelledby="suppressions-h" class="space-y-2">
          <h2 id="suppressions-h" class="section-lbl">DO-NOT-CONTACT LIST</h2>
          <p class="data-frag" style="font-size:11px;margin:0;">These addresses are never matched or sent to.</p>
          <p v-if="suppressionsSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
            COULDN'T LOAD THE DO-NOT-CONTACT LIST.
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="store.loadSuppressions()">RETRY</button>
          </p>
          <KhEmptyState v-else-if="!suppressions.length" title="NO SUPPRESSIONS" hint="Use DO NOT CONTACT on a sent request to add one." />
          <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
            <li
              v-for="s in suppressions" :key="s.id" class="hud-card"
              style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;"
              data-testid="suppression-row"
            >
              <span class="data-frag" style="font-size:12px;">{{ s.email }} · {{ s.reason }} · {{ new Date(s.created_at).toLocaleString() }}</span>
              <button
                type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;"
                data-testid="suppression-remove" @click="removeSuppression(s.id)"
              >REMOVE</button>
            </li>
          </ul>
        </section>
      </div>

    </div>
  </div>
</template>

<style scoped>
.tabs-bar {
  display: flex;
  gap: 0;
  padding: 0 16px;
  border-bottom: 1px solid var(--color-border);
}
.tab-item {
  padding: 10px 16px;
  font-family: var(--font-terminal);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--color-tertiary);
  background: transparent;
  border: 0;
  border-bottom: 2px solid transparent;
  cursor: pointer;
}
.tab-item.is-active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}
.outreach-grid {
  display: grid;
  grid-template-columns: minmax(320px, 1fr) minmax(360px, 1.2fr);
  gap: 16px;
  align-items: start;
}
@media (max-width: 900px) {
  .outreach-grid { grid-template-columns: 1fr; }
}
</style>
