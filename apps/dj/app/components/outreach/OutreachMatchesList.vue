<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { Mail, MailWarning } from 'lucide-vue-next'
import { useOutreachStore } from '~/stores/outreach'
import type { AvailabilitySlot, MatchedContact } from '~/types/outreach'

/**
 * Matches (04.10-04): per availability slot, the DJ's promoter contacts in
 * the slot's city, ranked (a past gig in that city first, then name — the
 * server's order). Contacts without an email (has_email=false, OUTR-05 v2.1)
 * are listed with a "no email" badge and NO Compose button — matchable but
 * not sendable. Venues in the city show as context chips.
 */
const store = useOutreachStore()
const { matches, matchesSec, selectedSlotId } = storeToRefs(store)

const emit = defineEmits<{
  (e: 'compose', payload: { contact: MatchedContact, slot: AvailabilitySlot }): void
}>()

/** The matches to show: the selected slot's group, or every group. */
const groups = computed(() => (selectedSlotId.value
  ? matches.value.filter(m => m.slot.id === selectedSlotId.value)
  : matches.value))
</script>

<template>
  <section aria-labelledby="matches-h" class="space-y-2">
    <h2 id="matches-h" class="section-lbl">MATCHES</h2>

    <p v-if="matchesSec.loading && !matches.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
    <KhEmptyState
      v-else-if="!groups.length"
      title="NO MATCHES YET"
      :hint="selectedSlotId ? 'No promoter contacts in this city: add some in Contacts.' : 'Add an availability slot and load its matches.'"
      data-testid="matches-empty"
    />
    <div v-else class="space-y-4">
      <div v-for="m in groups" :key="m.slot.id" class="space-y-2" data-testid="match-group">
        <p class="data-frag" style="font-size:11px;margin:0;">
          {{ m.slot.date }} · {{ m.slot.city }} · {{ m.contacts.length }} promoter{{ m.contacts.length === 1 ? '' : 's' }}
        </p>
        <div v-if="m.venues.length" style="display:flex;gap:6px;flex-wrap:wrap;">
          <span v-for="v in m.venues" :key="v.id" class="social-chip" data-testid="venue-chip">{{ v.name }}</span>
        </div>
        <ul style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
          <li
            v-for="c in m.contacts" :key="c.id" class="hud-card"
            style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;"
            data-testid="match-row"
          >
            <span style="min-width:0;">
              <span style="display:block;font-size:14px;font-weight:600;">
                {{ c.name }}
                <span v-if="c.past_gig_in_city" class="badge-hud badge-ready" style="margin-left:6px;">PAST GIG HERE</span>
              </span>
              <span class="data-frag" style="font-size:11px;">
                {{ c.company || 'Promoter' }} · {{ c.city }}
              </span>
            </span>
            <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
              <span v-if="!c.has_email" class="badge-hud badge-failed" data-testid="no-email-badge">
                <MailWarning style="width:10px;height:10px;" aria-hidden="true" /> NO EMAIL
              </span>
              <button
                v-if="c.has_email" type="button" class="btn-hud btn-hud-cta btn-hud-xs" style="min-height:36px;"
                data-testid="compose-btn" @click="emit('compose', { contact: c, slot: m.slot })"
              >
                <Mail style="width:12px;height:12px;" aria-hidden="true" /> COMPOSE
              </button>
            </span>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>
