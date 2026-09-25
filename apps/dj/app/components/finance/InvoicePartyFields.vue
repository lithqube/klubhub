<script setup lang="ts">
// Generic editor for a Party (docs/INVOICING.md §2). Used for the invoice
// customer ("Bill to") today and meant for the supplier billing profile form
// later, so it knows nothing about invoices: the parent passes the path
// prefix used for field ids and the messages keyed by full field path.
import { computed, watch } from 'vue'
import type { Party } from '../../types/finance'
import { fieldId, fieldMessageId } from '../../utils/invoiceFields'

const props = withDefaults(defineProps<{
  modelValue: Party
  /** Field path prefix, e.g. "customer" → ids for "customer.city". */
  pathPrefix: string
  legend: string
  disabled?: boolean
  /** Messages keyed by full field path ("customer.city"). */
  errors?: Record<string, string>
  /** Field keys shown as required. */
  required?: (keyof Party)[]
}>(), {
  disabled: false,
  errors: () => ({}),
  required: () => [],
})

const emit = defineEmits<{ 'update:modelValue': [value: Party] }>()

type TextKey = Exclude<keyof Party, 'contact_id' | 'is_business'>

interface FieldDef {
  key: TextKey
  label: string
  type?: string
  autocomplete?: string
  wide?: boolean
  maxlength?: number
  placeholder?: string
  hint?: string
}

const FIELDS: FieldDef[] = [
  { key: 'legal_name', label: 'LEGAL NAME', autocomplete: 'off', hint: 'Person or registered company name.' },
  { key: 'company', label: 'COMPANY / TRADING NAME', autocomplete: 'off' },
  { key: 'email', label: 'EMAIL', type: 'email', autocomplete: 'off', wide: true },
  { key: 'address_line1', label: 'ADDRESS', autocomplete: 'off', wide: true },
  { key: 'address_line2', label: 'ADDRESS LINE 2', autocomplete: 'off', wide: true },
  { key: 'city', label: 'CITY', autocomplete: 'off' },
  { key: 'postal_code', label: 'POSTAL CODE', autocomplete: 'off' },
  { key: 'region', label: 'REGION / STATE', autocomplete: 'off' },
  { key: 'country', label: 'COUNTRY (ISO-2)', autocomplete: 'off', maxlength: 2, placeholder: 'DE', hint: 'Two-letter code, e.g. DE, FR, US.' },
  { key: 'vat_id', label: 'VAT ID', autocomplete: 'off', placeholder: 'DE000000000', hint: 'EU VAT ID with country prefix.' },
  { key: 'tax_id', label: 'OTHER TAX ID', autocomplete: 'off', hint: 'Company tax number. Never a personal social security number.' },
]

const requiredSet = computed(() => new Set(props.required))

function path(key: keyof Party): string {
  return `${props.pathPrefix}.${key}`
}

function errorFor(key: keyof Party): string {
  return props.errors[path(key)] ?? ''
}

function describedBy(f: FieldDef): string | undefined {
  return errorFor(f.key) || f.hint ? fieldMessageId(path(f.key)) : undefined
}

// Several inputs can fire before the parent re-renders (browser autofill,
// fast typing across fields), so build each update on the latest emitted
// value, not on a props snapshot that may be one update behind.
let latest: Party = props.modelValue
watch(() => props.modelValue, (v) => { latest = v })

function update<K extends keyof Party>(key: K, value: Party[K]): void {
  latest = { ...latest, [key]: value }
  emit('update:modelValue', latest)
}

function onText(key: TextKey, e: Event): void {
  let v = (e.target as HTMLInputElement).value
  if (key === 'country') v = v.toUpperCase().slice(0, 2)
  update(key, v)
}
</script>

<template>
  <fieldset class="party" :disabled="disabled">
    <legend class="section-lbl party-legend">{{ legend }}</legend>

    <label class="party-business">
      <input
        :id="fieldId(path('is_business'))"
        type="checkbox"
        :checked="modelValue.is_business"
        @change="update('is_business', ($event.target as HTMLInputElement).checked)"
      >
      <span class="input-label" style="margin:0;">BUSINESS CUSTOMER</span>
    </label>

    <div class="party-grid">
      <div
        v-for="f in FIELDS"
        :key="f.key"
        class="party-field"
        :class="{ 'party-wide': f.wide }"
      >
        <label :for="fieldId(path(f.key))" class="input-label party-label">
          {{ f.label }}<span v-if="requiredSet.has(f.key)" aria-hidden="true"> *</span>
        </label>
        <input
          :id="fieldId(path(f.key))"
          class="hud-input"
          :class="{ 'party-invalid': !!errorFor(f.key) }"
          :type="f.type ?? 'text'"
          :value="modelValue[f.key]"
          :autocomplete="f.autocomplete"
          :maxlength="f.maxlength"
          :placeholder="f.placeholder"
          :required="requiredSet.has(f.key) || undefined"
          :aria-invalid="errorFor(f.key) ? 'true' : undefined"
          :aria-describedby="describedBy(f)"
          @input="onText(f.key, $event)"
        >
        <p
          v-if="errorFor(f.key) || f.hint"
          :id="fieldMessageId(path(f.key))"
          class="party-msg"
          :class="{ 'party-msg-error': !!errorFor(f.key) }"
        >
          {{ errorFor(f.key) || f.hint }}
        </p>
      </div>
    </div>
  </fieldset>
</template>

<style scoped>
.party { border: 0; padding: 0; margin: 0; min-width: 0; }
.party-legend { padding: 0; margin-bottom: 8px; }
.party-business { display: inline-flex; align-items: center; gap: 8px; min-height: 32px; margin-bottom: 8px; cursor: pointer; }
.party-business input { width: 16px; height: 16px; accent-color: var(--color-primary); }
.party-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 12px; }
.party-field { min-width: 0; }
.party-wide { grid-column: 1 / -1; }
.party-label { display: block; }
.party-invalid { border-left-color: var(--color-error); }
.party-msg { margin: 4px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.party-msg-error { color: var(--color-error); }
@media (max-width: 768px) {
  .party-grid { grid-template-columns: 1fr; }
  .party-business { min-height: 44px; }
  .hud-input { min-height: 44px; }
}
</style>
