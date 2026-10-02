<script setup lang="ts">
// Earnings entry editor — create income/expense (FIN-01, FIN-02) or edit an
// existing entry. The kind chip switches the required-string field between
// description (income) and notes (expense) exactly like the Go validator
// does. Currency code is uppercased in inputs/outputs; amount moves are
// integer minor units throughout the store layer. The dialog can also be
// pre-loaded with a gigId to attach an income to a known gig.
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import Dialog from '#kui/components/ui/dialog/Dialog.vue'
import DialogContent from '#kui/components/ui/dialog/DialogContent.vue'
import DialogTitle from '#kui/components/ui/dialog/DialogTitle.vue'
import DialogDescription from '#kui/components/ui/dialog/DialogDescription.vue'
import { useEarningsStore } from '../../stores/earnings'
import { useDialogFocus } from '../../utils/dialogFocus'
import { entryDateError } from '../../utils/invoiceFields'
import { formatMinor, minorToDecimalString, parseMoney } from '../../utils/money'
import type { Entry, EntryKind } from '../../types/finance'

const props = defineProps<{
  open: boolean
  kind: Entry['kind']
  gigId?: string | null
  editId?: string | null
}>()
const emit = defineEmits<{ 'update:open': [open: boolean]; saved: [entry: Entry] }>()

const store = useEarningsStore()
const { entries, currencies } = storeToRefs(store)

const titleEl = ref<InstanceType<typeof DialogTitle> | null>(null)
const { onOpenAutoFocus, onCloseAutoFocus } = useDialogFocus(() => props.open, titleEl)

const CATEGORY_OPTIONS: Record<Entry['kind'], string[]> = {
  income: ['gig_fee', 'royalties', 'one_off', 'tip', 'sponsorship', 'other_income'],
  expense: ['gear', 'travel', 'studio', 'marketing', 'admin', 'tax', 'other_expense'],
}
const CURRENCY_OPTIONS = ['EUR', 'USD', 'GBP', 'CHF', 'PLN', 'CZK', 'DKK', 'SEK', 'NOK', 'JPY']

const form = reactive({
  amount: '',
  currency: 'EUR',
  category: 'gig_fee',
  entry_date: '',
  description: '',
  notes: '',
  gig_id: null as string | null,
})

const editing = computed(() => !!props.editId)

// FE-1 hardening: in edit mode the dialog tracks the entry's own kind,
// not the last `props.kind` (which carries the create preset). The title,
// required-string branch, and PUT body all derive from this snapshot.
const localKind = ref<EntryKind>(props.kind)
const isIncome = computed(() => localKind.value === 'income')

const amountMinor = computed(() => parseMoney(form.amount, form.currency) ?? 0)

const requiredString = computed(() => (isIncome.value ? form.description.trim() : form.notes.trim()))
const amountError = computed(() => {
  if (form.amount.trim() === '') return 'Add the amount.'
  const minor = parseMoney(form.amount, form.currency)
  if (minor === null) return 'Use a positive number, e.g. 1500 or 1,500.00.'
  if (minor <= 0) return 'Amount must be greater than zero.'
  return ''
})
const currencyError = computed(() => {
  if (!/^[A-Z]{3}$/.test(form.currency)) return 'Use an uppercase 3-letter code, e.g. EUR.'
  return ''
})
const dateError = computed(() => entryDateError(form.entry_date))
const categoryError = computed(() => {
  // `form.category` is initialised to 'gig_fee' / 'gear' but we still
  // treat an undefined / empty / whitespace-only value as "not picked"
  // so the empty placeholder option in the <select> surfaces an error.
  const v = (form.category ?? '').toString().trim()
  return v === '' ? 'Pick a category.' : ''
})
const requiredError = computed(() => (requiredString.value === '' ? (isIncome.value ? 'Description is required for income.' : 'Notes are required for expenses.') : ''))
const gigIdError = computed(() => {
  if (form.gig_id && !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(form.gig_id)) {
    return 'Use a UUID or leave empty.'
  }
  return ''
})
const canSave = computed(
  () => !saving.value
    && !conflicted.value
    && editReady.value
    && !amountError.value
    && !currencyError.value
    && !dateError.value
    && !categoryError.value
    && !requiredError.value
    && !gigIdError.value
    && amountMinor.value > 0,
)

const previewLabel = computed(() => (amountMinor.value > 0 ? formatMinor(amountMinor.value, form.currency) : '—'))

