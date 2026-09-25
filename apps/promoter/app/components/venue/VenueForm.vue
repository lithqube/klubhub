<script setup lang="ts">
import { ArrowDown, ArrowUp, Lock, Plus, X } from 'lucide-vue-next'
import type { ApiError, Venue, VenueInput, VenueProtected } from '~/types/event'
import { looksLikeContact } from '~/utils/pii'

const props = defineProps<{
  initial?: Venue | null
  busy?: boolean
  serverError?: ApiError | null
  /** Decrypted protected fields after an audited reveal; null while masked. */
  revealed?: VenueProtected | null
  revealing?: boolean
  revealError?: string | null
  readOnly?: boolean
  submitLabel: string
}>()
const emit = defineEmits<{ submit: [VenueInput], cancel: [], reveal: [] }>()

const deviceTz = Intl.DateTimeFormat().resolvedOptions().timeZone
const zones = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [deviceTz, 'UTC']

const v = props.initial
const f = reactive({
  name: v?.name ?? '',
  city: v?.city ?? '',
  country: v?.country ?? '',
  timezone: v?.timezone ?? deviceTz,
  capacity: v?.capacity ?? null as number | null,
  curfew: v?.curfew_local ?? '',
  techNotes: v?.tech_notes ?? '',
  rooms: (v?.rooms ?? []).map(r => ({ id: r.id as string | undefined, name: r.name, capacity: r.capacity })),
})
const emptyProtected = (): VenueProtected => ({ address: '', geo: '', contact_name: '', contact_email: '', contact_phone: '' })
const p = reactive<VenueProtected>(emptyProtected())
watch(() => props.revealed, (r) => {
  if (r) Object.assign(p, r)
}, { immediate: true })

const hasSealed = computed(() => !!v && (v.protected.address || v.protected.geo || v.protected.contact))
/** Editing replaces all sealed fields, so existing values must be revealed first. */
const protectedEditable = computed(() => !props.readOnly && (!hasSealed.value || !!props.revealed))
const protectedDirty = computed(() => (Object.keys(p) as (keyof VenueProtected)[]).some(k => p[k] !== (props.revealed?.[k] ?? '')))

const touched = reactive<Record<string, boolean>>({})
const errors = computed(() => {
  const e: Record<string, string> = {}
  if (!f.name.trim()) e.name = 'Name the venue.'
  if (f.country && !/^[A-Za-z]{2}$/.test(f.country)) e.country = 'Two letters, e.g. DE.'
  if (f.capacity !== null && (!Number.isInteger(f.capacity) || f.capacity <= 0)) e.capacity = 'A whole number above 0.'
  f.rooms.forEach((r, i) => {
    if (!r.name.trim()) e[`rooms[${i}].name`] = 'Name the room or remove it.'
  })
  if (p.contact_email && !/^[^\s@]+@[^\s@]+$/.test(p.contact_email)) e.contact_email = 'Check the email address.'
  const s = props.serverError
  if (s?.field && !e[s.field]) e[s.field] = s.problem ? s.problem[0]!.toUpperCase() + s.problem.slice(1) + '.' : 'Check this field.'
  return e
})
const show = (k: string) => (touched[k] || props.serverError?.field === k ? errors.value[k] : undefined)
const pii = computed(() => looksLikeContact(f.techNotes))

function move(i: number, d: -1 | 1) {
  const j = i + d
  if (j < 0 || j >= f.rooms.length) return
  const [r] = f.rooms.splice(i, 1)
  f.rooms.splice(j, 0, r!)
}
function addRoom() {
  f.rooms.push({ id: undefined, name: '', capacity: null })
  nextTick(() => document.getElementById(`vf-rooms[${f.rooms.length - 1}].name`)?.focus())
}

function submit() {
  for (const k of ['name', 'country', 'capacity', 'contact_email', ...f.rooms.map((_, i) => `rooms[${i}].name`)]) touched[k] = true
  const first = Object.keys(errors.value)[0]
  if (first) {
    document.getElementById(`vf-${first}`)?.focus()
    return
  }
  const sendProtected = protectedEditable.value && (hasSealed.value ? protectedDirty.value : Object.values(p).some(Boolean))
  emit('submit', {
    name: f.name.trim(),
    city: f.city.trim(),
    country: f.country ? f.country.toUpperCase() : null,
    timezone: f.timezone,
    capacity: f.capacity || null,
    curfew_local: f.curfew || null,
    tech_notes: f.techNotes,
    rooms: f.rooms.map(r => ({ id: r.id, name: r.name.trim(), capacity: r.capacity || null })),
    protected: sendProtected ? { ...p } : null,
  })
}

