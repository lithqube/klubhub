<script setup lang="ts">
import { Plus, Trash2 } from 'lucide-vue-next'
import { useBookingsStore } from '~/stores/bookings'
import type { ApiError, EventSummary } from '~/types/event'
import type { OpenSlot } from '~/types/bookings'
import { formatFee } from '~/utils/bookings'

/**
 * Open slots of the selected event (04.10-05): the event picker, the slot
 * list and the add form. Slot creation and deletion go through the store;
 * selecting a slot emits it so the page loads its matches.
 */
const props = defineProps<{ eventId: string | null, events: EventSummary[], slots: OpenSlot[], loading: boolean }>()
const emit = defineEmits<{ 'update:eventId': [id: string], select: [slot: OpenSlot] }>()

const store = useBookingsStore()

const adding = ref(false)
const saving = ref(false)
const error = ref<ApiError | null>(null)
const form = reactive({ role: '', genres: '', feeMin: '', feeMax: '', currency: 'EUR', notes: '' })

const selected = ref<string | null>(null)

const canSubmit = computed(() => !!props.eventId && !saving.value)

function startAdd() {
  error.value = null
  form.role = ''; form.genres = ''; form.feeMin = ''; form.feeMax = ''; form.currency = 'EUR'; form.notes = ''
  adding.value = true
}

function parseGenres(text: string): string[] {
  return text.split(',').map(g => g.trim()).filter(Boolean)
}

function parseFee(text: string): number | null {
  const n = Number(text)
  return text.trim() !== '' && Number.isFinite(n) ? n : null
}

async function submit() {
  if (!props.eventId) return
  saving.value = true
  error.value = null
  try {
    await store.createSlot(props.eventId, {
      role: form.role.trim(),
      genres: parseGenres(form.genres),
      fee_min: parseFee(form.feeMin),
      fee_max: parseFee(form.feeMax),
      currency: form.currency.trim() || null,
      notes: form.notes.trim(),
    })
    adding.value = false
  } catch (e) {
    error.value = e as ApiError
  } finally {
    saving.value = false
  }
}

async function remove(slot: OpenSlot) {
  if (!window.confirm(`Delete the "${slot.role || 'open slot'}" slot?`)) return
  await store.deleteSlot(slot.id)
  if (selected.value === slot.id) selected.value = null
}

function pick(slot: OpenSlot) {
  selected.value = slot.id
  emit('select', slot)
}
</script>

<template>
  <section aria-labelledby="slots-h" class="space-y-2">
    <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px;">
      <h2 id="slots-h" class="section-lbl">OPEN SLOTS</h2>
      <button type="button" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" data-testid="slot-add-open" :disabled="!eventId" @click="startAdd">
        <Plus style="width:14px;height:14px;" aria-hidden="true" /> ADD SLOT
      </button>
    </div>

    <label style="display:grid;gap:2px;font-size:11px;max-width:420px;">
      EVENT
      <select :value="eventId ?? ''" class="hud-input" style="min-height:40px;" data-testid="slot-event-picker" @change="emit('update:eventId', ($event.target as HTMLSelectElement).value)">
        <option value="" disabled>SELECT AN EVENT</option>
        <option v-for="e in events" :key="e.id" :value="e.id">{{ e.title }} · {{ e.city }}</option>
      </select>
    </label>

    <p v-if="!eventId" class="data-frag" style="font-size:11px;">Pick an event to manage its open slots.</p>

    <template v-else>
      <p v-if="error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
        {{ error.error === 'invalid' ? `Invalid ${error.field ?? 'input'}: ${error.problem ?? ''}` : 'Could not save the slot.' }}
      </p>

      <form v-if="adding" class="hud-card" style="padding:12px 14px;display:grid;gap:10px;" data-testid="slot-form" novalidate @submit.prevent="submit">
        <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:8px;">
          <label style="display:grid;gap:2px;font-size:11px;">
            ROLE (OPTIONAL)
            <input v-model="form.role" class="hud-input" placeholder="e.g. Headliner" style="min-height:40px;" data-testid="slot-role">
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            GENRES (COMMA SEPARATED)
            <input v-model="form.genres" class="hud-input" placeholder="techno, hard techno" style="min-height:40px;" data-testid="slot-genres">
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            FEE MIN
            <input v-model="form.feeMin" class="hud-input" inputmode="decimal" placeholder="500" style="min-height:40px;" data-testid="slot-fee-min">
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            FEE MAX
            <input v-model="form.feeMax" class="hud-input" inputmode="decimal" placeholder="800" style="min-height:40px;" data-testid="slot-fee-max">
          </label>
          <label style="display:grid;gap:2px;font-size:11px;">
            CURRENCY
            <input v-model="form.currency" class="hud-input" placeholder="EUR" maxlength="3" style="min-height:40px;text-transform:uppercase;" data-testid="slot-currency">
          </label>
        </div>
        <label style="display:grid;gap:2px;font-size:11px;">
          NOTES
          <textarea v-model="form.notes" class="hud-input" rows="2" placeholder="Set times, billing terms…" style="min-height:60px;" data-testid="slot-notes"/>
        </label>
        <div style="display:flex;gap:8px;">
          <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" :disabled="!canSubmit" data-testid="slot-save">SAVE SLOT</button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="adding = false">CANCEL</button>
        </div>
      </form>

      <p v-if="loading && !slots.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
      <KhEmptyState v-else-if="!slots.length" title="NO OPEN SLOTS" hint="Add a slot to see which of your talent contacts match it." action-label="ADD SLOT" @action="startAdd" />
      <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
        <li v-for="s in slots" :key="s.id" class="hud-card" style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;" :style="selected === s.id ? 'border-color:var(--color-primary);' : ''" data-testid="slot-row">
          <button type="button" :aria-pressed="selected === s.id" style="min-width:0;flex:1;text-align:left;background:none;border:0;color:inherit;cursor:pointer;padding:6px;" class="slot-select" @click="pick(s)">
            <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ s.role || 'Open slot' }}</span>
            <span class="data-frag" style="font-size:11px;">
              <template v-if="s.genres.length">{{ s.genres.join(', ') }} · </template>{{ formatFee(s) || 'fee on request' }}
              <template v-if="s.notes"> · {{ s.notes }}</template>
            </span>
          </button>
          <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
            <span v-if="selected === s.id" class="badge-hud badge-ready">SELECTED</span>
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" aria-label="Delete slot" @click.stop="remove(s)">
              <Trash2 style="width:13px;height:13px;" aria-hidden="true" />
            </button>
          </span>
        </li>
      </ul>
    </template>
  </section>
</template>

<style scoped>
.slot-select:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
</style>
