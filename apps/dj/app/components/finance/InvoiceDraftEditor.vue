<script setup lang="ts">
// Editable body of a draft invoice: Bill to, tax, dates, SAVE (PUT) and the
// issue checklist. Form input is local and only re-seeded when a different
// invoice is shown or a save succeeds, so a 409 refetch never wipes what the
// user typed; they review the banner and press SAVE again themselves.
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import type { Invoice, InvoiceLine, InvoiceTaxFieldsValue, InvoiceUpdateInput, Party } from '../../types/finance'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import { dateOnly, toApiTime } from '../../utils/invoiceDisplay'
import {
  countryError,
  fieldId,
  fieldMessageId,
  focusField,
  inlineErrorMessage,
  prefixError,
  problemsByField,
} from '../../utils/invoiceFields'
import {
  draftsSignature,
  draftsToInput,
  lineProblems,
  linesToDrafts,
  parseValidationMessage,
  type LineDraft,
} from '../../utils/invoiceLines'
import InvoicePartyFields from './InvoicePartyFields.vue'
import InvoiceLinesEditor from './InvoiceLinesEditor.vue'
import InvoiceTaxFields from './InvoiceTaxFields.vue'
import InvoiceIssueChecklist from './InvoiceIssueChecklist.vue'

const props = defineProps<{ invoice: Invoice; issuing?: boolean }>()
const emit = defineEmits<{ issue: []; saved: [inv: Invoice] }>()

const store = useInvoiceStore()
const { issueCheck, issueCheckLoading, lines: savedLines } = storeToRefs(store)

interface DraftForm {
  lines: LineDraft[]
  customer: Party
  tax: InvoiceTaxFieldsValue
  supply_date: string
  due_at: string
  number_prefix: string
  internal_notes: string
  buyer_reference: string
  purchase_order_ref: string
  contract_ref: string
  payment_terms: string
}

function fromInvoice(inv: Invoice, lines: InvoiceLine[]): DraftForm {
  return {
    lines: linesToDrafts(lines, inv.currency),
    customer: structuredClone({ ...inv.customer }),
    tax: {
      vat_treatment: inv.vat_treatment,
      tax_rate_bps: inv.tax_rate_bps,
      tax_note: inv.tax_note,
      withholding_rate_bps: inv.withholding_rate_bps,
    },
    supply_date: dateOnly(inv.supply_date),
    due_at: dateOnly(inv.due_at),
    number_prefix: inv.number_prefix || 'INV',
    internal_notes: inv.internal_notes,
    buyer_reference: inv.buyer_reference ?? '',
    purchase_order_ref: inv.purchase_order_ref ?? '',
    contract_ref: inv.contract_ref ?? '',
    payment_terms: inv.payment_terms ?? '',
  }
}

/** Comparable form of a draft; row keys are client-only and left out. */
function signature(f: DraftForm): string {
  return JSON.stringify({ ...f, lines: draftsSignature(f.lines) })
}

const initial = fromInvoice(props.invoice, savedLines.value)
const form = reactive<DraftForm>(initial)
const baseline = ref(signature(initial))
const baselineLines = ref(draftsSignature(initial.lines))
// Set after a save attempt so every invalid line row shows its message.
const showLineProblems = ref(false)
// FE-2: the invoice's T1 token captured when the editor opened (or last
// reloaded). Always paired with the form fields, never borrowed from the
// Pinia cache after a same-id refresh.
const originalUpdatedAt = ref(props.invoice.updated_at)
// FE-2: non-null while conflicted. The form stays on T1 fields, Save is
// blocked, and the explicit DISCARD CHANGES AND RELOAD button adopts T2
// fields + T2 token together.
const conflictRemote = ref<Invoice | null>(null)
const conflicted = computed(() => conflictRemote.value !== null)
const reloadBtn = ref<HTMLButtonElement | null>(null)
const taxValid = ref(true)
const saving = ref(false)
const saveError = ref('')
const serverField = ref<{ field: string; message: string } | null>(null)
/** Every field named by the last 400 (several can fail at once). */
const serverFields = ref<Record<string, string>>({})
const showMore = ref(false)

