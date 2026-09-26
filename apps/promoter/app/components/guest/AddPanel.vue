<script setup lang="ts">
import { useGuestStore } from '~/stores/guest'
import type { ApiError } from '~/types/event'
import type { GuestInput, GuestList } from '~/types/guest'
import { allocationState, guestErrorText, MAX_PLUS_N, parsePastedGuests } from '~/utils/guests'

/**
 * Add guests: one at a time, or paste "Name +2" lines. Under an allocation
 * the server applies its rules (approval → pending, quota, deadline, +N);
 * "add directly as Going" is the staff override for approval.
 */
const props = defineProps<{ lists: GuestList[], defaultListId?: string | null }>()
const emit = defineEmits<{ done: [message: string], cancel: [] }>()
const store = useGuestStore()
const uid = useId()

const mode = ref<'one' | 'paste'>('paste')
const listId = ref(props.defaultListId ?? props.lists[0]?.id ?? '')
const allocationId = ref('')
const direct = ref(false)
const one = reactive({ name: '', plus: 0, note: '', email: '', phone: '' })
const paste = ref('')
const error = ref<ApiError | null>(null)
const saving = ref(false)

const list = computed(() => props.lists.find(l => l.id === listId.value) ?? null)
const openAllocations = computed(() => (list.value?.allocations ?? []).filter(a => allocationState(a) === 'open'))
const allocation = computed(() => openAllocations.value.find(a => a.id === allocationId.value) ?? null)
watch(listId, () => { allocationId.value = '' })

const parsed = computed(() => parsePastedGuests(paste.value))
const droppedEmails = computed(() => (list.value?.collect_contact ? 0 : parsed.value.filter(p => p.email).length))
const heads = computed(() => parsed.value.reduce((n, p) => n + 1 + p.plus_n, 0))
const maxPlus = computed(() => allocation.value?.plus_n_max ?? MAX_PLUS_N)
const overPlus = computed(() => parsed.value.filter(p => p.plus_n > maxPlus.value).map(p => p.name))
const errorText = computed(() => (error.value ? guestErrorText(error.value) : ''))

async function submit() {
  if (!list.value) return
  const status = direct.value ? 'going' as const : undefined
  const guests: GuestInput[] = mode.value === 'one'
    ? [{ name: one.name, plus_n: Number(one.plus) || 0, note: one.note, status,
        ...(list.value.collect_contact ? { email: one.email, phone: one.phone } : {}) }]
    : parsed.value.map(p => ({ name: p.name, plus_n: p.plus_n, status, ...(list.value!.collect_contact && p.email ? { email: p.email } : {}) }))
  if (!guests.length) return
  saving.value = true
  error.value = null
  try {
    const r = await store.addGuests({
      list_id: list.value.id, allocation_id: allocation.value?.id ?? null, source: mode.value === 'one' ? 'manual' : 'paste', guests,
    })
    const dups = r.duplicates.map(i => guests[i]!.name)
    const pending = r.added.filter(g => g.status === 'pending').length
    let msg = `Added ${r.added.length} to ${list.value.name}.`
    if (pending) msg += ` ${pending} waiting for approval.`
    if (dups.length) msg += ` Skipped ${dups.length} already on the list: ${dups.join(', ')}.`
    Object.assign(one, { name: '', plus: 0, note: '', email: '', phone: '' })
    paste.value = ''
    emit('done', msg)
  } catch (e) {
    error.value = e as ApiError
  } finally {
    saving.value = false
  }
}

const canSubmit = computed(() => !!list.value && !saving.value && (mode.value === 'one' ? !!one.name.trim() : parsed.value.length > 0))
</script>

