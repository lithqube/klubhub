<script setup lang="ts">
import { Ban } from 'lucide-vue-next'
import { useBookingsStore } from '~/stores/bookings'
import type { OutreachRequest } from '~/types/bookings'
import { offerStatusClass } from '~/utils/bookings'

/**
 * The sent log (04.10-05): every offer with its delivery status. Each row
 * has a one-click "Do not contact" action (SPEC §9 v2.1 — opt-out has an
 * owner): it posts the row's recipient email to the suppression list, so a
 * reply of "stop" has a one-click path even before inbound-reply handling
 * exists.
 */
defineProps<{ offers: OutreachRequest[], loading: boolean }>()

const store = useBookingsStore()

async function doNotContact(o: OutreachRequest) {
  if (!window.confirm(`Add ${o.to_email} to your do-not-contact list? They will not be matched or sent to again.`)) return
  await store.addSuppression({ email: o.to_email, reason: 'opted out via the sent log' })
}
</script>

<template>
  <section aria-labelledby="sent-h" class="space-y-2">
    <h2 id="sent-h" class="section-lbl">SENT OFFERS</h2>

    <p v-if="loading && !offers.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
    <KhEmptyState v-else-if="!offers.length" title="NOTHING SENT YET" hint="Compose an offer from a matched talent contact and it lands here." />
    <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
      <li v-for="o in offers" :key="o.id" class="hud-card" style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;" data-testid="offer-row">
        <span style="min-width:0;">
          <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ o.subject }}</span>
          <span class="data-frag" style="font-size:11px;">
            to {{ o.to_email }} · {{ new Date(o.created_at).toLocaleString() }}
            <template v-if="o.email_error"> · {{ o.email_error }}</template>
          </span>
        </span>
        <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
          <span class="badge-hud" :class="offerStatusClass(o.status)" data-testid="offer-status">{{ o.status.toUpperCase() }}</span>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" data-testid="offer-suppress" @click="doNotContact(o)">
            <Ban style="width:12px;height:12px;" aria-hidden="true" /> DO NOT CONTACT
          </button>
        </span>
      </li>
    </ul>
  </section>
</template>
