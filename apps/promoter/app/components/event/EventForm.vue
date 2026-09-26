<script setup lang="ts">
import type { ApiError, EventDetail, EventInput, LocationMode, Venue, Visibility } from '~/types/event'
import { durationLabel, instantToZoned, resolveNight, zoneAbbrev, zonedToInstant } from '~/utils/datetime'

/**
 * Create / edit an event (UX §4.3). Wall-clock inputs in the event
 * timezone resolve to instants; an end at or before the start is the next
 * day. Server validation errors map back to fields via `serverError`.
 */
const props = defineProps<{
  venues: Venue[]
  initial?: EventDetail | null
  busy?: boolean
  serverError?: ApiError | null
  submitLabel: string
}>()
const emit = defineEmits<{ submit: [input: EventInput], cancel: [] }>()

const deviceTz = Intl.DateTimeFormat().resolvedOptions().timeZone
const zones = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [deviceTz, 'UTC']

function fromInitial() {
  const e = props.initial
  const tz = e?.timezone ?? deviceTz
  const s = e ? instantToZoned(e.starts_at, tz) : null
  const reveal = e?.location_reveal_at ? instantToZoned(e.location_reveal_at, tz) : null
  const publish = e?.publish_at ? instantToZoned(e.publish_at, tz) : null
  return {
    title: e?.title ?? '',
    date: s?.date ?? '',
    start: s?.time ?? '23:00',
    end: e ? instantToZoned(e.ends_at, tz).time : '06:00',
    doors: e?.doors_at ? instantToZoned(e.doors_at, tz).time : '',
    timezone: tz,
    venueId: e?.venue_id ?? '',
    city: e?.city ?? '',
    locationMode: (e?.location_mode ?? 'venue') as LocationMode,
    revealDate: reveal?.date ?? '',
    revealTime: reveal?.time ?? '12:00',
    visibility: (e?.visibility ?? 'public') as Visibility,
    publishDate: publish?.date ?? '',
    publishTime: publish?.time ?? '18:00',
    minAge: e?.min_age ?? null as number | null,
    genres: (e?.genres ?? []).join(', '),
    cost: e?.cost_text ?? '',
    ticketUrl: e?.external_ticket_url ?? '',
    capacity: e?.capacity ?? null as number | null,
    description: e?.description_md ?? '',
  }
}
const f = reactive(fromInitial())
const touched = reactive<Record<string, boolean>>({})

const venue = computed(() => props.venues.find(v => v.id === f.venueId) ?? null)
watch(() => f.venueId, () => {
  if (venue.value) {
    f.timezone = venue.value.timezone
    if (!f.city) f.city = venue.value.city
  }
})

const night = computed(() => (f.date && f.start && f.end ? resolveNight(f.date, f.start, f.end, f.doors || null, f.timezone) : null))
const nextDayName = computed(() => (night.value?.overnight
  ? new Intl.DateTimeFormat('en-GB', { timeZone: f.timezone, weekday: 'short' }).format(new Date(night.value.ends_at)).toUpperCase()
  : ''))
const zone = computed(() => (night.value ? zoneAbbrev(night.value.starts_at, f.timezone) : ''))

const errors = computed(() => {
  const e: Record<string, string> = {}
  if (!f.title.trim()) e.title = 'Give the night a title.'
  if (!f.date) e.date = 'Pick the date the night starts.'
  if (!f.venueId && !f.city.trim()) e.city = 'Choose a venue or enter a city.'
  if (f.ticketUrl && !/^https:\/\/\S+$/.test(f.ticketUrl)) e.ticketUrl = 'Use an https:// link.'
  if (f.locationMode === 'secret' && f.revealDate && !f.revealTime) e.reveal = 'Set a reveal time.'
  if (props.serverError?.field) {
    const map: Record<string, string> = { title: 'title', starts_at: 'date', ends_at: 'date', city: 'city', external_ticket_url: 'ticketUrl', venue_id: 'city', timezone: 'timezone' }
    e[map[props.serverError.field] ?? 'form'] = props.serverError.problem ?? 'Check this field.'
  }
  return e
})
const submitted = ref(false)
const summary = ref<HTMLElement | null>(null)
const show = (k: string) => (touched[k] || submitted.value) && errors.value[k]

