<script setup lang="ts">
import { Download, Plus, Upload } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useAudienceStore } from '~/stores/audience'
import type { ApiError } from '~/types/event'
import { CONTACT_SOURCES, CONTACT_STATUSES, type Contact, type ContactInput, type ContactSource, type ContactStatus } from '~/types/audience'

useHead({ title: 'Audience' })

const store = useAudienceStore()
const { contacts, counts, segments, loading, error } = storeToRefs(store)

const statusTab = ref<ContactStatus | ''>('')
const sourceFilter = ref<ContactSource | ''>('')
const q = ref('')

await useAsyncData('audience', () => store.load())

async function refresh() {
  await store.load({ status: statusTab.value || undefined, source: sourceFilter.value || undefined, q: q.value || undefined })
}
watch([statusTab, sourceFilter], refresh)

const badgeClass: Record<ContactStatus, string> = {
  active: 'badge-ready',
  unsubscribed: 'badge-draft',
  bounced: 'badge-archived',
  complained: 'badge-failed',
}

const sourceLabel: Record<ContactSource, string> = {
  rsvp: 'RSVP', follow: 'FOLLOW', notify_me: 'NOTIFY ME', csv: 'CSV IMPORT', door: 'DOOR',
}

// ---------------------------------------------------------------- add contact ---
const adding = ref(false)
const addError = ref<ApiError | null>(null)
const addSaving = ref(false)
const form = reactive({ name: '', email: '', phone: '', source: 'csv' as ContactSource, basis: 'soft_opt_in' as 'consent' | 'soft_opt_in', formText: '' })

function startAdd() {
  addError.value = null
  form.name = ''; form.email = ''; form.phone = ''; form.source = 'csv'; form.basis = 'soft_opt_in'; form.formText = ''
  adding.value = true
}

async function saveContact() {
  addSaving.value = true
  addError.value = null
  try {
    const input: ContactInput = {
      name: form.name, email: form.email, phone: form.phone, source: form.source,
      consent: { basis: form.basis, form_text: form.formText },
    }
    await store.createContact(input)
    adding.value = false
  } catch (e) {
    addError.value = e as ApiError
  } finally {
    addSaving.value = false
  }
}

async function unsubscribe(c: Contact) {
  if (!window.confirm(`Unsubscribe ${c.name || c.email}? They will no longer receive marketing sends.`)) return
  await store.setStatus(c.id, { status: 'unsubscribed' })
}

async function remove(c: Contact) {
  if (!window.confirm(`Delete ${c.name || c.email}? This removes their consent record too.`)) return
  await store.deleteContact(c.id)
}

// ---------------------------------------------------------------- CSV import ---
const importing = ref(false)
const importFile = ref<File | null>(null)
const importBasis = ref<'consent' | 'soft_opt_in'>('soft_opt_in')
const importFormText = ref('')
const importSaving = ref(false)
const importNotice = ref('')

function parseCsv(text: string): { name?: string, email?: string, phone?: string }[] {
  const lines = text.split(/\r?\n/).filter(l => l.trim())
  if (!lines.length) return []
  const header = lines[0]!.split(',').map(h => h.trim().toLowerCase())
  const nameIdx = header.indexOf('name')
  const emailIdx = header.indexOf('email')
  const phoneIdx = header.indexOf('phone')
  return lines.slice(1).map((line) => {
    const cells = line.split(',').map(c => c.trim())
    return { name: nameIdx >= 0 ? cells[nameIdx] : undefined, email: emailIdx >= 0 ? cells[emailIdx] : undefined, phone: phoneIdx >= 0 ? cells[phoneIdx] : undefined }
  })
}

async function runImport() {
  if (!importFile.value) return
  importSaving.value = true
  importNotice.value = ''
  try {
    const rows = parseCsv(await importFile.value.text())
    const res = await store.importCSV(rows, { basis: importBasis.value, form_text: importFormText.value })
    importNotice.value = `${res.added} added, ${res.duplicates} already on file, ${res.invalid} skipped (no name or email).`
    importing.value = false
  } catch (e) {
    importNotice.value = (e as ApiError).error ?? 'Import failed.'
  } finally {
    importSaving.value = false
  }
}

// ---------------------------------------------------------------- segments ---
const segmentName = ref('')
const segmentStatus = ref<ContactStatus | ''>('')
const segmentSource = ref<ContactSource | ''>('')
const segmentSaving = ref(false)

