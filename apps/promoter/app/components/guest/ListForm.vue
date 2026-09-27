<script setup lang="ts">
import type { ApiError } from '~/types/event'
import type { EntryTerms, ListInput, ListType, PriceMode } from '~/types/guest'
import { instantToZoned } from '~/utils/datetime'
import { cutoffInstant, guestErrorText, LIST_TYPES } from '~/utils/guests'

/**
 * Create or edit a guest list (event) or a standing list (template). Event
 * lists store an absolute cutoff; the organiser types a local time and it
 * resolves against the night, like the server does for standing lists.
 */
const props = defineProps<{
  initial?: { name: string, type: ListType, entry_terms: EntryTerms, collect_contact: boolean } | null
  template?: boolean
  event?: { starts_at: string, timezone: string } | null
  error?: ApiError | null
  saving?: boolean
  submitLabel?: string
}>()
const emit = defineEmits<{ save: [ListInput], cancel: [] }>()

const uid = useId()
const t = props.initial?.entry_terms
const f = reactive({
  name: props.initial?.name ?? '',
  type: (props.initial?.type ?? 'comp') as ListType,
  price: (t?.price_mode ?? 'free') as PriceMode,
  priceText: t?.reduced_price_text ?? '',
  cutoff: t?.cutoff_local ?? (t?.cutoff_at && props.event ? instantToZoned(t.cutoff_at, props.event.timezone).time : ''),
  perks: (t?.perks ?? []).join(', '),
  contact: props.initial?.collect_contact ?? false,
})
const wasCollecting = props.initial?.collect_contact ?? false

const errorText = computed(() => (props.error ? guestErrorText(props.error) : ''))

function submit() {
  if (wasCollecting && !f.contact && !window.confirm('Turning contact details off erases the emails and phone numbers already on this list. Continue?')) return
  const cutoff = f.cutoff || null
  const terms: EntryTerms = {
    price_mode: f.price,
    reduced_price_text: f.price === 'reduced' ? f.priceText : '',
    perks: f.perks.split(',').map(p => p.trim()).filter(Boolean),
  }
  if (props.template) terms.cutoff_local = cutoff
  else terms.cutoff_at = cutoff && props.event ? cutoffInstant(props.event.starts_at, props.event.timezone, cutoff) : null
  emit('save', { name: f.name, type: f.type, entry_terms: terms, collect_contact: f.contact })
}
</script>

<template>
  <form class="hud-card" style="padding:12px 14px;display:grid;gap:10px;" novalidate @submit.prevent="submit">
    <div class="form-grid">
      <label>
        <span class="section-lbl">NAME</span>
        <input :id="`${uid}-name`" v-model="f.name" class="hud-input" required maxlength="80" placeholder="Artist guests">
      </label>
      <label>
        <span class="section-lbl">TYPE</span>
        <select v-model="f.type" class="hud-input">
          <option v-for="x in LIST_TYPES" :key="x.id" :value="x.id">{{ x.label }}</option>
        </select>
      </label>
    </div>

    <fieldset style="border:0;margin:0;padding:0;display:grid;gap:6px;">
      <legend class="section-lbl">ENTRY</legend>
      <div style="display:flex;flex-wrap:wrap;gap:12px;align-items:center;">
        <label style="display:inline-flex;align-items:center;gap:6px;min-height:44px;font-size:13px;">
          <input v-model="f.price" type="radio" value="free" :name="`${uid}-price`"> FREE
        </label>
        <label style="display:inline-flex;align-items:center;gap:6px;min-height:44px;font-size:13px;">
          <input v-model="f.price" type="radio" value="reduced" :name="`${uid}-price`"> REDUCED
        </label>
        <label v-if="f.price === 'reduced'" style="flex:1;min-width:160px;">
          <span class="sr-only">What they pay</span>
          <input v-model="f.priceText" class="hud-input" maxlength="60" placeholder="€10 before 01:00">
        </label>
      </div>
      <div class="form-grid">
        <label>
          <span class="section-lbl">CUTOFF {{ event ? `· ${event.timezone.toUpperCase()}` : '· LOCAL TIME' }}</span>
          <input v-model="f.cutoff" class="hud-input" type="time" :aria-describedby="`${uid}-cutoff-hint`">
          <span :id="`${uid}-cutoff-hint`" style="font-size:11px;color:var(--color-on-surface-variant);">
            {{ template ? 'Each event gets this time on its own night.' : 'After this, the door sees PAST CUTOFF.' }} Leave empty for none.
          </span>
        </label>
        <label>
          <span class="section-lbl">PERKS</span>
          <input v-model="f.perks" class="hud-input" maxlength="220" placeholder="drink token, coat check">
          <span style="font-size:11px;color:var(--color-on-surface-variant);">Comma-separated, up to 8.</span>
        </label>
      </div>
    </fieldset>

    <label style="display:flex;align-items:flex-start;gap:8px;min-height:44px;font-size:13px;">
      <input v-model="f.contact" type="checkbox" style="margin-top:3px;">
      <span>
        COLLECT EMAIL &amp; PHONE
        <span style="display:block;font-size:11px;color:var(--color-on-surface-variant);">
          Off: names only (default). On: also email and phone, erased with the rest after the retention period (Settings → Data retention).
        </span>
      </span>
    </label>

    <p v-if="errorText" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ errorText }}</p>
    <div style="display:flex;flex-wrap:wrap;gap:6px;">
      <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="saving || !f.name.trim()">{{ submitLabel ?? 'SAVE LIST' }}</button>
      <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="emit('cancel')">CANCEL</button>
    </div>
  </form>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
}
@media (min-width: 640px) {
  .form-grid { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
label { display: grid; gap: 4px; }
</style>