const PROTECTED: { key: keyof VenueProtected, label: string, flag: 'address' | 'geo' | 'contact', type?: string, hint: string }[] = [
  { key: 'address', label: 'ADDRESS', flag: 'address', hint: 'Exported only when an event shows the venue.' },
  { key: 'geo', label: 'MAP PIN', flag: 'geo', hint: 'Coordinates or a map link. Exported with the address.' },
  { key: 'contact_name', label: 'CONTACT NAME', flag: 'contact', hint: 'Never exported.' },
  { key: 'contact_email', label: 'CONTACT EMAIL', flag: 'contact', type: 'email', hint: 'Never exported.' },
  { key: 'contact_phone', label: 'CONTACT PHONE', flag: 'contact', type: 'tel', hint: 'Never exported.' },
]
</script>

<template>
  <form class="space-y-3" novalidate @submit.prevent="submit">
    <fieldset :disabled="readOnly" style="border:0;padding:0;margin:0;" class="space-y-3">
      <section class="glass" style="padding:18px;display:grid;gap:10px;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));" aria-label="Venue">
        <label style="grid-column:1/-1;">
          <span class="section-lbl">NAME *</span>
          <input id="vf-name" v-model="f.name" class="hud-input" required maxlength="200" :aria-invalid="!!show('name')" @blur="touched.name = true">
          <span v-if="show('name')" style="font-size:12px;color:var(--color-error);">{{ show('name') }}</span>
        </label>
        <label><span class="section-lbl">CITY</span><input id="vf-city" v-model="f.city" class="hud-input" maxlength="120"></label>
        <label>
          <span class="section-lbl">COUNTRY</span>
          <input id="vf-country" v-model="f.country" class="hud-input" maxlength="2" placeholder="DE" autocapitalize="characters" :aria-invalid="!!show('country')" @blur="touched.country = true">
          <span v-if="show('country')" style="font-size:12px;color:var(--color-error);">{{ show('country') }}</span>
        </label>
        <label>
          <span class="section-lbl">TIMEZONE</span>
          <select id="vf-timezone" v-model="f.timezone" class="hud-input"><option v-for="z in zones" :key="z" :value="z">{{ z }}</option></select>
        </label>
        <label>
          <span class="section-lbl">CAPACITY</span>
          <input id="vf-capacity" v-model.number="f.capacity" class="hud-input" type="number" min="1" :aria-invalid="!!show('capacity')" @blur="touched.capacity = true">
          <span v-if="show('capacity')" style="font-size:12px;color:var(--color-error);">{{ show('capacity') }}</span>
        </label>
        <label>
          <span class="section-lbl">CURFEW (LOCAL)</span>
          <input id="vf-curfew_local" v-model="f.curfew" class="hud-input" type="time">
          <span style="font-size:11px;color:var(--color-on-surface-variant);">New events here get this curfew on every stage.</span>
        </label>
      </section>

      <section class="glass" style="padding:18px;" aria-labelledby="rooms-h">
        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;">
          <h2 id="rooms-h" class="section-lbl" style="margin:0;">ROOMS · {{ f.rooms.length }}</h2>
          <button v-if="!readOnly" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="addRoom"><Plus :size="14" aria-hidden="true" /> ADD ROOM</button>
        </div>
        <p style="margin:6px 0 10px;font-size:12px;color:var(--color-on-surface-variant);">Rooms become the stages of new events at this venue.</p>
        <p v-if="!f.rooms.length" style="font-size:13px;margin:0;">No rooms. Events here start with one stage.</p>
        <ol style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
          <li v-for="(r, i) in f.rooms" :key="r.id ?? `new-${i}`" style="display:grid;grid-template-columns:minmax(0,1fr) 84px auto;gap:6px;align-items:start;">
            <label>
              <span class="sr-only">Room {{ i + 1 }} name</span>
              <input :id="`vf-rooms[${i}].name`" v-model="r.name" class="hud-input" maxlength="80" placeholder="Main Room" :aria-invalid="!!show(`rooms[${i}].name`)" @blur="touched[`rooms[${i}].name`] = true">
              <span v-if="show(`rooms[${i}].name`)" style="font-size:12px;color:var(--color-error);">{{ show(`rooms[${i}].name`) }}</span>
            </label>
            <label><span class="sr-only">Room {{ i + 1 }} capacity</span><input v-model.number="r.capacity" class="hud-input" type="number" min="1" placeholder="CAP."></label>
            <div v-if="!readOnly" style="display:flex;">
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :disabled="i === 0" :aria-label="`Move ${r.name || 'room'} up`" @click="move(i, -1)"><ArrowUp :size="14" /></button>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :disabled="i === f.rooms.length - 1" :aria-label="`Move ${r.name || 'room'} down`" @click="move(i, 1)"><ArrowDown :size="14" /></button>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Remove ${r.name || 'room'}`" @click="f.rooms.splice(i, 1)"><X :size="14" /></button>
            </div>
          </li>
        </ol>
      </section>

      <section class="glass" style="padding:18px;" aria-labelledby="tech-h">
        <label>
          <span id="tech-h" class="section-lbl">TECH NOTES</span>
          <textarea id="vf-tech_notes" v-model="f.techNotes" class="hud-textarea" rows="4" maxlength="5000" placeholder="Decks, mixer, sound system, load-in" aria-describedby="tech-hint" />
        </label>
        <p id="tech-hint" style="margin:6px 0 0;font-size:12px;" :style="pii ? 'color:var(--color-secondary);font-weight:600;' : 'color:var(--color-on-surface-variant);'" aria-live="polite">
          <template v-if="pii">This looks like {{ pii === 'email' ? 'an email address' : 'a phone number' }}. Tech notes are not encrypted: put contacts in the protected fields below.</template>
          <template v-else>Internal. Shared with your team, never exported. Not encrypted.</template>
        </p>
      </section>
    </fieldset>

    <section class="glass" style="padding:18px;border-left:3px solid var(--color-primary);" aria-labelledby="prot-h">
      <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;">
        <h2 id="prot-h" class="section-lbl" style="margin:0;display:flex;align-items:center;gap:6px;"><Lock :size="12" aria-hidden="true" /> PROTECTED · ENCRYPTED</h2>
        <button
          v-if="hasSealed && !revealed" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;"
          :disabled="revealing" @click="emit('reveal')"
        >
          {{ revealing ? 'REVEALING…' : readOnly ? 'SHOW' : 'SHOW TO EDIT' }}
        </button>
      </div>
      <p style="margin:6px 0 10px;font-size:12px;color:var(--color-on-surface-variant);">
        Stored encrypted with your organisation's key. Showing them needs two-factor sign-in and is recorded in the audit log.
      </p>
      <p v-if="revealError" role="alert" style="margin:0 0 10px;font-size:13px;color:var(--color-error);">
        {{ revealError }}
        <NuxtLink v-if="revealError.includes('two-factor')" to="/account/security" style="color:var(--color-primary);">Set up two-factor →</NuxtLink>
      </p>
      <div style="display:grid;gap:10px;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));">
        <label v-for="row in PROTECTED" :key="row.key" :style="row.key === 'address' ? 'grid-column:1/-1;' : ''">
          <span class="section-lbl">{{ row.label }}</span>
          <input
            v-if="protectedEditable" :id="`vf-${row.key}`" v-model="p[row.key]" class="hud-input" :type="row.type ?? 'text'"
            autocomplete="off" :aria-invalid="!!show(row.key)" @blur="touched[row.key] = true"
          >
          <div v-else class="hud-input" :aria-label="`${row.label}: ${initial?.protected[row.flag] ? 'hidden' : 'not set'}`" style="display:flex;align-items:center;color:var(--color-on-surface-variant);">
            {{ revealed ? (revealed[row.key] || '—') : initial?.protected[row.flag] ? '••••••••' : '—' }}
          </div>
          <span v-if="show(row.key)" style="font-size:12px;color:var(--color-error);">{{ show(row.key) }}</span>
          <span v-else style="font-size:11px;color:var(--color-on-surface-variant);">{{ row.hint }}</span>
        </label>
      </div>
    </section>

    <div v-if="!readOnly" style="display:flex;gap:8px;justify-content:flex-end;">
      <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="emit('cancel')">CANCEL</button>
      <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="busy">{{ busy ? 'SAVING…' : submitLabel }}</button>
    </div>
  </form>
</template>
