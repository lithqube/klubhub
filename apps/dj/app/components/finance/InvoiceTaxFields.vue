<script setup lang="ts">
// VAT treatment, rate, legal note and withholding for a draft invoice.
// Treatment behaviour comes from utils/vatTreatment.ts, never from
// branching on treatment names here.
import { ref, watch, computed } from 'vue'
import type { InvoiceTaxFieldsValue as TaxFieldsValue, Party, VatTreatment } from '../../types/finance'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import {
  VAT_TREATMENT_ORDER,
  applyTreatmentChange,
  vatTreatmentMeta,
  WITHHOLDING_MAX_BPS,
} from '../../utils/vatTreatment'
import { bpsToPercent, percentToBps } from '../../utils/money'
import { fieldId, fieldMessageId } from '../../utils/invoiceFields'

const props = withDefaults(defineProps<{
  modelValue: TaxFieldsValue
  customer: Party
  disabled?: boolean
  errors?: Record<string, string>
}>(), { disabled: false, errors: () => ({}) })

const emit = defineEmits<{
  'update:modelValue': [value: TaxFieldsValue]
  /** Local parse state so the parent can block SAVE on bad input. */
  'update:valid': [valid: boolean]
}>()

const store = useInvoiceStore()

const meta = computed(() => vatTreatmentMeta(props.modelValue.vat_treatment))

// Percent inputs keep the typed text so "7," or "19." is not rewritten mid-edit.
const rateText = ref(bpsToPercent(props.modelValue.tax_rate_bps))
const withholdingText = ref(bpsToPercent(props.modelValue.withholding_rate_bps))

watch(() => props.modelValue.tax_rate_bps, (bps) => {
  if (percentToBps(rateText.value) !== bps) rateText.value = bpsToPercent(bps)
})
watch(() => props.modelValue.withholding_rate_bps, (bps) => {
  if (percentToBps(withholdingText.value) !== bps) withholdingText.value = bpsToPercent(bps)
})

const rateError = computed(() => {
  if (!meta.value.rateEditable) return ''
  const bps = percentToBps(rateText.value)
  if (bps === null) return 'Enter a rate like 19 or 7.5.'
  if (bps > 10000) return 'The rate cannot exceed 100%.'
  return ''
})

const withholdingError = computed(() => {
  const bps = percentToBps(withholdingText.value === '' ? '0' : withholdingText.value)
  if (bps === null) return 'Enter a percentage like 15.'
  if (bps > 10000) return 'Withholding cannot exceed 100%.'
  return ''
})

// Drafts may hold up to 100%; issuing needs ≤ 50% (docs/INVOICING.md §4.1).
const withholdingWarning = computed(() => {
  const bps = percentToBps(withholdingText.value)
  return bps !== null && bps > WITHHOLDING_MAX_BPS
    ? `Above ${bpsToPercent(WITHHOLDING_MAX_BPS)}% can be saved but not issued.`
    : ''
})

watch([rateError, withholdingError], ([a, b]) => emit('update:valid', !a && !b), { immediate: true })

// Build on the latest emitted value: several patches can land before the
// parent re-renders (e.g. SUGGEST right after typing a rate).
let latest: TaxFieldsValue = props.modelValue
watch(() => props.modelValue, (v) => { latest = v })

function patch(p: Partial<TaxFieldsValue>): void {
  latest = { ...latest, ...p }
  emit('update:modelValue', latest)
}

function onTreatment(e: Event): void {
  const next = (e.target as HTMLSelectElement).value as VatTreatment
  const changed = applyTreatmentChange(latest, next)
  rateText.value = bpsToPercent(changed.tax_rate_bps)
  patch(changed)
}

function onRate(e: Event): void {
  rateText.value = (e.target as HTMLInputElement).value
  const bps = percentToBps(rateText.value)
  if (bps !== null && bps <= 10000) patch({ tax_rate_bps: bps })
}

function onWithholding(e: Event): void {
  withholdingText.value = (e.target as HTMLInputElement).value
  const bps = percentToBps(withholdingText.value === '' ? '0' : withholdingText.value)
  if (bps !== null && bps <= 10000) patch({ withholding_rate_bps: bps })
}

// ── Suggestion ──
const suggesting = ref(false)
const suggestion = ref('')
const suggestError = ref('')

async function suggest(): Promise<void> {
  suggesting.value = true
  suggestError.value = ''
  suggestion.value = ''
  try {
    const s = await store.suggestTax(props.customer)
    rateText.value = bpsToPercent(s.tax_rate_bps)
    patch({ vat_treatment: s.vat_treatment, tax_rate_bps: s.tax_rate_bps, tax_note: s.tax_note })
    suggestion.value = `Applied ${vatTreatmentMeta(s.vat_treatment).label}: ${s.reason}`
  } catch (e) {
    suggestError.value = toFinanceError(e).message
  } finally {
    suggesting.value = false
  }
}

function err(path: string, local = ''): string {
  return local || props.errors[path] || ''
}
</script>

