<script setup lang="ts">
// Earnings entry composer — create income/expense (FIN-01, FIN-02) or edit an
// existing entry, INLINE in the Entries section (no modal: receipt thumbnails
// and per-file upload status need room, and on a phone the camera app switches
// away and can lose a half-filled modal). The kind chip switches the
// required-string field between description (income) and notes (expense)
// exactly like the Go validator does. Currency code is uppercased in
// inputs/outputs; amount moves are integer minor units throughout the store
// layer. The form can be pre-loaded with a gigId to attach an income to a
// known gig. Receipts (photos, scans, PDFs) are picked here and uploaded
// through the earnings store once the entry exists.
//
// The host (EarningsWorkspace) mounts it while the store says a composer is
// open, at the top of the section for a new entry or in place of the row for
// an edit, and unmounts it on `close`.
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useEarningsStore } from '../../stores/earnings'
import { useReceiptQueue } from '../../composables/useReceiptQueue'
import { focusWhenReady } from '../../utils/dialogFocus'
import { entryDateError } from '../../utils/invoiceFields'
import { formatMinor, minorToDecimalString, parseMoney } from '../../utils/money'
import type { Entry, EntryKind } from '../../types/finance'
import EntryReceipts from './EntryReceipts.vue'

const props = defineProps<{
  kind: Entry['kind']
  gigId?: string | null
  editId?: string | null
  /** Bumped by the store each time a composer is (re)requested: pull focus back to the first field. */
  focusSeq?: number
}>()
const emit = defineEmits<{ close: []; saved: [entry: Entry] }>()

const store = useEarningsStore()
const { entries, currencies } = storeToRefs(store)

const root = ref<HTMLElement | null>(null)
const amountEl = ref<HTMLInputElement | null>(null)

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

const receipts = useReceiptQueue()

// A new entry that was saved but still has receipts to send (or retry) turns
// into an edit of that entry without leaving the form, so nothing is lost.
const savedId = ref<string | null>(null)
const currentId = computed(() => savedId.value ?? props.editId ?? null)
const editing = computed(() => !!currentId.value)

// FE-1 hardening: in edit mode the form tracks the entry's own kind,
// not the last `props.kind` (which carries the create preset). The title,
// required-string branch, and PUT body all derive from this snapshot.
const localKind = ref<EntryKind>(props.kind)
const isIncome = computed(() => localKind.value === 'income')
/** A voided entry keeps its receipts readable but not editable. */
const entryVoided = ref(false)

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
// A message is shown only once the field has been left (blur): a fresh form is
// neutral, not an accusation. The rules above stay the single source of truth
// (canSave reads them directly), and SAVE stays disabled until they all pass.
const touched = reactive<Record<string, boolean>>({})
function touch(key: string): void {
  touched[key] = true
}
const amountShown = computed(() => (touched.amount ? amountError.value : ''))
const currencyShown = computed(() => (touched.currency ? currencyError.value : ''))
const dateShown = computed(() => (touched.date ? dateError.value : ''))
const categoryShown = computed(() => (touched.category ? categoryError.value : ''))
const requiredShown = computed(() => (touched.required ? requiredError.value : ''))
const gigIdShown = computed(() => (touched.gig ? gigIdError.value : ''))
const hasBlockingError = computed(
  () => !!(amountError.value || currencyError.value || dateError.value || categoryError.value || requiredError.value || gigIdError.value),
)

const canSave = computed(
  () => !saving.value
    && !conflicted.value
    && editReady.value
    // Uploads in flight finish first, so SAVE never races a receipt.
    && !receipts.busy
    && !amountError.value
    && !currencyError.value
    && !dateError.value
    && !categoryError.value
    && !requiredError.value
    && !gigIdError.value
    && amountMinor.value > 0,
)

const previewLabel = computed(() => (amountMinor.value > 0 ? formatMinor(amountMinor.value, form.currency) : '—'))

const title = computed(() =>
  editing.value ? (isIncome.value ? 'EDIT INCOME' : 'EDIT EXPENSE') : (isIncome.value ? 'NEW INCOME' : 'NEW EXPENSE'),
)

let boundReceiptsTo: string | null = null
/** Points the receipts section at the entry and loads its stored files. */
function syncReceipts(e: Entry): void {
  entryVoided.value = e.status === 'voided'
  if (boundReceiptsTo === e.id) return
  boundReceiptsTo = e.id
  receipts.bind(e.id)
  if ((e.attachment_count ?? 0) > 0) void receipts.loadSaved(e.id)
}

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
  syncReceipts(e)
}

const reloadBtn = ref<HTMLButtonElement | null>(null)
const saving = ref(false)
const error = ref('')
/** Polite status line: "Entry saved. 1 of 3 receipts failed to upload." */
const notice = ref('')

