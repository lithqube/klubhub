<script setup lang="ts">
import { Ban, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { useBookingsStore } from '~/stores/bookings'
import type { TalentContact, TalentSource } from '~/types/bookings'

/**
 * The talent book (04.10-05): the DJs the organisation knows. Each row can
 * be edited, deleted, or marked do-not-contact (one click, confirm — the
 * contact's email joins the suppression list and the contact leaves the
 * matchable pool).
 */
defineProps<{ contacts: TalentContact[], loading: boolean }>()
const emit = defineEmits<{ edit: [contact: TalentContact], add: [] }>()

const store = useBookingsStore()

const sourceLabel: Record<TalentSource, string> = {
  past_gig: 'PAST GIG', referral: 'REFERRAL', manual: 'MANUAL',
}

async function doNotContact(c: TalentContact) {
  if (!window.confirm(`Mark ${c.name || c.email} as do-not-contact? They will not be matched or sent to again.`)) return
  await store.suppressTalent(c.id)
}

async function remove(c: TalentContact) {
  if (!window.confirm(`Delete ${c.name || c.email} from the talent book?`)) return
  await store.deleteTalent(c.id)
}
</script>

<template>
  <section aria-labelledby="talent-h" class="space-y-2">
    <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px;">
      <h2 id="talent-h" class="section-lbl">TALENT CONTACTS</h2>
      <button type="button" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" data-testid="talent-add-open" @click="emit('add')">
        <Plus style="width:14px;height:14px;" aria-hidden="true" /> ADD DJ
      </button>
    </div>

    <p v-if="loading && !contacts.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
    <KhEmptyState v-else-if="!contacts.length" title="NO TALENT CONTACTS" hint="Add the DJs you know — they become matchable for your open slots." action-label="ADD DJ" @action="emit('add')" />
    <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
      <li v-for="c in contacts" :key="c.id" class="hud-card" style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;" data-testid="talent-row">
        <span style="min-width:0;">
          <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
            {{ c.name || c.email }}
            <span v-if="!c.has_email" class="badge-hud badge-archived" style="margin-left:6px;">NO EMAIL</span>
          </span>
          <span class="data-frag" style="font-size:11px;">
            <template v-if="c.email">{{ c.email }} · </template>{{ c.city }}<template v-if="c.city && c.country"> · {{ c.country }}</template>
            <template v-if="c.genres.length"> · {{ c.genres.join(', ') }}</template>
            · {{ sourceLabel[c.source] }}
          </span>
        </span>
        <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" aria-label="Edit contact" data-testid="talent-edit" @click="emit('edit', c)">
            <Pencil style="width:13px;height:13px;" aria-hidden="true" />
          </button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" data-testid="talent-suppress" @click="doNotContact(c)">
            <Ban style="width:12px;height:12px;" aria-hidden="true" /> DO NOT CONTACT
          </button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" aria-label="Delete contact" @click="remove(c)">
            <Trash2 style="width:13px;height:13px;" aria-hidden="true" />
          </button>
        </span>
      </li>
    </ul>
  </section>
</template>
