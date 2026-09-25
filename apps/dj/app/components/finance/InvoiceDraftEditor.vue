<script setup lang="ts">
// Editable body of a draft invoice: Bill to, tax, dates, SAVE (PUT) and the
// issue checklist. Form input is local and only re-seeded when a different
// invoice is shown or a save succeeds, so a 409 refetch never wipes what the
// user typed; they review the banner and press SAVE again themselves.
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import type { Invoice, InvoiceTaxFieldsValue, InvoiceUpdateInput, Party } from '../../types/finance'
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
import InvoicePartyFields from './InvoicePartyFields.vue'
import InvoiceTaxFields from './InvoiceTaxFields.vue'
import InvoiceIssueChecklist from './InvoiceIssueChecklist.vue'

const props = defineProps<{ invoice: Invoice; issuing?: boolean }>()
const emit = defineEmits<{ issue: []; saved: [inv: Invoice] }>()

const store = useInvoiceStore()
const { issueCheck, issueCheckLoading } = storeToRefs(store)

interface DraftForm {
  customer: Party
  tax: InvoiceTaxFieldsValue
  supply_date: string
  due_at: string
  number_prefix: string
  internal_notes: string
}

function fromInvoice(inv: Invoice): DraftForm {
  return {
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
  }
}

const form = reactive<DraftForm>(fromInvoice(props.invoice))
const baseline = ref(JSON.stringify(fromInvoice(props.invoice)))
const taxValid = ref(true)
const saving = ref(false)
const saveError = ref('')
const serverField = ref<{ field: string; message: string } | null>(null)
const showMore = ref(false)

function reseed(inv: Invoice): void {
  const next = fromInvoice(inv)
  Object.assign(form, next)
  baseline.value = JSON.stringify(next)
  serverField.value = null
}

watch(() => props.invoice.id, () => {
  reseed(props.invoice)
  saveError.value = ''
})

const dirty = computed(() => JSON.stringify(form) !== baseline.value)

const localErrors = computed(() => {
  const e: Record<string, string> = {}
  const c = countryError(form.customer.country)
  if (c) e['customer.country'] = c
  const p = prefixError(form.number_prefix)
  if (p) e.number_prefix = p
  if (form.due_at && form.supply_date && form.due_at < form.supply_date) {
    e.due_at = 'The due date is before the supply date.'
  }
  return e
})

/** Local checks first, then the 400 field, then saved-draft issue problems. */
const fieldErrors = computed(() => {
  const merged: Record<string, string> = { ...problemsByField(issueCheck.value?.problems) }
  if (serverField.value) merged[serverField.value.field] = serverField.value.message
  return { ...merged, ...localErrors.value }
})

const canSave = computed(() => dirty.value && !saving.value && taxValid.value && Object.keys(localErrors.value).length === 0)

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
  }
}

async function save(): Promise<void> {
  if (!canSave.value) return
  saving.value = true
  saveError.value = ''
  serverField.value = null
  try {
    const inv = await store.updateInvoice(props.invoice.id, input())
    reseed(inv)
    emit('saved', inv)
  } catch (e) {
    const err = toFinanceError(e)
    if (err.code === 'validation_failed' && err.field) {
      const msg = err.message.replace(/^validation failed:\s*/, '')
      serverField.value = { field: err.field, message: msg }
      saveError.value = `Check ${err.field.replace(/[._]/g, ' ')}: ${msg}`
      await nextTick()
      focusField(err.field)
    } else {
      saveError.value = inlineErrorMessage(err) ?? ''
    }
  } finally {
    saving.value = false
  }
}

function onFocusProblem(field: string): void {
  if (field === 'number_prefix') showMore.value = true
  void nextTick(() => {
    if (!focusField(field)) saveError.value = 'That field is not editable here.'
  })
}

const CUSTOMER_REQUIRED: (keyof Party)[] = ['legal_name', 'address_line1', 'city', 'country']
</script>

<template>
  <form class="draft" novalidate @submit.prevent="save">
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
      <p class="draft-status" :class="{ 'draft-msg-error': !!saveError }" aria-live="polite">
        {{ saveError || (dirty ? 'Unsaved changes.' : 'All changes saved.') }}
      </p>
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
.draft { display: flex; flex-direction: column; gap: 18px; }
.draft-dates { border: 0; padding: 0; margin: 0; min-width: 0; }
.draft-legend { padding: 0; margin-bottom: 8px; }
.draft-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 12px; }
.draft-wide { grid-column: 1 / -1; }
.draft-label { display: block; }
.draft-msg { margin: 4px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.draft-msg:empty { display: none; }
.draft-msg-error { color: var(--color-error); }
.draft-more { display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 0; background: transparent; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.draft-more:hover, .draft-more:focus-visible { color: var(--color-primary); outline: none; }
.draft-save { display: flex; align-items: center; justify-content: flex-end; gap: 12px; padding-top: 12px; border-top: 1px dashed color-mix(in srgb, var(--color-primary) 12%, transparent); }
.draft-status { margin: 0 auto 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .draft-grid { grid-template-columns: 1fr; }
  .draft-more, .hud-input { min-height: 44px; }
  .draft-save { flex-wrap: wrap; }
  .draft-save .btn-hud { flex: 1; }
}
</style>