<template>
  <fieldset class="tax" :disabled="disabled">
    <legend class="section-lbl tax-legend">TAX</legend>

    <div class="tax-row">
      <div class="tax-grow">
        <label :for="fieldId('vat_treatment')" class="input-label tax-label">VAT TREATMENT</label>
        <select
          :id="fieldId('vat_treatment')"
          class="hud-input"
          :value="modelValue.vat_treatment"
          :aria-invalid="err('vat_treatment') ? 'true' : undefined"
          :aria-describedby="fieldMessageId('vat_treatment')"
          @change="onTreatment"
        >
          <option v-for="t in VAT_TREATMENT_ORDER" :key="t" :value="t">
            {{ vatTreatmentMeta(t).label }}
          </option>
        </select>
      </div>
      <button
        type="button"
        class="btn-hud btn-hud-ghost btn-hud-sm tax-suggest"
        :disabled="suggesting || disabled"
        @click="suggest"
      >
        {{ suggesting ? 'SUGGESTING…' : 'SUGGEST' }}
      </button>
    </div>
    <p :id="fieldMessageId('vat_treatment')" class="tax-msg" :class="{ 'tax-msg-error': !!err('vat_treatment') }">
      {{ err('vat_treatment') || meta.description }}
    </p>
    <p v-if="suggestion || suggestError" class="tax-msg" :class="{ 'tax-msg-error': !!suggestError }" role="status">
      {{ suggestError || suggestion }}
    </p>

    <div class="tax-grid">
      <div>
        <label :for="fieldId('tax_rate_bps')" class="input-label tax-label">RATE %</label>
        <input
          :id="fieldId('tax_rate_bps')"
          class="hud-input"
          inputmode="decimal"
          :value="meta.rateEditable ? rateText : '0'"
          :disabled="!meta.rateEditable"
          :aria-invalid="err('tax_rate_bps', rateError) ? 'true' : undefined"
          :aria-describedby="fieldMessageId('tax_rate_bps')"
          @input="onRate"
        >
        <p :id="fieldMessageId('tax_rate_bps')" class="tax-msg" :class="{ 'tax-msg-error': !!err('tax_rate_bps', rateError) }" aria-live="polite">
          {{ err('tax_rate_bps', rateError) || (meta.rateEditable ? '' : 'Fixed at 0% for this treatment.') }}
        </p>
      </div>
      <div>
        <label :for="fieldId('withholding_rate_bps')" class="input-label tax-label">WITHHOLDING %</label>
        <input
          :id="fieldId('withholding_rate_bps')"
          class="hud-input"
          inputmode="decimal"
          :value="withholdingText"
          :aria-invalid="err('withholding_rate_bps', withholdingError) ? 'true' : undefined"
          :aria-describedby="fieldMessageId('withholding_rate_bps')"
          @input="onWithholding"
        >
        <p :id="fieldMessageId('withholding_rate_bps')" class="tax-msg" :class="{ 'tax-msg-error': !!err('withholding_rate_bps', withholdingError) }" aria-live="polite">
          {{ err('withholding_rate_bps', withholdingError) || withholdingWarning || 'Tax the promoter withholds from your fee, e.g. artist tax' }}
        </p>
      </div>
    </div>

    <div>
      <label :for="fieldId('tax_note')" class="input-label tax-label">
        LEGAL NOTE<span v-if="meta.requiresNote" aria-hidden="true"> *</span>
      </label>
      <textarea
        :id="fieldId('tax_note')"
        class="hud-textarea"
        rows="2"
        style="min-height:56px;"
        :value="modelValue.tax_note"
        :required="meta.requiresNote || undefined"
        :aria-invalid="err('tax_note') ? 'true' : undefined"
        :aria-describedby="fieldMessageId('tax_note')"
        @input="patch({ tax_note: ($event.target as HTMLTextAreaElement).value })"
      />
      <p :id="fieldMessageId('tax_note')" class="tax-msg" :class="{ 'tax-msg-error': !!err('tax_note') }">
        {{ err('tax_note') || 'Printed on the invoice. Have an accountant confirm the wording for your country.' }}
      </p>
    </div>
  </fieldset>
</template>

<style scoped>
.tax { border: 0; padding: 0; margin: 0; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
.tax-legend { padding: 0; margin-bottom: 8px; }
.tax-row { display: flex; gap: 8px; align-items: flex-end; }
.tax-grow { flex: 1; min-width: 0; }
.tax-label { display: block; }
.tax-suggest { height: 40px; min-height: 40px; }
.tax-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 12px; }
.tax-msg { margin: 4px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.tax-msg:empty { display: none; }
.tax-msg-error { color: var(--color-error); }
.hud-input:disabled { opacity: 1; color: var(--color-tertiary); cursor: not-allowed; }
@media (max-width: 768px) {
  .tax-grid { grid-template-columns: 1fr; }
  .tax-suggest, .hud-input { min-height: 44px; height: 44px; }
}
</style>