function reseed(inv: Invoice): void {
  const next = fromInvoice(inv, savedLines.value)
  Object.assign(form, next)
  baseline.value = signature(next)
  baselineLines.value = draftsSignature(next.lines)
  showLineProblems.value = false
  serverField.value = null
  serverFields.value = {}
  // FE-2: a successful reseed adopts both the new fields AND the new
  // version token, clearing any prior conflict state.
  originalUpdatedAt.value = inv.updated_at
  conflictRemote.value = null
}

let lifetime = 0
let alive = true
onBeforeUnmount(() => { alive = false; lifetime++ })
watch(() => props.invoice.id, () => {
  lifetime++
  saving.value = false
  reseed(props.invoice)
  saveError.value = ''
}, { flush: 'sync' })

const dirty = computed(() => signature(form) !== baseline.value)
/** Lines are sent (as the full list) only when they were edited. */
const linesEdited = computed(() => draftsSignature(form.lines) !== baselineLines.value)
const lineLocalProblems = computed(() => (linesEdited.value ? lineProblems(form.lines, props.invoice.currency) : {}))

const localErrors = computed(() => {
  const e: Record<string, string> = {}
  const c = countryError(form.customer.country)
  if (c) e['customer.country'] = c
  const p = prefixError(form.number_prefix)
  if (p) e.number_prefix = p
  if (form.due_at && form.supply_date && form.due_at < form.supply_date) {
    e.due_at = 'The due date is before the supply date.'
  }
  for (const [k, max] of [['buyer_reference', 100], ['purchase_order_ref', 100], ['contract_ref', 100], ['payment_terms', 500]] as const) {
    if ([...form[k]].length > max) e[k] = `Use at most ${max} characters.`
  }
  return e
})

/** Local checks first, then the 400 field, then saved-draft issue problems. */
const fieldErrors = computed(() => {
  const merged: Record<string, string> = { ...problemsByField(issueCheck.value?.problems) }
  if (serverField.value) merged[serverField.value.field] = serverField.value.message
  return { ...merged, ...serverFields.value, ...localErrors.value }
})

const canSave = computed(() =>
  !conflicted.value && dirty.value && !saving.value && taxValid.value
  && Object.keys(localErrors.value).length === 0
  && Object.keys(lineLocalProblems.value).length === 0)

function input(): InvoiceUpdateInput {
  return {
    customer: { ...form.customer, country: form.customer.country.trim().toUpperCase() },
    vat_treatment: form.tax.vat_treatment,
    tax_rate_bps: form.tax.tax_rate_bps,
    tax_note: form.tax.tax_note,
    withholding_rate_bps: form.tax.withholding_rate_bps,
    // supply_date is a plain YYYY-MM-DD; due_at is a timestamp (§4.1).
    supply_date: form.supply_date || null,
    due_at: toApiTime(form.due_at),
    number_prefix: form.number_prefix.trim().toUpperCase(),
    internal_notes: form.internal_notes,
    // The API replaces all four on every PUT, so omitting one would clear it.
    buyer_reference: form.buyer_reference.trim(),
    purchase_order_ref: form.purchase_order_ref.trim(),
    contract_ref: form.contract_ref.trim(),
    payment_terms: form.payment_terms.trim(),
    ...(linesEdited.value ? { lines: draftsToInput(form.lines, props.invoice.currency) } : {}),
    // FE-2: pair T1 fields with the T1 token captured on open / reload.
    updated_at: originalUpdatedAt.value,
  }
}