function loadFromEntry(e: Entry): void {
  // Use the same currency-aware splitter as PaymentForm so 0-decimal
  // currencies like JPY round-trip exactly. A hardcoded /100 would
  // silently drop the last two digits for JPY (and scale up for 3-decimal
  // currencies), turning ¥1200 into "12" → amount_minor = 12 on save.
  form.amount = minorToDecimalString(e.amount_minor, e.currency)
  form.currency = e.currency
  form.category = e.category
  form.entry_date = e.entry_date
  form.description = e.description
  form.notes = e.notes
  form.gig_id = e.gig_id
  // FE-1: kind belongs to the entry, not to the last create preset.
  // FE-2: capture the original version token so we can send it on save
  // and recognise a same-id list refresh as a separate snapshot.
  localKind.value = e.kind
  originalUpdatedAt.value = e.updated_at
  // A successful reseed (open / reload) clears any prior conflict state.
  conflictRemote.value = null
}

const reloadBtn = ref<HTMLButtonElement | null>(null)
const saving = ref(false)
const error = ref('')

// FE-2: the entry's T1 token captured when the dialog opened or last
// reloaded. Always paired with the form fields, never with a later token
// borrowed from the Pinia cache. Empty in create mode.
const originalUpdatedAt = ref('')
// Latest authoritative entry the store surfaced via a 409 conflict GET.
// Non-null means the editor is conflicted: form keeps T1 fields but the
// user must explicitly DISCARD CHANGES AND RELOAD to adopt T2 fields + T2
// token together.
const conflictRemote = ref<Entry | null>(null)
const conflicted = computed(() => conflictRemote.value !== null)
// Edit mode needs the entry's own snapshot (kind + version token). When the
// row is not cached the dialog reloads it and refuses to save until that
// snapshot arrives; it never PUTs the create preset with an empty token.
const editReady = computed(() => !props.editId || originalUpdatedAt.value !== '')

let lifetime = 0
let alive = true
onBeforeUnmount(() => { alive = false; lifetime++ })
// `kind` is only the create preset: in edit mode the entry's own kind wins,
// so a preset change must not re-run the load and wipe unsaved edits.
watch([() => props.open, () => props.editId, () => (props.editId ? null : props.kind)], ([open, editId]) => {
  const kind = props.kind
  lifetime++
  saving.value = false
  error.value = ''
  conflictRemote.value = null
  if (!open) return
  if (editId) {
    const e = entries.value.find((x) => x.id === editId)
    if (e) {
      loadFromEntry(e)
      return
    }
    // Not cached (filter change / refetch): blank fields, no token, Save
    // stays disabled until the authoritative entry is reloaded.
    originalUpdatedAt.value = ''
    void reloadMissing(editId, lifetime)
  }
  form.amount = ''
  form.currency = 'EUR'
  form.category = kind === 'income' ? 'gig_fee' : 'gear'
  form.entry_date = new Date().toISOString().slice(0, 10)
  form.description = ''
  form.notes = ''
  form.gig_id = props.gigId ?? null
  // Create mode uses the preset kind and has no version token.
  localKind.value = kind
  originalUpdatedAt.value = ''
  conflictRemote.value = null
  saving.value = false
  error.value = ''
}, { immediate: true, flush: 'sync' })

async function reloadMissing(id: string, generation: number): Promise<void> {
  const isCurrent = () => alive && props.open && lifetime === generation && props.editId === id
  const { entry: remote, superseded } = await store.readEntry(id)
  if (!isCurrent()) return
  if (remote) loadFromEntry(remote)
  else if (superseded) {
    // A newer read/write took over; adopt whatever it cached, stay quiet.
    const cached = entries.value.find((x) => x.id === id)
    if (cached) loadFromEntry(cached)
  } else error.value = 'Could not load this entry. Close the dialog and try again.'
}

