<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { FileText, Send } from 'lucide-vue-next'
import { useOutreachStore } from '~/stores/outreach'
import { useSettingsStore } from '~/stores/settings'
import type { AvailabilitySlot, MatchedContact } from '~/types/outreach'
import { OUTREACH_VARIABLES, renderRequestText, requestErrorText, requestVariables } from '~/utils/outreach'

/**
 * Composer (04.10-04): one booking-request email to a matched promoter
 * contact. The template picker pre-fills a PER-REQUEST COPY of the subject
 * and body — the template itself is never mutated (RIDER-02 precedent); the
 * server re-renders authoritatively at send. The preview renders the {{var}}
 * map client-side. An EPK export can be attached (SPEC §5.4 3a).
 */
const props = defineProps<{ contact: MatchedContact | null, availSlot: AvailabilitySlot | null }>()

const store = useOutreachStore()
const { templates, epkExports, requestsSec } = storeToRefs(store)
const settings = useSettingsStore()

const templateId = ref<string>('')
const subject = ref('')
const body = ref('')
const epkExportId = ref<string>('')
const sending = ref(false)
const notice = ref('')
const errorText = ref('')

/** The DJ's own contact block (user_settings.contact_info) — the {{dj_*}}
 * variables and the ReplyTo of every outreach email (OUTR-06 v2.1). */
const djContact = computed(() => {
  let email = ''
  let phone = ''
  try {
    const parsed = JSON.parse(settings.contactInfo || '{}') as { name?: string, email?: string, phone?: string }
    email = parsed.email ?? ''
    phone = parsed.phone ?? ''
  } catch { /* unparseable contact_info renders empty */ }
  return { name: settings.djName, email, phone }
})

const previewVars = computed(() => requestVariables({
  djName: djContact.value.name,
  djEmail: djContact.value.email,
  djPhone: djContact.value.phone,
  contact: props.contact,
  slot: props.availSlot,
  // The public EPK URL is server config (EPK_PUBLIC_URL); the preview renders
  // {{epk_link}} empty — the server fills it authoritatively at send.
  epkLink: '',
}))

const previewSubject = computed(() => renderRequestText(subject.value, previewVars.value))
const previewBody = computed(() => renderRequestText(body.value, previewVars.value))

const selectedEpk = computed(() => epkExports.value.find(e => e.id === epkExportId.value) ?? null)

const variableHints = OUTREACH_VARIABLES.map(v => `{{${v}}}`).join('  ')

/** Pre-fill the per-request copy from the chosen template (raw variables). */
function applyTemplate(id: string) {
  const t = templates.value.find(x => x.id === id)
  if (!t) return
  subject.value = t.subject
  body.value = t.body
}

watch(templateId, (id) => { if (id) applyTemplate(id) })

// A new contact (or the default template arriving late) re-seeds the copy.
watch(() => props.contact?.id, () => {
  notice.value = ''
  errorText.value = ''
  epkExportId.value = ''
  const def = templates.value.find(t => t.is_default) ?? templates.value[0] ?? null
  templateId.value = def?.id ?? ''
  if (def) applyTemplate(def.id)
})

watch(templates, (list) => {
  if (!templateId.value && list.length) {
    const def = list.find(t => t.is_default) ?? list[0]!
    templateId.value = def.id
    if (props.contact) applyTemplate(def.id)
  }
})

async function send() {
  if (!props.contact || sending.value) return
  sending.value = true
  notice.value = ''
  errorText.value = ''
  try {
    await store.sendRequest({
      contact_id: props.contact.id,
      availability_slot_id: props.availSlot?.id ?? null,
      template_id: templateId.value || null,
      epk_export_id: epkExportId.value || null,
      subject: subject.value,
      body: body.value,
    })
    notice.value = `Request sent to ${props.contact.name || props.contact.email}.`
  } catch (e) {
    errorText.value = requestErrorText(e as never)
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <section v-if="contact" aria-labelledby="composer-h" class="hud-card glass" style="padding:14px;display:grid;gap:12px;" data-testid="composer">
    <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;flex-wrap:wrap;">
      <h2 id="composer-h" class="section-lbl" style="margin:0;">COMPOSE REQUEST</h2>
      <span class="data-frag" style="font-size:11px;" data-testid="composer-contact">
        to {{ contact.name }} · {{ availSlot ? `${availSlot.date} · ${availSlot.city}` : 'no slot' }}
      </span>
    </div>

    <label style="display:grid;gap:4px;">
      <span class="data-frag" style="font-size:10px;">TEMPLATE</span>
      <select v-model="templateId" class="hud-input" data-testid="composer-template">
        <option v-for="t in templates" :key="t.id" :value="t.id">
          {{ t.name }}{{ t.is_default ? ' (default)' : '' }}
        </option>
      </select>
    </label>

    <label style="display:grid;gap:4px;">
      <span class="data-frag" style="font-size:10px;">SUBJECT</span>
      <input v-model="subject" type="text" class="hud-input" data-testid="composer-subject" >
    </label>

    <label style="display:grid;gap:4px;">
      <span class="data-frag" style="font-size:10px;">BODY</span>
      <textarea v-model="body" class="hud-textarea" rows="10" data-testid="composer-body"/>
      <span class="data-frag" style="font-size:10px;opacity:.7;" data-testid="composer-vars">{{ variableHints }}</span>
    </label>

    <label style="display:grid;gap:4px;">
      <span class="data-frag" style="font-size:10px;">ATTACH EPK (OPTIONAL)</span>
      <select v-model="epkExportId" class="hud-input" data-testid="composer-epk">
        <option value="">— no attachment —</option>
        <option v-for="e in epkExports" :key="e.id" :value="e.id">
          {{ e.garageObjectKey.split('/').pop() }} · {{ new Date(e.createdAt).toLocaleDateString() }}
        </option>
      </select>
    </label>

    <div class="hud-card" style="padding:10px 12px;display:grid;gap:6px;" data-testid="composer-preview">
      <span class="data-frag" style="font-size:10px;">PREVIEW</span>
      <span style="font-size:13px;font-weight:600;">{{ previewSubject }}</span>
      <pre style="margin:0;font-family:var(--font-data);font-size:12px;white-space:pre-wrap;word-break:break-word;">{{ previewBody }}</pre>
      <span v-if="selectedEpk" class="badge-hud badge-draft" data-testid="composer-epk-badge">
        <FileText style="width:10px;height:10px;" aria-hidden="true" /> {{ selectedEpk.garageObjectKey.split('/').pop() }} ATTACHED
      </span>
    </div>

    <p v-if="notice" role="status" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-ready, #2fbf71);font-size:13px;margin:0;" data-testid="composer-notice">{{ notice }}</p>
    <p v-if="errorText" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;margin:0;" data-testid="composer-error">{{ errorText }}</p>

    <div>
      <button
        type="button" class="btn-hud btn-hud-cta" style="min-height:40px;" :disabled="sending || requestsSec.loading"
        data-testid="composer-send" @click="send"
      >
        <Send style="width:12px;height:12px;" aria-hidden="true" /> {{ sending ? 'SENDING…' : 'SEND REQUEST' }}
      </button>
    </div>
  </section>

  <section v-else class="glass" style="padding:14px;" data-testid="composer-empty">
    <KhEmptyState title="NO CONTACT SELECTED" hint="Pick a matched promoter contact to compose a booking request." />
  </section>
</template>