<template>
  <form class="hud-card" style="padding:12px 14px;display:grid;gap:10px;" aria-labelledby="add-guests-h" novalidate @submit.prevent="submit">
    <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:6px;">
      <h3 id="add-guests-h" class="section-lbl" style="margin:0;">ADD GUESTS</h3>
      <div role="radiogroup" aria-label="How to add" style="display:flex;gap:4px;">
        <button type="button" role="radio" :aria-checked="mode === 'paste'" class="btn-hud btn-hud-sm" :class="mode === 'paste' ? 'btn-hud-cta' : 'btn-hud-ghost'" style="min-height:44px;" @click="mode = 'paste'">PASTE NAMES</button>
        <button type="button" role="radio" :aria-checked="mode === 'one'" class="btn-hud btn-hud-sm" :class="mode === 'one' ? 'btn-hud-cta' : 'btn-hud-ghost'" style="min-height:44px;" @click="mode = 'one'">ONE GUEST</button>
      </div>
    </div>

    <div class="form-grid">
      <div class="field">
        <label :for="`${uid}-list`" class="section-lbl">LIST</label>
        <select :id="`${uid}-list`" v-model="listId" class="hud-input" required>
          <option v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</option>
        </select>
      </div>
      <div class="field">
        <label :for="`${uid}-alloc`" class="section-lbl">ALLOCATION</label>
        <select :id="`${uid}-alloc`" v-model="allocationId" class="hud-input" :disabled="!openAllocations.length">
          <option value="">{{ openAllocations.length ? 'None — straight onto the list' : 'No open allocations' }}</option>
          <option v-for="a in openAllocations" :key="a.id" :value="a.id">{{ a.label }} · {{ a.used }}/{{ a.quota }}</option>
        </select>
      </div>
    </div>

    <template v-if="mode === 'paste'">
      <div class="field">
        <label :for="`${uid}-paste`" class="section-lbl">NAMES · ONE PER LINE</label>
        <textarea :id="`${uid}-paste`" v-model="paste" class="hud-textarea" rows="5" placeholder="Mara Weiss +1&#10;Tom&#10;Ines Duarte +2" />
      </div>
      <p v-if="parsed.length" role="status" class="data-frag" style="font-size:9px;margin:0;">
        {{ parsed.length }} {{ parsed.length === 1 ? 'GUEST' : 'GUESTS' }} · {{ heads }} HEADS
      </p>
      <p v-if="droppedEmails" style="margin:0;font-size:12px;color:var(--color-on-surface-variant);">
        {{ droppedEmails }} {{ droppedEmails === 1 ? 'email' : 'emails' }} will be left out: {{ list?.name }} is name-only.
      </p>
      <p v-if="overPlus.length" style="margin:0;font-size:12px;color:var(--color-error);">
        {{ overPlus.join(', ') }}: more than +{{ maxPlus }} allowed here.
      </p>
    </template>
    <div v-else class="form-grid">
      <label style="display:grid;gap:4px;">
        <span class="section-lbl">NAME</span>
        <input :id="`${uid}-name`" v-model="one.name" class="hud-input" maxlength="120" autocomplete="off">
      </label>
      <label style="display:grid;gap:4px;">
        <span class="section-lbl">+N</span>
        <input v-model.number="one.plus" class="hud-input" type="number" min="0" :max="maxPlus" inputmode="numeric">
      </label>
      <template v-if="list?.collect_contact">
        <label style="display:grid;gap:4px;"><span class="section-lbl">EMAIL</span><input v-model="one.email" class="hud-input" type="email" autocomplete="off"></label>
        <label style="display:grid;gap:4px;"><span class="section-lbl">PHONE</span><input v-model="one.phone" class="hud-input" type="tel" autocomplete="off"></label>
      </template>
      <label style="display:grid;gap:4px;grid-column:1/-1;">
        <span class="section-lbl">NOTE · ENCRYPTED</span>
        <input v-model="one.note" class="hud-input" maxlength="500" autocomplete="off">
      </label>
      <p v-if="list && !list.collect_contact" style="grid-column:1/-1;margin:0;font-size:12px;color:var(--color-on-surface-variant);">
        {{ list.name }} is name-only: no email or phone is stored.
      </p>
    </div>

    <label style="display:flex;align-items:center;gap:8px;min-height:44px;font-size:13px;">
      <input v-model="direct" type="checkbox">
      ADD DIRECTLY AS GOING<span v-if="allocation?.requires_approval" style="color:var(--color-on-surface-variant);">&nbsp;· skips approval</span>
    </label>

    <p v-if="errorText" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ errorText }}</p>
    <div style="display:flex;flex-wrap:wrap;gap:6px;">
      <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="!canSubmit">
        {{ saving ? 'ADDING…' : mode === 'paste' && parsed.length ? `ADD ${parsed.length}` : 'ADD' }}
      </button>
      <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="emit('cancel')">CLOSE</button>
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
.form-grid > label, .field { display: grid; gap: 4px; }
</style>