async function save(): Promise<void> {
  if (!canSave.value) {
    // Surface why a pressed-while-disabled form (Enter key) did nothing.
    if (Object.keys(lineLocalProblems.value).length) showLineProblems.value = true
    return
  }
  const generation = lifetime
  const id = props.invoice.id
  const isCurrent = () => alive && generation === lifetime
  saving.value = true
  saveError.value = ''
  serverField.value = null
  serverFields.value = {}
  try {
    const inv = await store.updateInvoice(id, input(), isCurrent)
    if (!isCurrent()) return
    reseed(inv)
    emit('saved', inv)
  } catch (e) {
    if (!isCurrent()) return
    const err = toFinanceError(e)
    if (err.code === 'conflict') {
      // FE-2: the store has already kicked off the authoritative GET
      // for this id; once it lands, `store.current` holds T2. The form
      // keeps the user's unsaved T1 draft, Save is blocked, and the
      // explicit DISCARD CHANGES AND RELOAD button adopts both fields
      // and the new token together.
      saveError.value = 'This invoice changed elsewhere. Discard your changes and reload to continue.'
      conflictRemote.value = store.current && store.current.id === props.invoice.id ? store.current : null
      // Move focus to the only way forward so keyboard/AT users land on it.
      await nextTick()
      if (isCurrent()) reloadBtn.value?.focus()
    } else if (err.code === 'validation_failed' && err.field) {
      const all = parseValidationMessage(err.message)
      const msg = all[err.field] ?? err.message.replace(/^validation failed:\s*/, '')
      serverField.value = { field: err.field, message: msg }
      serverFields.value = all
      const others = Object.keys(all).length - 1
      saveError.value = `Check ${err.field.replace(/[._]/g, ' ').replace(/\[(\d+)\]/g, (_m, n: string) => ` ${Number(n) + 1}`)}: ${msg}${others > 0 ? ` (and ${others} more)` : ''}`
      await nextTick()
      if (isCurrent()) focusField(err.field)
    } else {
      saveError.value = inlineErrorMessage(err) ?? ''
    }
  } finally {
    if (isCurrent()) saving.value = false
  }
}

/** FE-2: explicitly adopt T2 fields + T2 token together. Save remains
 * disabled while conflicted, so this is the only path back to a savable
 * editor when the remote snapshot has diverged. */
async function discardAndReload(): Promise<void> {
  if (saving.value) return
  const generation = lifetime
  const id = props.invoice.id
  const isCurrent = () => alive && lifetime === generation
  saving.value = true
  try {
    const remote = await store.fetchInvoice(id)
    if (!isCurrent()) return
    if (!remote) {
      // fetchInvoice records currentError only for a failed (not superseded) read.
      if (store.currentError) saveError.value = 'Could not reload the invoice.'
      return
    }
    reseed(remote)
    saveError.value = ''
  } finally {
    if (isCurrent()) saving.value = false
  }
}

function onFocusProblem(field: string): void {
  if (field === 'number_prefix' || field === 'internal_notes') showMore.value = true
  void nextTick(() => {
    if (!focusField(field)) saveError.value = 'That field is not editable here.'
  })
}

type ReferenceKey = 'buyer_reference' | 'purchase_order_ref' | 'contract_ref' | 'payment_terms'
const REFERENCE_FIELDS: { key: ReferenceKey; label: string; hint: string; wide?: boolean }[] = [
  {
    key: 'buyer_reference',
    label: 'BUYER REFERENCE (LEITWEG-ID)',
    hint: 'Required on e-invoices to German public-sector clients (the Leitweg-ID); optional otherwise.',
  },
  { key: 'purchase_order_ref', label: 'PURCHASE ORDER', hint: 'The client\'s order number, if they gave one.' },
  { key: 'contract_ref', label: 'CONTRACT REFERENCE', hint: 'Booking or contract number, if any.' },
  {
    key: 'payment_terms',
    label: 'PAYMENT TERMS',
    hint: 'Printed on the invoice, e.g. "Payable within 14 days".',
    wide: true,
  },
]

