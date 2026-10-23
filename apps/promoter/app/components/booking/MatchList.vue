<script setup lang="ts">
import { Send, MailWarning } from 'lucide-vue-next'
import type { OpenSlot, TalentMatch } from '~/types/bookings'

/**
 * Ranked talent matches for the selected slot (SPEC §8: genre overlap desc,
 * then most recently added). Contacts without an email (has_email=false,
 * OFFER-10) are listed with a "no email" badge and NO compose button —
 * matchable but not sendable.
 */
defineProps<{ openSlot: OpenSlot | null, matches: TalentMatch[], loading: boolean }>()
const emit = defineEmits<{ compose: [match: TalentMatch] }>()
</script>

<template>
  <section aria-labelledby="matches-h" class="space-y-2">
    <h2 id="matches-h" class="section-lbl">MATCHED TALENT</h2>

    <p v-if="!openSlot" class="data-frag" style="font-size:11px;">Select a slot to see who matches it.</p>
    <template v-else>
      <p v-if="loading && !matches.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
      <KhEmptyState v-else-if="!matches.length" title="NO MATCHES" :hint="`No talent contacts in ${openSlot.genres.length ? 'these genres' : 'this city'} yet. Add some in the Talent tab.`" />
      <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
        <li v-for="m in matches" :key="m.contact.id" class="hud-card" style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;" data-testid="match-row">
          <span style="min-width:0;">
            <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ m.contact.name || m.contact.email }}</span>
            <span class="data-frag" style="font-size:11px;">
              {{ m.contact.city }}<template v-if="m.contact.city && m.contact.country"> · {{ m.contact.country }}</template>
              <template v-if="m.contact.genres.length"> · {{ m.contact.genres.join(', ') }}</template>
            </span>
            <span v-if="m.overlap_genres.length" style="display:flex;gap:4px;flex-wrap:wrap;margin-top:4px;">
              <span v-for="g in m.overlap_genres" :key="g" class="badge-hud badge-ready">{{ g }}</span>
            </span>
          </span>
          <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
            <span v-if="!m.contact.has_email" class="badge-hud badge-archived" data-testid="no-email-badge">
              <MailWarning style="width:11px;height:11px;" aria-hidden="true" /> NO EMAIL
            </span>
            <button
              v-else type="button" class="btn-hud btn-hud-cta btn-hud-xs" style="min-height:36px;"
              data-testid="compose-btn" @click="emit('compose', m)"
            >
              <Send style="width:12px;height:12px;" aria-hidden="true" /> COMPOSE OFFER
            </button>
          </span>
        </li>
      </ul>
    </template>
  </section>
</template>