async function save(): Promise<void> {
  if (!props.open || !canSave.value) return
  const generation = lifetime
  const id = props.editId
  const isCurrent = () => alive && props.open && lifetime === generation
  saving.value = true
  error.value = ''
  try {
    const fields = {
      // FE-1: use the entry's own kind (captured on open / reload),
      // not the create preset.
      kind: localKind.value,
      amount_minor: amountMinor.value,
      currency: form.currency,
      category: form.category,
      entry_date: form.entry_date,
      description: form.description,
      notes: form.notes,
      gig_id: form.gig_id || null,
    }
    // Create has no version token and the API rejects unknown fields, so
    // only edit pairs the T1 fields with the T1 token captured on open.
    const e = id
      ? await store.updateEntry(id, { ...fields, updated_at: originalUpdatedAt.value })
      : await store.createEntry(fields)
    if (!isCurrent()) return
    emit('saved', e)
    emit('update:open', false)
  } catch (err: unknown) {
    if (!isCurrent()) return
    const ferr = err as { message?: string; code?: string }
    if (ferr?.code === 'conflict') {
      // FE-2: by the time the store throws, `await fetchEntry(id)` has
      // already run and upserted the authoritative T2 row into
      // entries.value. Surface it so canSave can disable Save, and so the
      // explicit DISCARD CHANGES AND RELOAD button can adopt it as a
      // single atomic snapshot (fields + version together).
      // Until then the form keeps the user's unsaved T1 draft verbatim.
      error.value = 'This entry changed elsewhere. Discard your changes and reload to continue.'
      conflictRemote.value = entries.value.find((x) => x.id === props.editId) ?? null
      // Move focus to the only way forward so keyboard/AT users land on it.
      await nextTick()
      if (isCurrent()) reloadBtn.value?.focus()
    } else if (ferr?.code === 'validation_failed') {
      error.value = ferr.message ?? 'Check the fields.'
    } else {
      error.value = ferr?.message ?? 'Could not save the entry.'
    }
  } finally {
    if (isCurrent()) saving.value = false
  }
}

/** FE-2: explicitly adopt T2 fields + T2 token together. Save remains
 * disabled while conflicted, so this is the only path back to a savable
 * editor when the remote snapshot has diverged. */
async function discardAndReload(): Promise<void> {
  if (!props.open || !props.editId || saving.value) return
  const generation = lifetime
  const id = props.editId
  const isCurrent = () => alive && props.open && lifetime === generation
  saving.value = true
  try {
    const { entry: remote, superseded } = await store.readEntry(id)
    if (!isCurrent()) return
    if (!remote) { if (!superseded) error.value = 'Could not reload the entry.'; return }
    loadFromEntry(remote)
    error.value = ''
  } finally {
    if (isCurrent()) saving.value = false
  }
}

function setOpen(open: boolean): void {
  if (!open) { lifetime++; saving.value = false }
  emit('update:open', open)
}
function close(): void { setOpen(false) }
</script>