// FE-2: the entry's T1 token captured when the form opened or last
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
// row is not cached the form reloads it and refuses to save until that
// snapshot arrives; it never PUTs the create preset with an empty token.
const editReady = computed(() => !currentId.value || originalUpdatedAt.value !== '')

let lifetime = 0
let alive = true
// Where focus goes back to when the composer closes: the control that opened
// it, or (an edit replaces its row, taking the EDIT button with it) the
// re-rendered EDIT button of that row.
const opener = typeof document !== 'undefined' && document.activeElement instanceof HTMLElement && document.activeElement !== document.body
  ? document.activeElement
  : null
const originEditId = props.editId ?? null
const originKind = props.kind

function restoreFocus(): void {
  if (typeof document === 'undefined') return
  const fallback = originEditId
    ? `[data-edit-for="${originEditId.replace(/["\\]/g, '')}"]`
    : `[data-new-entry="${originKind}"]`
  const target = opener?.isConnected ? opener : document.querySelector<HTMLElement>(fallback)
  target?.focus({ preventScroll: true })
}

onMounted(() => {
  focusWhenReady(amountEl)
  // An in-place edit can sit below the fold of a long list.
  if (typeof root.value?.scrollIntoView === 'function') root.value.scrollIntoView({ block: 'nearest' })
})
onBeforeUnmount(() => {
  const el = root.value
  // Only take focus back when it was in the form (or lost): if the user
  // already moved on to another control, leave it there.
  const active = typeof document === 'undefined' ? null : document.activeElement
  const hadFocus = !!el && (el.contains(active) || active === document.body)
  alive = false
  lifetime++
  receipts.dispose()
  if (hadFocus) void nextTick(restoreFocus)
})
watch(() => props.focusSeq, () => focusWhenReady(amountEl))