async function saveSegment() {
  if (!segmentName.value.trim()) return
  segmentSaving.value = true
  try {
    await store.createSegment({ name: segmentName.value.trim(), filter: { status: segmentStatus.value || undefined, source: segmentSource.value || undefined } })
    segmentName.value = ''; segmentStatus.value = ''; segmentSource.value = ''
  } finally {
    segmentSaving.value = false
  }
}

async function removeSegment(id: string, name: string) {
  if (!window.confirm(`Delete the "${name}" segment? Contacts are not affected.`)) return
  await store.deleteSegment(id)
}
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">AUDIENCE</h1>
        <p class="page-sub">A consent-first list of followers, past attendees and notify-me sign-ups.</p>
      </div>
      <div style="display:flex;gap:8px;">
        <a :href="store.exportUrl({ status: statusTab || undefined, source: sourceFilter || undefined })" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" data-testid="audience-export">
          <Download style="width:14px;height:14px;" aria-hidden="true" /> EXPORT
        </a>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" data-testid="audience-import-open" @click="importing = true">
          <Upload style="width:14px;height:14px;" aria-hidden="true" /> IMPORT CSV
        </button>
        <button type="button" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" data-testid="audience-add" @click="startAdd">
          <Plus style="width:14px;height:14px;" aria-hidden="true" /> ADD CONTACT
        </button>
      </div>
    </div>

    <div class="page-body space-y-4">
      <p v-if="error" role="alert" class="glass" style="padding:12px 14px;border-left:3px solid var(--color-error);font-size:13px;">
        COULD NOT LOAD THE AUDIENCE.
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="refresh">RETRY</button>
      </p>

      <section aria-labelledby="segments-h" class="space-y-2">
        <h2 id="segments-h" class="section-lbl">SEGMENTS</h2>
        <ul v-if="segments.length" style="list-style:none;margin:0;padding:0;display:flex;flex-wrap:wrap;gap:6px;">
          <li v-for="s in segments" :key="s.id" class="badge-hud" style="display:flex;align-items:center;gap:6px;">
            {{ s.name }} · {{ s.matching }}
            <button type="button" aria-label="Delete segment" style="background:none;border:0;color:inherit;cursor:pointer;padding:0;line-height:1;" @click="removeSegment(s.id, s.name)">×</button>
          </li>
        </ul>
        <form class="glass" style="padding:10px 12px;display:flex;gap:8px;flex-wrap:wrap;align-items:end;" @submit.prevent="saveSegment">
          <label style="display:grid;gap:2px;font-size:11px;">
            NAME
            <input v-model="segmentName" class="hud-input" placeholder="e.g. Berlin regulars" style="min-height:40px;">
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            STATUS
            <select v-model="segmentStatus" class="hud-input" style="min-height:40px;">
              <option value="">ANY</option>
              <option v-for="s in CONTACT_STATUSES" :key="s" :value="s">{{ s.toUpperCase() }}</option>
            </select>
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            SOURCE
            <select v-model="segmentSource" class="hud-input" style="min-height:40px;">
              <option value="">ANY</option>
              <option v-for="s in CONTACT_SOURCES" :key="s" :value="s">{{ sourceLabel[s] }}</option>
            </select>
          </label>
          <button type="submit" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:40px;" :disabled="segmentSaving || !segmentName.trim()">SAVE SEGMENT</button>
        </form>
      </section>

      <section aria-labelledby="contacts-h" class="space-y-2">
        <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px;">
          <h2 id="contacts-h" class="section-lbl">CONTACTS</h2>
          <input v-model="q" class="hud-input" placeholder="SEARCH NAME OR EMAIL" style="min-height:40px;max-width:220px;" @keyup.enter="refresh" @blur="refresh">
        </div>
        <div role="tablist" aria-label="Contact status" style="display:flex;gap:4px;flex-wrap:wrap;">
          <button type="button" role="tab" :aria-selected="statusTab === ''" class="btn-hud btn-hud-xs" :class="statusTab === '' ? 'btn-hud-cta' : 'btn-hud-ghost'" @click="statusTab = ''">
            ALL {{ counts.all }}
          </button>
          <button
            v-for="s in CONTACT_STATUSES" :key="s" type="button" role="tab" :aria-selected="statusTab === s"
            class="btn-hud btn-hud-xs" :class="statusTab === s ? 'btn-hud-cta' : 'btn-hud-ghost'" @click="statusTab = s"
          >
            {{ s.toUpperCase() }} {{ counts[s] }}
          </button>
        </div>

        <p v-if="loading && !contacts.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
        <KhEmptyState v-else-if="!contacts.length" title="NO CONTACTS YET" hint="Add one, or import a CSV of past attendees." action-label="ADD CONTACT" @action="startAdd" />
        <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
          <li v-for="c in contacts" :key="c.id" class="hud-card" style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;">
            <span style="min-width:0;">
              <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ c.name || c.email }}</span>
              <span class="data-frag" style="font-size:11px;">
                <template v-if="c.name && c.email">{{ c.email }} · </template>{{ sourceLabel[c.source] }}
              </span>
            </span>
            <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
              <span class="badge-hud" :class="badgeClass[c.status]">{{ c.status.toUpperCase() }}</span>
              <button v-if="c.status === 'active'" type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" @click="unsubscribe(c)">UNSUBSCRIBE</button>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" aria-label="Delete contact" @click="remove(c)">DELETE</button>
            </span>
          </li>
        </ul>
      </section>
    </div>

    <Dialog v-model:open="adding">
      <DialogContent>
        <DialogTitle>ADD CONTACT</DialogTitle>
        <DialogDescription>Adds one contact to the audience, with the consent record required to email them later.</DialogDescription>
        <form style="display:grid;gap:10px;" @submit.prevent="saveContact">
          <p v-if="addError" role="alert" style="font-size:12px;color:var(--color-error);">{{ addError.problem || addError.error }}</p>
          <label style="display:grid;gap:2px;font-size:11px;">NAME<input v-model="form.name" class="hud-input" style="min-height:44px;"></label>
          <label style="display:grid;gap:2px;font-size:11px;">EMAIL<input v-model="form.email" type="email" class="hud-input" style="min-height:44px;"></label>
          <label style="display:grid;gap:2px;font-size:11px;">PHONE<input v-model="form.phone" class="hud-input" style="min-height:44px;"></label>
          <label style="display:grid;gap:2px;font-size:11px;">
            SOURCE
            <select v-model="form.source" class="hud-input" style="min-height:44px;">
              <option v-for="s in CONTACT_SOURCES" :key="s" :value="s">{{ sourceLabel[s] }}</option>
            </select>
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            LAWFUL BASIS
            <select v-model="form.basis" class="hud-input" style="min-height:44px;">
              <option value="consent">CONSENT</option>
              <option value="soft_opt_in">SOFT OPT-IN (past customer)</option>
            </select>
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            FORM TEXT SHOWN (compliance record)
            <textarea v-model="form.formText" class="hud-textarea" rows="2" placeholder="e.g. Sign up to hear about future nights" />
          </label>
          <div style="display:flex;justify-content:flex-end;gap:8px;">
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="adding = false">CANCEL</button>
            <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" :disabled="addSaving">SAVE</button>
          </div>
        </form>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="importing">
      <DialogContent>
        <DialogTitle>IMPORT CSV</DialogTitle>
        <DialogDescription>Columns: name, email, phone. Every imported row shares one consent record below.</DialogDescription>
        <form style="display:grid;gap:10px;" @submit.prevent="runImport">
          <input type="file" accept=".csv,text/csv" class="hud-input" style="min-height:44px;" @change="importFile = ($event.target as HTMLInputElement).files?.[0] ?? null">
          <label style="display:grid;gap:2px;font-size:11px;">
            LAWFUL BASIS FOR THIS BATCH
            <select v-model="importBasis" class="hud-input" style="min-height:44px;">
              <option value="soft_opt_in">SOFT OPT-IN (past customers)</option>
              <option value="consent">CONSENT</option>
            </select>
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            FORM TEXT / SOURCE NOTE (compliance record)
            <textarea v-model="importFormText" class="hud-textarea" rows="2" placeholder="e.g. Door sign-up sheet, Klubnacht 04" />
          </label>
          <p v-if="importNotice" role="status" class="data-frag" style="font-size:11px;">{{ importNotice }}</p>
          <div style="display:flex;justify-content:flex-end;gap:8px;">
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="importing = false">CANCEL</button>
            <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" :disabled="importSaving || !importFile">IMPORT</button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>
