<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { Ban, FileText } from 'lucide-vue-next'
import { useOutreachStore } from '~/stores/outreach'
import type { OutreachRequest } from '~/types/outreach'
import { deliveryLabel, requestStatusClass } from '~/utils/outreach'

/**
 * The sent log (04.10-04): every booking request with its delivery status
 * and EPK attachment. Each row has a one-click "Do not contact" action
 * (SPEC §9 v2.1 — opt-out has an owner): it posts the row's recipient email
 * to the suppression list, so a reply of "stop" has a one-click path even
 * before inbound-reply handling exists.
 */
const store = useOutreachStore()
const { requests, requestsSec } = storeToRefs(store)

async function doNotContact(r: OutreachRequest) {
  if (!window.confirm(`Add ${r.to_email} to your do-not-contact list? They will not be matched or sent to again.`)) return
  await store.addSuppression({ email: r.to_email, reason: 'opted out via the sent log' }).catch(() => { /* suppressionsSec.error shows it */ })
}
</script>

<template>
  <section aria-labelledby="sent-h" class="space-y-2">
    <h2 id="sent-h" class="section-lbl">SENT REQUESTS</h2>

    <p v-if="requestsSec.loading && !requests.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
    <KhEmptyState v-else-if="!requests.length" title="NOTHING SENT YET" hint="Compose a booking request from a matched promoter contact and it lands here." />
    <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
      <li
        v-for="r in requests" :key="r.id" class="hud-card"
        style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;"
        data-testid="request-row"
      >
        <span style="min-width:0;">
          <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ r.subject }}</span>
          <span class="data-frag" style="font-size:11px;">
            to {{ r.to_email }} · {{ new Date(r.created_at).toLocaleString() }}
            <template v-if="r.email_error"> · {{ r.email_error }}</template>
          </span>
        </span>
        <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
          <span v-if="r.epk_export_id" class="social-chip" data-testid="request-epk">
            <FileText style="width:10px;height:10px;" aria-hidden="true" /> EPK
          </span>
          <span class="badge-hud" :class="requestStatusClass(r.status)" data-testid="request-status">{{ deliveryLabel(r) }}</span>
          <button
            type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;"
            data-testid="request-suppress" @click="doNotContact(r)"
          >
            <Ban style="width:12px;height:12px;" aria-hidden="true" /> DO NOT CONTACT
          </button>
        </span>
      </li>
    </ul>
  </section>
</template>
