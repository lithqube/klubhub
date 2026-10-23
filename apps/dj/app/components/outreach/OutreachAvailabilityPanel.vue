<script setup lang="ts">
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { CalendarPlus, Trash2 } from 'lucide-vue-next'
import { useOutreachStore } from '~/stores/outreach'
import { formatSlotDate, formatSlotFee, textToGenres } from '~/utils/outreach'

/**
 * Availability (04.10-04): the DJ's open dates — a date + city the DJ can be
 * booked in. Slots drive the match list (promoter contacts in the same city).
 */
const store = useOutreachStore()
const { slots, slotsSec } = storeToRefs(store)

const adding = ref(false)
const date = ref('')
const city = ref('')
const country = ref('DE')
const genres = ref('')
const feeMin = ref('')
const feeMax = ref('')
const currency = ref('EUR')
const notes = ref('')
const formError = ref('')

function resetForm() {
  date.value = ''
  city.value = ''
  country.value = 'DE'
  genres.value = ''
  feeMin.value = ''
  feeMax.value = ''
  currency.value = 'EUR'
  notes.value = ''
  formError.value = ''
}

async function save() {
  formError.value = ''
  if (!date.value) { formError.value = 'Pick a date.'; return }
  if (!city.value.trim()) { formError.value = 'City is required.'; return }
  const min = feeMin.value === '' ? null : Number(feeMin.value)
  const max = feeMax.value === '' ? null : Number(feeMax.value)
  if (min != null && Number.isNaN(min)) { formError.value = 'Fee min must be a number.'; return }
  if (max != null && Number.isNaN(max)) { formError.value = 'Fee max must be a number.'; return }
  try {
    await store.addSlot({
      date: date.value,
      city: city.value.trim(),
      country: country.value.trim(),
      genres: textToGenres(genres.value),
      fee_min: min,
      fee_max: max,
      currency: currency.value.trim() || null,
      notes: notes.value.trim(),
    })
    adding.value = false
    resetForm()
  } catch (e) {
    formError.value = (e as { data?: { error?: unknown } })?.data?.error as string || 'Could not save the slot.'
  }
}

async function remove(id: string) {
  if (!window.confirm('Delete this availability slot?')) return
  await store.removeSlot(id).catch(() => { /* slotsSec.error shows it */ })
}
</script>

<template>
  <section aria-labelledby="avail-h" class="space-y-2">
    <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;">
      <h2 id="avail-h" class="section-lbl" style="margin:0;">AVAILABILITY</h2>
      <button
        v-if="!adding" type="button" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:36px;"
        data-testid="slot-add-open" @click="adding = true"
      >
        <CalendarPlus style="width:12px;height:12px;" aria-hidden="true" /> ADD SLOT
      </button>
    </div>

    <form v-if="adding" class="hud-card glass" style="padding:14px;display:grid;gap:10px;" data-testid="slot-form" @submit.prevent="save">
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;">
        <label style="display:grid;gap:4px;">
          <span class="data-frag" style="font-size:10px;">DATE</span>
          <input v-model="date" type="date" class="hud-input" data-testid="slot-date" >
        </label>
        <label style="display:grid;gap:4px;">
          <span class="data-frag" style="font-size:10px;">CITY</span>
          <input v-model="city" type="text" class="hud-input" placeholder="Berlin" data-testid="slot-city" >
        </label>
      </div>
      <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:10px;">
        <label style="display:grid;gap:4px;">
          <span class="data-frag" style="font-size:10px;">COUNTRY</span>
          <input v-model="country" type="text" class="hud-input" data-testid="slot-country" >
        </label>
        <label style="display:grid;gap:4px;">
          <span class="data-frag" style="font-size:10px;">FEE MIN</span>
          <input v-model="feeMin" type="number" min="0" step="0.01" class="hud-input" placeholder="800" data-testid="slot-fee-min" >
        </label>
        <label style="display:grid;gap:4px;">
          <span class="data-frag" style="font-size:10px;">FEE MAX</span>
          <input v-model="feeMax" type="number" min="0" step="0.01" class="hud-input" placeholder="1200" data-testid="slot-fee-max" >
        </label>
      </div>
      <div style="display:grid;grid-template-columns:2fr 1fr;gap:10px;">
        <label style="display:grid;gap:4px;">
          <span class="data-frag" style="font-size:10px;">GENRES (COMMA-SEPARATED)</span>
          <input v-model="genres" type="text" class="hud-input" placeholder="techno, hard techno" data-testid="slot-genres" >
        </label>
        <label style="display:grid;gap:4px;">
          <span class="data-frag" style="font-size:10px;">CURRENCY</span>
          <select v-model="currency" class="hud-input" data-testid="slot-currency">
            <option value="EUR">EUR</option>
            <option value="USD">USD</option>
            <option value="GBP">GBP</option>
            <option value="CHF">CHF</option>
          </select>
        </label>
      </div>
      <label style="display:grid;gap:4px;">
        <span class="data-frag" style="font-size:10px;">NOTES</span>
        <textarea v-model="notes" class="hud-textarea" rows="2" data-testid="slot-notes"/>
      </label>
      <p v-if="formError" role="alert" class="data-frag" style="font-size:11px;color:var(--color-error);margin:0;" data-testid="slot-error">{{ formError }}</p>
      <div style="display:flex;gap:8px;">
        <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:36px;" data-testid="slot-save">SAVE SLOT</button>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:36px;" data-testid="slot-cancel" @click="adding = false; resetForm()">CANCEL</button>
      </div>
    </form>

    <p v-if="slotsSec.loading && !slots.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
    <KhEmptyState v-else-if="!slots.length" title="NO AVAILABILITY YET" hint="Add an open date: promoter contacts in the same city show up as matches." />
    <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
      <li
        v-for="s in slots" :key="s.id" class="hud-card"
        style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;"
        data-testid="slot-row"
      >
        <span style="min-width:0;">
          <span style="display:block;font-size:14px;font-weight:600;">{{ formatSlotDate(s.date) }} · {{ s.city }}{{ s.country ? `, ${s.country}` : '' }}</span>
          <span class="data-frag" style="font-size:11px;">
            {{ s.genres.join(', ') || 'any genre' }}
            <template v-if="formatSlotFee(s)"> · {{ formatSlotFee(s) }}</template>
            <template v-if="s.notes"> · {{ s.notes }}</template>
          </span>
        </span>
        <button
          type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;flex-shrink:0;"
          aria-label="Delete slot" data-testid="slot-delete" @click="remove(s.id)"
        >
          <Trash2 style="width:12px;height:12px;" aria-hidden="true" />
        </button>
      </li>
    </ul>
  </section>
</template>