const previewLocation = computed(() => {
  const city = f.city || venue.value?.city || '—'
  if (f.locationMode === 'city_only') return { now: city, later: null }
  if (f.locationMode === 'secret') {
    return {
      now: `${city.toUpperCase()} — LOCATION TBA`,
      later: f.revealDate ? `from ${f.revealDate} ${f.revealTime}: ${venue.value?.name ?? city}` : 'hidden until you change it',
    }
  }
  return { now: venue.value ? `${venue.value.name}, ${city}` : city, later: null }
})

async function submit() {
  submitted.value = true
  if (Object.keys(errors.value).filter(k => k !== 'form').length || !night.value) {
    await nextTick()
    summary.value?.focus()
    return
  }
  const n = night.value
  emit('submit', {
    title: f.title.trim(),
    starts_at: n.starts_at, ends_at: n.ends_at, doors_at: n.doors_at,
    timezone: f.timezone,
    venue_id: f.venueId || null,
    city: f.city.trim(),
    location_mode: f.locationMode,
    location_reveal_at: f.locationMode === 'secret' && f.revealDate ? zonedToInstant(f.revealDate, f.revealTime, f.timezone).toISOString() : null,
    visibility: f.visibility,
    publish_at: f.publishDate ? zonedToInstant(f.publishDate, f.publishTime, f.timezone).toISOString() : null,
    min_age: f.minAge,
    genres: f.genres.split(',').map(g => g.trim()).filter(Boolean),
    description_md: f.description,
    cost_text: f.cost,
    external_ticket_url: f.ticketUrl || null,
    capacity: f.capacity,
  })
}
watch(() => props.serverError, async (e) => {
  if (e) {
    submitted.value = true
    await nextTick()
    summary.value?.focus()
  }
})
</script>