// `kind` is only the create preset: in edit mode the entry's own kind wins,
// so a preset change must not re-run the load and wipe unsaved edits.
watch([() => props.editId, () => (props.editId ? null : props.kind)], ([editId]) => {
  const kind = props.kind
  lifetime++
  saving.value = false
  error.value = ''
  notice.value = ''
  conflictRemote.value = null
  savedId.value = null
  entryVoided.value = false
  boundReceiptsTo = null
  receipts.reset()
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

async function reloadMissing(id: string, generation: number, retried = false): Promise<void> {
  const isCurrent = () => alive && lifetime === generation && props.editId === id
  const { entry: remote, superseded } = await store.readEntry(id)
  if (!isCurrent()) return
  if (remote) loadFromEntry(remote)
  else if (superseded) {
    // A newer read/write took over; adopt whatever it cached, stay quiet.
    const cached = entries.value.find((x) => x.id === id)
    if (cached) loadFromEntry(cached)
    // Nothing cached yet: the form does not watch `entries`, so read once
    // more (a fresh read generation) rather than stay blank and unsavable.
    else if (!retried) await reloadMissing(id, generation, true)
    else error.value = 'Could not load this entry. Close the form and try again.'
  } else error.value = 'Could not load this entry. Close the form and try again.'
}

function receiptsSummary(): string {
  const failed = receipts.failedCount
  const total = receipts.attempted
  return `Entry saved. ${failed} of ${total} receipt${total === 1 ? '' : 's'} failed to upload.`
}

async function save(): Promise<void> {
  if (!canSave.value) return
  const generation = lifetime
  const id = currentId.value
  const isCurrent = () => alive && lifetime === generation
  saving.value = true
  error.value = ''
  notice.value = ''
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
    if (!id) {
      if (receipts.queuedCount === 0) { emit('close'); return }
      // The entry exists: switch to it (fresh token), then send the files. A
      // failure keeps the form open on the saved entry with the failed files.
      savedId.value = e.id
      originalUpdatedAt.value = e.updated_at
      localKind.value = e.kind
      boundReceiptsTo = e.id
      const { failed } = await receipts.uploadQueued(e.id)
      if (!isCurrent()) return
      if (failed === 0) { emit('close'); return }
      notice.value = receiptsSummary()
      await nextTick()
      if (isCurrent()) focusFirstFailed()
      return
    }
    // Edit: a file that failed to upload stays visible rather than being
    // dropped by the close.
    originalUpdatedAt.value = e.updated_at
    if (receipts.pendingCount > 0) {
      notice.value = receiptsSummary()
      return
    }
    emit('close')
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
      conflictRemote.value = entries.value.find((x) => x.id === currentId.value) ?? null
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

function focusFirstFailed(): void {
  root.value?.querySelector<HTMLElement>('.rc-item-failed button')?.focus()
}

/** FE-2: explicitly adopt T2 fields + T2 token together. Save remains
 * disabled while conflicted, so this is the only path back to a savable
 * editor when the remote snapshot has diverged. */
async function discardAndReload(): Promise<void> {
  const id = currentId.value
  if (!id || saving.value) return
  const generation = lifetime
  const isCurrent = () => alive && lifetime === generation
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

function close(): void {
  lifetime++
  saving.value = false
  emit('close')
}

function onEscape(e: KeyboardEvent): void {
  // Escape inside an open <select> closes its list, not the form.
  if ((e.target as HTMLElement | null)?.tagName === 'SELECT') return
  e.stopPropagation()
  close()
}
</script>

<template>
  <section ref="root" class="ee" aria-labelledby="ee-title" @keydown.esc="onEscape">
    <header class="ee-head">
      <h3 id="ee-title" class="ee-title">{{ title }}</h3>
      <p class="ee-desc">{{ isIncome ? 'Records money received.' : 'Records money paid out.' }}</p>
    </header>

    <p class="ee-fin10" role="note">
      FIN-10 — No tax or VAT is computed on this entry. Multi-currency
      totals are grouped by code and never converted (FIN-09).
    </p>

    <form class="ee-form" novalidate @submit.prevent="save">
      <div class="ee-grid">
        <div class="ee-f ee-f-amount">
          <label for="ee-amount" class="input-label ee-label">AMOUNT *</label>
          <input
            id="ee-amount"
            ref="amountEl"
            v-model="form.amount"
            class="hud-input"
            inputmode="decimal"
            :aria-invalid="!!amountShown || undefined"
            placeholder="1500"
            @blur="touch('amount')"
          >
          <p class="ee-msg" :class="{ 'ee-error': !!amountShown }">{{ amountShown || (amountMinor > 0 ? `Stored as ${previewLabel} minor units.` : 'Positive number, e.g. 1500 or 1,500.00.') }}</p>
        </div>
        <div class="ee-f ee-f-currency">
          <label for="ee-currency" class="input-label ee-label">CURRENCY *</label>
          <input
            id="ee-currency"
            v-model="form.currency"
            class="hud-input"
            maxlength="3"
            :aria-invalid="!!currencyShown || undefined"
            list="ee-currencies"
            @blur="touch('currency')"
          >
          <datalist id="ee-currencies">
            <option v-for="c in CURRENCY_OPTIONS" :key="c" :value="c" />
            <option v-for="c in currencies" :key="`cur-${c}`" :value="c" />
          </datalist>
          <p class="ee-msg" :class="{ 'ee-error': !!currencyShown }">{{ currencyShown || '3-letter ISO 4217 code.' }}</p>
        </div>
        <div class="ee-f ee-f-date">
          <label for="ee-date" class="input-label ee-label">ENTRY DATE *</label>
          <input id="ee-date" v-model="form.entry_date" type="date" class="hud-input" :aria-invalid="!!dateShown || undefined" @blur="touch('date')" @change="touch('date')">
          <p class="ee-msg" :class="{ 'ee-error': !!dateShown }">{{ dateShown || 'Date the income or expense applies to.' }}</p>
        </div>
        <div class="ee-f ee-f-category">
          <label for="ee-category" class="input-label ee-label">CATEGORY *</label>
          <select id="ee-category" v-model="form.category" class="hud-input" :aria-invalid="!!categoryShown || undefined" @blur="touch('category')" @change="touch('category')">
            <!--
              The empty placeholder lets the user (and our TDD) clear the
              selection; otherwise the select falls back to the first option
              and there is no way to land on `''` through real DOM events.
            -->
            <option value="" disabled>SELECT CATEGORY…</option>
            <option v-for="c in CATEGORY_OPTIONS[localKind]" :key="c" :value="c">{{ c }}</option>
          </select>
          <p class="ee-msg" :class="{ 'ee-error': !!categoryShown }">{{ categoryShown || 'Anything not listed goes to other_income / other_expense.' }}</p>
        </div>

        <div v-if="isIncome" class="ee-f ee-f-wide">
          <label for="ee-description" class="input-label ee-label">DESCRIPTION *</label>
          <input
            id="ee-description"
            v-model="form.description"
            class="hud-input"
            placeholder="Sample Records — Royalties Q1"
            :aria-invalid="!!requiredShown || undefined"
            @blur="touch('required')"
          >
          <p class="ee-msg" :class="{ 'ee-error': !!requiredShown }">{{ requiredString ? 'Used in the recent activity list.' : 'Required for income entries.' }}</p>
        </div>
        <div v-else class="ee-f ee-f-wide">
          <label for="ee-notes" class="input-label ee-label">NOTES *</label>
          <textarea
            id="ee-notes"
            v-model="form.notes"
            class="hud-textarea"
            rows="2"
            placeholder="What was this expense for?"
            :aria-invalid="!!requiredShown || undefined"
            @blur="touch('required')"
          />
          <p class="ee-msg" :class="{ 'ee-error': !!requiredShown }">{{ requiredString ? 'Shown in the activity list.' : 'Required for expense entries.' }}</p>
        </div>

        <div class="ee-f ee-f-wide">
          <label for="ee-gig" class="input-label ee-label">LINKED GIG (OPTIONAL)</label>
          <input
            id="ee-gig"
            v-model="form.gig_id"
            class="hud-input"
            placeholder="Gig UUID"
            :aria-invalid="!!gigIdShown || undefined"
            @blur="touch('gig')"
          >
          <p class="ee-msg" :class="{ 'ee-error': !!gigIdShown }">{{ gigIdShown || 'Paste a gig id when this row came from one.' }}</p>
        </div>
      </div>

      <div class="ee-section">
        <EntryReceipts :q="receipts" :read-only="entryVoided" :disabled="saving" />
      </div>

      <p v-if="notice" class="ee-note" role="status" aria-live="polite">{{ notice }}</p>
      <p v-if="error" class="ee-msg ee-error" role="alert">{{ error }}</p>

      <!-- Why SAVE is off, in plain words and the muted token: not an error until a field has been left. -->
      <p v-if="hasBlockingError && editReady && !saving && !conflicted" class="ee-need">Fill in the fields marked * to save.</p>

      <div class="ee-actions">
        <button type="button" class="btn-hud btn-hud-ghost" @click="close">{{ savedId ? 'CLOSE' : 'CANCEL' }}</button>
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
          {{ saving ? (receipts.busy ? 'UPLOADING RECEIPTS…' : 'SAVING…') : (editing ? 'SAVE CHANGES' : (isIncome ? 'CREATE INCOME' : 'CREATE EXPENSE')) }}
        </button>
      </div>
    </form>
  </section>
</template>

<style scoped>
/* Flat plane + dashed line (DESIGN.md): a form, not floating chrome. No glass, no shadow. */
.ee { display: flex; flex-direction: column; min-width: 0; padding: 14px 16px 16px; background: var(--color-surface-container-low); border: 1px dashed color-mix(in srgb, var(--color-on-surface) 22%, transparent); }
.ee-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 12px; }
.ee-title { margin: 0; font-family: var(--font-command); font-size: 14px; font-weight: 700; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); outline: none; }
.ee-desc { margin: 0; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.ee-fin10 { margin: 8px 0 0; padding: 8px 12px; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--color-tertiary); background: color-mix(in srgb, var(--color-tertiary) 8%, transparent); border-left: 3px solid var(--color-tertiary); }
.ee-form { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
/* 12-column grid: short fields share a row. */
.ee-grid { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 8px 12px; }
.ee-f { min-width: 0; }
.ee-f-amount { grid-column: span 3; }
.ee-f-currency { grid-column: span 2; }
.ee-f-date { grid-column: span 3; }
.ee-f-category { grid-column: span 4; }
.ee-f-wide { grid-column: 1 / -1; }
.ee-label { display: block; margin-bottom: 4px; }
.ee-msg { margin: 3px 0 0; font-family: var(--font-data); font-size: 11px; line-height: 1.4; color: var(--color-tertiary); }
.ee-msg:empty { display: none; }
.ee-need { margin: 6px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-tertiary); text-align: right; }
.ee-error { color: var(--color-error); }
/* Section break: a dashed rule and a larger release than the 8px inside a cluster. */
.ee-section { margin-top: 10px; padding-top: 14px; border-top: 1px dashed color-mix(in srgb, var(--color-on-surface) 14%, transparent); }
.ee-note { margin: 0; padding: 8px 12px; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); background: color-mix(in srgb, var(--color-on-surface) 6%, transparent); border-left: 3px solid var(--color-on-surface-variant); }
.ee-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; padding-top: 14px; border-top: 1px dashed color-mix(in srgb, var(--color-on-surface) 14%, transparent); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 900px) {
  .ee-f-amount { grid-column: span 7; }
  .ee-f-currency { grid-column: span 5; }
  .ee-f-date, .ee-f-category { grid-column: span 6; }
}
@media (max-width: 768px) {
  .ee { padding: 12px; }
  .ee-grid { gap: 8px; }
  .ee-f-amount { grid-column: span 8; }
  .ee-f-currency { grid-column: span 4; }
  .ee-f-date, .ee-f-category { grid-column: 1 / -1; }
  .ee .hud-input { min-height: 44px; }
  .ee-actions { flex-direction: column-reverse; }
  .ee-actions .btn-hud { width: 100%; }
}
</style>
