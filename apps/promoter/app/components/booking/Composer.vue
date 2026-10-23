<script setup lang="ts">
import { Send, TriangleAlert } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useBookingsStore } from '~/stores/bookings'
import { useOrgStore } from '~/stores/org'
import { useSessionStore } from '~/stores/session'
import type { ApiError, EventSummary } from '~/types/event'
import type { OpenSlot, TalentContact } from '~/types/bookings'
import { mentionsVenue, offerErrorText, offerVariables, renderOfferText, venueWithheld, OFFER_VARIABLES } from '~/utils/bookings'

/**
 * The offer composer (04.10-05): template picker, subject/body editor with
 * a live preview, and send. The editor works on a per-request copy — the
 * template itself is never mutated here (it is only changed in the
 * template editor). The preview renders variables client-side; the server
 * re-renders authoritatively at send.
 *
 * OFFER-09: when the event withholds its venue (secret before the reveal,
 * or city_only) and the text mentions {{venue}}, a warning banner shows
 * and the preview renders the venue empty.
 */
const props = defineProps<{ event: EventSummary | null, openSlot: OpenSlot | null, contact: TalentContact | null }>()
const emit = defineEmits<{ sent: [message: string] }>()

// Rendered via helpers: a literal {{…}} inside a mustache breaks the template parser.
const varLabel = (v: string) => `{{${v}}}`
const VENUE_VAR = '{{venue}}'

const store = useBookingsStore()
const { templates } = storeToRefs(store)
const orgStore = useOrgStore()
const session = useSessionStore()

const templateId = ref('')
const subject = ref('')
const body = ref('')
const edited = ref(false)
const sending = ref(false)
const error = ref<ApiError | null>(null)
const notice = ref('')

const selectedTemplate = computed(() => templates.value.find(t => t.id === templateId.value) ?? null)

// The preview approximates the server's render: the member's name is not in
// /auth/me, so the email stands in for promoter_name (the server renders
// the real name at send).
const vars = computed(() => offerVariables({
  orgName: orgStore.org?.name ?? '',
  promoterName: session.me?.email ?? '',
  promoterEmail: session.me?.email ?? '',
  contact: props.contact,
  event: props.event,
  venueName: props.event?.venue_name ?? null,
  slot: props.openSlot,
}))

const previewSubject = computed(() => renderOfferText(subject.value, vars.value))
const previewBody = computed(() => renderOfferText(body.value, vars.value))

const venueWarning = computed(() => {
  if (!props.event || !venueWithheld(props.event.location_mode, props.event.location_reveal_at)) return false
  return mentionsVenue(subject.value) || mentionsVenue(body.value)
})

const canSend = computed(() => !!props.contact?.has_email && !!subject.value.trim() && !!body.value.trim() && !sending.value)

watch(templateId, (id) => {
  const t = templates.value.find(x => x.id === id)
  if (!t) return
  // A per-request copy: picking a template pre-fills the editor, it never
  // writes back to the template.
  subject.value = t.subject
  body.value = t.body
  edited.value = false
})

watch(() => props.contact?.id, () => {
  // A new target starts from the default template's copy.
  const t = templates.value.find(x => x.is_default) ?? templates.value[0] ?? null
  templateId.value = t?.id ?? ''
  subject.value = t?.subject ?? ''
  body.value = t?.body ?? ''
  edited.value = false
  error.value = null
  notice.value = ''
})

async function send() {
  if (!props.contact) return
  sending.value = true
  error.value = null
  notice.value = ''
  try {
    await store.sendOffer({
      talent_contact_id: props.contact.id,
      open_slot_id: props.openSlot?.id ?? null,
      template_id: selectedTemplate.value?.id ?? null,
      subject: subject.value,
      body: body.value,
    })
    notice.value = `Offer sent to ${props.contact.name || props.contact.email}.`
    emit('sent', notice.value)
  } catch (e) {
    error.value = e as ApiError
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <section aria-labelledby="composer-h" class="space-y-2">
    <h2 id="composer-h" class="section-lbl">COMPOSE OFFER</h2>

    <KhEmptyState v-if="!contact" title="NO DJ SELECTED" hint="Pick a matched talent contact to compose an offer." />
    <div v-else class="hud-card" style="padding:12px 14px;display:grid;gap:10px;" data-testid="composer">
      <div class="data-frag" style="font-size:11px;display:flex;flex-wrap:wrap;gap:6px;align-items:center;">
        <span class="badge-hud">TO: {{ contact.name || contact.email }}</span>
        <span v-if="event" class="badge-hud badge-draft">{{ event.title }}</span>
        <span v-if="openSlot" class="badge-hud badge-draft">{{ openSlot.role || 'Open slot' }}</span>
      </div>

      <p v-if="!contact.has_email" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
        THIS DJ HAS NO EMAIL ADDRESS: offers cannot be sent to them.
      </p>

      <p v-if="venueWarning" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-warning, #e0a800);font-size:13px;display:flex;gap:8px;align-items:center;" data-testid="venue-warning">
        <TriangleAlert style="width:14px;height:14px;flex-shrink:0;" aria-hidden="true" />
        THIS EVENT WITHHOLDS ITS VENUE: {{ VENUE_VAR }} WILL BE SENT EMPTY.
      </p>

      <label style="display:grid;gap:2px;font-size:11px;">
        TEMPLATE
        <select v-model="templateId" class="hud-input" style="min-height:40px;" data-testid="composer-template">
          <option value="" disabled>SELECT A TEMPLATE</option>
          <option v-for="t in templates" :key="t.id" :value="t.id">
            {{ t.name }}{{ t.is_default ? ' (DEFAULT)' : '' }}
          </option>
        </select>
      </label>

      <label style="display:grid;gap:2px;font-size:11px;">
        SUBJECT
        <input v-model="subject" class="hud-input" style="min-height:40px;" data-testid="composer-subject" @input="edited = true">
      </label>

      <label style="display:grid;gap:2px;font-size:11px;">
        BODY
        <textarea v-model="body" class="hud-input" rows="8" style="min-height:120px;font-family:inherit;" data-testid="composer-body" @input="edited = true"/>
      </label>

      <details class="data-frag" style="font-size:11px;">
        <summary style="cursor:pointer;">AVAILABLE VARIABLES</summary>
        <span style="display:flex;flex-wrap:wrap;gap:4px;margin-top:4px;">
          <code v-for="v in OFFER_VARIABLES" :key="v" class="badge-hud">{{ varLabel(v) }}</code>
        </span>
      </details>

      <div class="glass" style="padding:10px 12px;display:grid;gap:6px;" data-testid="composer-preview">
        <span class="section-lbl" style="font-size:10px;">PREVIEW</span>
        <span style="font-size:13px;font-weight:600;">{{ previewSubject }}</span>
        <pre style="margin:0;font-size:12px;white-space:pre-wrap;font-family:inherit;">{{ previewBody }}</pre>
      </div>

      <p v-if="error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;" data-testid="composer-error">
        {{ offerErrorText(error) }}
      </p>
      <p v-if="notice" role="status" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-ready, #2fbf71);font-size:13px;" data-testid="composer-notice">
        {{ notice }}
      </p>

      <div>
        <button type="button" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" :disabled="!canSend" data-testid="composer-send" @click="send">
          <Send style="width:14px;height:14px;" aria-hidden="true" /> {{ sending ? 'SENDING…' : 'SEND OFFER' }}
        </button>
      </div>
    </div>
  </section>
</template>