<template>
  <form novalidate class="space-y-4" @submit.prevent="submit">
    <div
      v-if="submitted && Object.keys(errors).length"
      ref="summary" tabindex="-1" role="alert" class="glass accent-bar-failed"
      style="padding:12px 16px;border-left:3px solid var(--color-error);"
    >
      <div class="section-lbl">FIX {{ Object.keys(errors).length }} {{ Object.keys(errors).length === 1 ? 'THING' : 'THINGS' }} TO SAVE</div>
      <ul style="margin:6px 0 0;padding-left:18px;font-size:13px;">
        <li v-for="(msg, key) in errors" :key="key"><a :href="`#f-${key}`" style="color:inherit;">{{ msg }}</a></li>
      </ul>
    </div>

    <div style="display:grid;gap:16px;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));align-items:start;">
      <section class="glass hud-card space-y-3" style="padding:18px;" aria-labelledby="essentials">
        <h2 id="essentials" class="section-lbl">ESSENTIALS</h2>
        <label class="block">
          <span class="section-lbl">TITLE *</span>
          <input id="f-title" v-model="f.title" class="hud-input" required :aria-invalid="!!show('title')" aria-describedby="e-title" @blur="touched.title = true">
          <span v-if="show('title')" id="e-title" style="font-size:12px;color:var(--color-error);">{{ errors.title }}</span>
        </label>
        <label class="block">
          <span class="section-lbl">DATE *</span>
          <input id="f-date" v-model="f.date" class="hud-input" type="date" required :aria-invalid="!!show('date')" @blur="touched.date = true">
          <span v-if="show('date')" style="font-size:12px;color:var(--color-error);">{{ errors.date }}</span>
        </label>
        <div style="display:grid;grid-template-columns:repeat(3,1fr);gap:8px;">
          <label><span class="section-lbl">DOORS</span><input v-model="f.doors" class="hud-input" type="time"></label>
          <label><span class="section-lbl">STARTS *</span><input v-model="f.start" class="hud-input" type="time" required></label>
          <label>
            <span class="section-lbl">ENDS *</span>
            <input v-model="f.end" class="hud-input" type="time" required aria-describedby="night-hint">
          </label>
        </div>
        <p v-if="night" id="night-hint" style="margin:0;font-size:12px;color:var(--color-on-surface-variant);" aria-live="polite">
          <template v-if="night.overnight">Ends +1 {{ nextDayName }} · </template>{{ durationLabel(night.durationMinutes) }} · times in {{ f.timezone }} ({{ zone }})
          <strong v-if="night.dstShiftMinutes" style="display:block;color:var(--color-secondary);">
            CLOCKS CHANGE THIS NIGHT — IT IS {{ durationLabel(night.durationMinutes) }}, NOT {{ durationLabel(night.durationMinutes - night.dstShiftMinutes) }}.
          </strong>
        </p>
        <p v-if="f.timezone !== deviceTz" style="margin:0;font-size:12px;color:var(--color-tertiary);">
          Your device is in {{ deviceTz }}; times above are {{ f.timezone }}.
        </p>
        <label class="block">
          <span class="section-lbl">VENUE</span>
          <select v-model="f.venueId" class="hud-input">
            <option value="">No venue (city only)</option>
            <option v-for="v in venues" :key="v.id" :value="v.id">{{ v.name }} · {{ v.city }}</option>
          </select>
        </label>
        <label class="block">
          <span class="section-lbl">CITY {{ f.venueId ? '' : '*' }}</span>
          <input id="f-city" v-model="f.city" class="hud-input" :aria-invalid="!!show('city')" @blur="touched.city = true">
          <span v-if="show('city')" style="font-size:12px;color:var(--color-error);">{{ errors.city }}</span>
        </label>
        <label class="block">
          <span class="section-lbl">TIMEZONE</span>
          <select id="f-timezone" v-model="f.timezone" class="hud-input">
            <option v-for="z in zones" :key="z" :value="z">{{ z }}</option>
          </select>
        </label>
      </section>

      <aside class="glass" style="padding:18px;" aria-labelledby="public-sees">
        <h2 id="public-sees" class="section-lbl">PUBLIC SEES</h2>
        <div style="margin-top:8px;font-family:var(--font-command);font-size:16px;font-weight:700;text-transform:uppercase;">{{ f.title || 'UNTITLED' }}</div>
        <div v-if="f.date" style="font-size:13px;">{{ f.date }} · {{ f.start }}–{{ f.end }}<span v-if="night?.overnight"> (+1)</span></div>
        <div style="font-size:13px;">{{ previewLocation.now }}</div>
        <div v-if="previewLocation.later" style="font-size:12px;color:var(--color-tertiary);">then: {{ previewLocation.later }}</div>
        <div class="data-frag" style="margin-top:10px;font-size:8px;">{{ f.visibility.toUpperCase() }}{{ f.publishDate ? ` · GOES LIVE ${f.publishDate} ${f.publishTime}` : '' }}</div>
      </aside>
    </div>

    <details class="glass" style="padding:14px 18px;" :open="f.locationMode !== 'venue'">
      <summary class="section-lbl" style="cursor:pointer;min-height:32px;display:flex;align-items:center;">LOCATION PRIVACY · {{ f.locationMode.replace('_', ' ').toUpperCase() }}</summary>
      <fieldset style="border:0;padding:0;margin:10px 0 0;">
        <legend class="sr-only">Location privacy</legend>
        <div class="theme-seg" role="radiogroup" aria-label="Location privacy">
          <label v-for="m in (['venue', 'city_only', 'secret'] as LocationMode[])" :key="m" class="theme-opt" :class="{ active: f.locationMode === m }" style="min-height:44px;cursor:pointer;">
            <input v-model="f.locationMode" type="radio" name="location_mode" :value="m" class="sr-only">{{ m.replace('_', ' ').toUpperCase() }}
          </label>
        </div>
        <p style="font-size:12px;color:var(--color-on-surface-variant);margin:8px 0;">
          <template v-if="f.locationMode === 'venue'">The venue is shown and exported.</template>
          <template v-else-if="f.locationMode === 'city_only'">Only the city is shown; the venue is never exported.</template>
          <template v-else>The city is shown; the venue stays hidden until the reveal time. Without a reveal time it stays hidden until you change it.</template>
        </p>
        <div v-if="f.locationMode === 'secret'" style="display:grid;grid-template-columns:1fr 1fr;gap:8px;max-width:360px;">
          <label><span class="section-lbl">REVEAL DATE</span><input v-model="f.revealDate" class="hud-input" type="date"></label>
          <label><span class="section-lbl">REVEAL TIME</span><input v-model="f.revealTime" class="hud-input" type="time"></label>
        </div>
      </fieldset>
    </details>

    <details class="glass" style="padding:14px 18px;">
      <summary class="section-lbl" style="cursor:pointer;min-height:32px;display:flex;align-items:center;">RELEASE · {{ f.visibility.toUpperCase() }}{{ f.publishDate ? ' · EMBARGOED' : '' }}</summary>
      <div style="display:grid;gap:8px;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));margin-top:10px;">
        <label><span class="section-lbl">VISIBILITY</span>
          <select v-model="f.visibility" class="hud-input"><option value="public">Public</option><option value="unlisted">Unlisted (link only)</option><option value="private">Private</option></select>
        </label>
        <label><span class="section-lbl">GO LIVE DATE</span><input v-model="f.publishDate" class="hud-input" type="date"></label>
        <label><span class="section-lbl">GO LIVE TIME</span><input v-model="f.publishTime" class="hud-input" type="time"></label>
      </div>
      <p style="font-size:12px;color:var(--color-on-surface-variant);margin:8px 0 0;">Leave the go-live date empty to publish when you choose.</p>
    </details>

    <details class="glass" style="padding:14px 18px;">
      <summary class="section-lbl" style="cursor:pointer;min-height:32px;display:flex;align-items:center;">DETAILS · GENRES · AGE · COST · TICKETS</summary>
      <div style="display:grid;gap:8px;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));margin-top:10px;">
        <label><span class="section-lbl">GENRES (COMMA-SEPARATED)</span><input v-model="f.genres" class="hud-input" placeholder="techno, house"></label>
        <label><span class="section-lbl">MIN AGE</span><input v-model.number="f.minAge" class="hud-input" type="number" min="0" max="30"></label>
        <label><span class="section-lbl">COST</span><input v-model="f.cost" class="hud-input" placeholder="€20 / €25 at the door" maxlength="200"></label>
        <label><span class="section-lbl">CAPACITY</span><input v-model.number="f.capacity" class="hud-input" type="number" min="1"></label>
        <label style="grid-column:1/-1;"><span class="section-lbl">TICKET LINK</span>
          <input id="f-ticketUrl" v-model="f.ticketUrl" class="hud-input" type="url" placeholder="https://" :aria-invalid="!!show('ticketUrl')" @blur="touched.ticketUrl = true">
          <span v-if="show('ticketUrl')" style="font-size:12px;color:var(--color-error);">{{ errors.ticketUrl }}</span>
        </label>
        <label style="grid-column:1/-1;"><span class="section-lbl">DESCRIPTION</span>
          <textarea v-model="f.description" class="hud-textarea" rows="5" maxlength="20000" />
        </label>
      </div>
    </details>

    <div style="display:flex;gap:8px;justify-content:flex-end;">
      <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="emit('cancel')">CANCEL</button>
      <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="busy">{{ busy ? 'SAVING…' : submitLabel }}</button>
    </div>
  </form>
</template>