const CUSTOMER_REQUIRED: (keyof Party)[] = ['legal_name', 'address_line1', 'city', 'country']
</script>

<template>
  <form class="draft" novalidate @submit.prevent="save">
    <InvoiceLinesEditor
      v-model="form.lines"
      :currency="invoice.currency"
      :errors="fieldErrors"
      :show-all-problems="showLineProblems"
    />

    <InvoicePartyFields
      v-model="form.customer"
      path-prefix="customer"
      legend="BILL TO"
      :errors="fieldErrors"
      :required="CUSTOMER_REQUIRED"
    />

    <InvoiceTaxFields
      v-model="form.tax"
      :customer="form.customer"
      :errors="fieldErrors"
      @update:valid="taxValid = $event"
    />

    <fieldset class="draft-dates">
      <legend class="section-lbl draft-legend">DATES</legend>
      <div class="draft-grid">
        <div>
          <label :for="fieldId('supply_date')" class="input-label draft-label">SUPPLY DATE *</label>
          <input
            :id="fieldId('supply_date')"
            v-model="form.supply_date"
            type="date"
            class="hud-input"
            :aria-invalid="fieldErrors.supply_date ? 'true' : undefined"
            :aria-describedby="fieldMessageId('supply_date')"
          >
          <p :id="fieldMessageId('supply_date')" class="draft-msg" :class="{ 'draft-msg-error': !!fieldErrors.supply_date }">
            {{ fieldErrors.supply_date || 'The day you played. Defaults to the gig date.' }}
          </p>
        </div>
        <div>
          <label :for="fieldId('due_at')" class="input-label draft-label">DUE DATE</label>
          <input
            :id="fieldId('due_at')"
            v-model="form.due_at"
            type="date"
            class="hud-input"
            :aria-invalid="fieldErrors.due_at ? 'true' : undefined"
            :aria-describedby="fieldMessageId('due_at')"
          >
          <p :id="fieldMessageId('due_at')" class="draft-msg" :class="{ 'draft-msg-error': !!fieldErrors.due_at }">
            {{ fieldErrors.due_at || '' }}
          </p>
        </div>
      </div>
    </fieldset>

    <fieldset class="draft-dates">
      <legend class="section-lbl draft-legend">REFERENCES AND TERMS</legend>
      <div class="draft-grid">
        <div v-for="f in REFERENCE_FIELDS" :key="f.key" :class="{ 'draft-wide': f.wide }">
          <label :for="fieldId(f.key)" class="input-label draft-label">{{ f.label }}</label>
          <input
            :id="fieldId(f.key)"
            v-model="form[f.key]"
            class="hud-input"
            autocomplete="off"
            :aria-invalid="fieldErrors[f.key] ? 'true' : undefined"
            :aria-describedby="fieldMessageId(f.key)"
          >
          <p :id="fieldMessageId(f.key)" class="draft-msg" :class="{ 'draft-msg-error': !!fieldErrors[f.key] }">
            {{ fieldErrors[f.key] || f.hint }}
          </p>
        </div>
      </div>
    </fieldset>

    <div>
      <button
        type="button"
        class="draft-more"
        :aria-expanded="showMore"
        aria-controls="inv-draft-more"
        @click="showMore = !showMore"
      >
        <span aria-hidden="true">{{ showMore ? '−' : '+' }}</span> ADVANCED
      </button>
      <div v-show="showMore" id="inv-draft-more" class="draft-grid" style="margin-top:8px;">
        <div>
          <label :for="fieldId('number_prefix')" class="input-label draft-label">NUMBER PREFIX</label>
          <input
            :id="fieldId('number_prefix')"
            v-model="form.number_prefix"
            class="hud-input"
            maxlength="20"
            autocomplete="off"
            :aria-invalid="fieldErrors.number_prefix ? 'true' : undefined"
            :aria-describedby="fieldMessageId('number_prefix')"
          >
          <p :id="fieldMessageId('number_prefix')" class="draft-msg" :class="{ 'draft-msg-error': !!fieldErrors.number_prefix }">
            {{ fieldErrors.number_prefix || `Numbered at issue, e.g. ${(form.number_prefix || 'INV').toUpperCase()}-0001-${invoice.currency}.` }}
          </p>
        </div>
        <div class="draft-wide">
          <label :for="fieldId('internal_notes')" class="input-label draft-label">INTERNAL NOTES</label>
          <textarea
            :id="fieldId('internal_notes')"
            v-model="form.internal_notes"
            class="hud-textarea"
            rows="2"
            style="min-height:56px;"
          />
          <p class="draft-msg">Only for you; not printed on the invoice.</p>
        </div>
      </div>
    </div>

    <div class="draft-save">
      <p
        class="draft-status"
        :class="{ 'draft-msg-error': !!saveError }"
        :role="conflicted ? 'alert' : undefined"
        :aria-live="conflicted ? undefined : 'polite'"
      >
        {{ saveError || (Object.keys(lineLocalProblems).length ? 'Fix the line items to save.' : dirty ? 'Unsaved changes.' : 'All changes saved.') }}
      </p>
      <!--
        FE-2 conflict escape hatch. Save stays disabled while conflicted,
        so this is the only path back to a savable editor after the
        store's authoritative GET surfaces T2 fields + T2 token.
      -->
      <button
        v-if="conflicted"
        ref="reloadBtn"
        type="button"
        class="btn-hud btn-hud-ghost"
        data-testid="inv-discard-reload"
        @click="discardAndReload"
      >
        DISCARD CHANGES AND RELOAD
      </button>
      <button type="submit" class="btn-hud btn-hud-cta" :disabled="!canSave">
        {{ saving ? 'SAVING…' : 'SAVE DRAFT' }}
      </button>
    </div>

    <InvoiceIssueChecklist
      :check="issueCheck"
      :loading="issueCheckLoading"
      :dirty="dirty"
      :busy="issuing"
      @focus="onFocusProblem"
      @issue="emit('issue')"
    />
  </form>