<template>
  <Dialog :open="open" @update:open="setOpen">
    <DialogContent
      class="max-w-[480px] w-[calc(100%-32px)] bg-surface-container max-h-[90vh] overflow-y-auto"
      @open-auto-focus="onOpenAutoFocus"
      @close-auto-focus="onCloseAutoFocus"
    >
      <DialogTitle ref="titleEl" tabindex="-1" class="ee-title">
        {{ editing ? (isIncome ? 'EDIT INCOME' : 'EDIT EXPENSE') : (isIncome ? 'NEW INCOME' : 'NEW EXPENSE') }}
      </DialogTitle>
      <DialogDescription class="ee-desc">
        {{ isIncome ? 'Records money received.' : 'Records money paid out.' }}
      </DialogDescription>

      <p class="ee-fin10" role="note">
        FIN-10 — No tax or VAT is computed on this entry. Multi-currency
        totals are grouped by code and never converted (FIN-09).
      </p>

      <form class="ee-form" novalidate @submit.prevent="save">
        <div class="ee-amount-row">
          <div>
            <label for="ee-amount" class="input-label ee-label">AMOUNT *</label>
            <input
              id="ee-amount"
              v-model="form.amount"
              class="hud-input"
              inputmode="decimal"
              :aria-invalid="!!amountError || undefined"
              placeholder="1500"
            >
            <p class="ee-msg" :class="{ 'ee-error': !!amountError }">{{ amountError || `Stored as ${previewLabel} minor units.` }}</p>
          </div>
          <div>
            <label for="ee-currency" class="input-label ee-label">CURRENCY *</label>
            <input
              id="ee-currency"
              v-model="form.currency"
              class="hud-input"
              maxlength="3"
              :aria-invalid="!!currencyError || undefined"
              list="ee-currencies"
            >
            <datalist id="ee-currencies">
              <option v-for="c in CURRENCY_OPTIONS" :key="c" :value="c" />
              <option v-for="c in currencies" :key="`cur-${c}`" :value="c" />
            </datalist>
            <p class="ee-msg" :class="{ 'ee-error': !!currencyError }">{{ currencyError || '3-letter ISO 4217 code.' }}</p>
          </div>
        </div>

        <div class="ee-row-2">
          <div>
            <label for="ee-date" class="input-label ee-label">ENTRY DATE *</label>
            <input id="ee-date" v-model="form.entry_date" type="date" class="hud-input" :aria-invalid="!!dateError || undefined">
            <p class="ee-msg" :class="{ 'ee-error': !!dateError }">{{ dateError || 'Date the income or expense applies to.' }}</p>
          </div>
          <div>
            <label for="ee-category" class="input-label ee-label">CATEGORY *</label>
            <select id="ee-category" v-model="form.category" class="hud-input">
              <!--
                The empty placeholder lets the user (and our TDD) clear the
                selection; otherwise the select falls back to the first option
                and there is no way to land on `''` through real DOM events.
              -->
              <option value="" disabled>SELECT CATEGORY…</option>
              <option v-for="c in CATEGORY_OPTIONS[localKind]" :key="c" :value="c">{{ c }}</option>
            </select>
            <p class="ee-msg" :class="{ 'ee-error': !!categoryError }">{{ categoryError || 'Anything not listed goes to other_income / other_expense.' }}</p>
          </div>
        </div>

        <div v-if="isIncome">
          <label for="ee-description" class="input-label ee-label">DESCRIPTION *</label>
          <input
            id="ee-description"
            v-model="form.description"
            class="hud-input"
            placeholder="Sample Records — Royalties Q1"
            :aria-invalid="!!requiredError || undefined"
          >
          <p class="ee-msg" :class="{ 'ee-error': !!requiredError && !requiredString }">{{ requiredString ? 'Used in the recent activity list.' : 'Required for income entries.' }}</p>
        </div>
        <div v-else>
          <label for="ee-notes" class="input-label ee-label">NOTES *</label>
          <textarea
            id="ee-notes"
            v-model="form.notes"
            class="hud-textarea"
            rows="2"
            placeholder="What was this expense for?"
            :aria-invalid="!!requiredError || undefined"
          />
          <p class="ee-msg" :class="{ 'ee-error': !!requiredError && !requiredString }">{{ requiredString ? 'Shown in the activity list.' : 'Required for expense entries.' }}</p>
        </div>

        <div>
          <label for="ee-gig" class="input-label ee-label">LINKED GIG (OPTIONAL)</label>
          <input
            id="ee-gig"
            v-model="form.gig_id"
            class="hud-input"
            placeholder="Gig UUID"
            :aria-invalid="!!gigIdError || undefined"
          >
          <p class="ee-msg" :class="{ 'ee-error': !!gigIdError }">{{ gigIdError || 'Paste a gig id when this row came from one.' }}</p>
        </div>

        <p v-if="error" class="ee-msg ee-error" role="alert">{{ error }}</p>

        <div class="ee-actions">
          <button type="button" class="btn-hud btn-hud-ghost" @click="close">CANCEL</button>
          <!--
            FE-2 conflict escape hatch: only meaningful in edit mode while
            conflicted. Adopting T2 fields + T2 token together is the only
            way back to a savable editor after a 409, so the button is the
            canonical reload action rather than an "Ok" / dismiss.
          -->
          <button
            v-if="editing && conflicted"
            ref="reloadBtn"
            type="button"
            class="btn-hud btn-hud-ghost"
            data-testid="ee-discard-reload"
            @click="discardAndReload"
          >
            DISCARD CHANGES AND RELOAD
          </button>
          <button type="submit" class="btn-hud btn-hud-cta" :disabled="!canSave">
            {{ saving ? 'SAVING…' : (editing ? 'SAVE CHANGES' : (isIncome ? 'CREATE INCOME' : 'CREATE EXPENSE')) }}
          </button>
        </div>
      </form>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
.ee-title { font-size: 14px; outline: none; }
.ee-desc { margin-top: 6px; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.ee-fin10 { margin: 8px 0 0; padding: 8px 12px; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--color-tertiary); background: color-mix(in srgb, var(--color-tertiary) 8%, transparent); border-left: 3px solid var(--color-tertiary); }
.ee-form { display: flex; flex-direction: column; gap: 14px; margin-top: 16px; }
.ee-label { display: block; margin-bottom: 6px; }
.ee-amount-row { display: grid; grid-template-columns: 1fr 110px; gap: 10px; }
.ee-row-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.ee-msg { margin: 4px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.ee-msg:empty { display: none; }
.ee-error { color: var(--color-error); }
.ee-actions { display: flex; justify-content: flex-end; gap: 8px; }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .ee-amount-row, .ee-row-2 { grid-template-columns: 1fr; }
  .ee-actions { flex-direction: column-reverse; }
  .ee-actions .btn-hud { width: 100%; }
}
</style>