</template>

<style scoped>
/* Rhythm: 8px between fields, a dashed rule + 20px release between sections. */
.draft { display: flex; flex-direction: column; gap: 20px; }
.draft-dates { border: 0; padding: 0; margin: 0; min-width: 0; }
.draft > fieldset:not(:first-child) { padding-top: 14px; border-top: 1px dashed color-mix(in srgb, var(--color-on-surface) 14%, transparent); }
.draft-legend { padding: 0; margin-bottom: 8px; color: var(--color-on-surface); font-weight: 700; letter-spacing: .08em; }
.draft-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px 12px; }
.draft-wide { grid-column: 1 / -1; }
.draft-label { display: block; }
.draft-msg { margin: 3px 0 0; font-family: var(--font-data); font-size: 11px; line-height: 1.4; color: var(--color-tertiary); }
.draft-msg:empty { display: none; }
.draft-msg-error { color: var(--color-error); }
.draft-more { display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 0; background: transparent; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 700; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.draft-more:hover, .draft-more:focus-visible { color: var(--color-primary); outline: none; }
/* Pinned to the foot of the sheet's scroll area so SAVE is always in reach on a long form. */
.draft-save { position: sticky; bottom: 0; z-index: 2; display: flex; align-items: center; justify-content: flex-end; gap: 12px; margin: 0 -24px; padding: 12px 24px; background: color-mix(in srgb, var(--color-surface-container) 94%, transparent); backdrop-filter: blur(12px); border-top: 1px dashed color-mix(in srgb, var(--color-primary) 18%, transparent); }
.draft-status { margin: 0 auto 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .draft-grid { grid-template-columns: 1fr; }
  .draft-more, .hud-input { min-height: 44px; }
  .draft-save { flex-wrap: wrap; }
  .draft-save .btn-hud { flex: 1; }
}
</style>
