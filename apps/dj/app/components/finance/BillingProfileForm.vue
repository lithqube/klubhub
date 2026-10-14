<script setup lang="ts">
// The DJ's billing identity (the supplier on every invoice). Besides the
// address and VAT ID it carries what EN 16931 e-invoices need: the national
// tax number (Steuernummer) and the IBAN/BIC customers pay into. The server
// validates and normalises; this form shows its field errors on the inputs.
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import type { BillingProfile, EntityKind, TaxIdKind, UpdateBillingProfileRequest } from '../../types/finance'
import { useBillingProfileStore } from '../../stores/billingProfile'
import { toFinanceError } from '../../stores/invoice'
import { parseValidationMessage } from '../../utils/invoiceLines'
import { bpsToPercent, percentToBps } from '../../utils/money'

const store = useBillingProfileStore()
const { profile, loading, error: loadError } = storeToRefs(store)

type TextKey =
  | 'legal_name' | 'trading_name' | 'contact_email' | 'contact_phone' | 'address_line1' | 'address_line2'
  | 'address_city' | 'address_region' | 'address_postal' | 'address_country' | 'jurisdiction'
  | 'default_currency' | 'tax_id' | 'tax_number' | 'iban' | 'bic' | 'payment_instructions'

interface Form extends Record<TextKey, string> {
  entity_kind: EntityKind
  tax_id_kind: TaxIdKind
  vat_exempt_small_business: boolean
  /** Percent as typed ("19", "7.5"). */
  default_vat_rate: string
}

const ENTITY_KINDS: { value: EntityKind; label: string }[] = [
  { value: 'individual', label: 'Individual' },
  { value: 'sole_trader', label: 'Sole trader / freelancer' },
  { value: 'partnership', label: 'Partnership' },
  { value: 'llc', label: 'LLC / GmbH' },
  { value: 'corp', label: 'Corporation' },
  { value: 'other', label: 'Other' },
]
const TAX_ID_KINDS: { value: TaxIdKind; label: string }[] = [
  { value: '', label: 'None' },
  { value: 'vat', label: 'VAT ID' },
  { value: 'ein', label: 'EIN (US)' },
  { value: 'gst', label: 'GST' },
  { value: 'abn', label: 'ABN' },
  { value: 'other', label: 'Other' },
]

interface FieldDef {
  key: TextKey
  label: string
  required?: boolean
  type?: string
  hint?: string
  placeholder?: string
  maxlength?: number
  wide?: boolean
  /** Columns of 12 the field spans (layout only); defaults to 6. */
  span?: number
  /** Starts a new cluster: extra space above, so related fields read as a group. */
  group?: boolean
  multiline?: boolean
  autocomplete?: string
}

const FIELDS: FieldDef[] = [
  { key: 'legal_name', label: 'LEGAL NAME', required: true },
  { key: 'trading_name', label: 'TRADING NAME' },
  { key: 'contact_email', label: 'CONTACT EMAIL', required: true, type: 'email' },
  { key: 'contact_phone', label: 'CONTACT PHONE', type: 'tel' },
  { key: 'address_line1', label: 'ADDRESS', required: true, wide: true, span: 12, group: true },
  { key: 'address_line2', label: 'ADDRESS LINE 2', wide: true, span: 12 },
  { key: 'address_city', label: 'CITY', required: true, span: 5 },
  { key: 'address_postal', label: 'POSTAL CODE', required: true, span: 3 },
  { key: 'address_region', label: 'REGION / STATE', span: 4 },
  { key: 'address_country', label: 'COUNTRY (ISO-2)', maxlength: 2, placeholder: 'DE', hint: 'Two-letter code, e.g. DE.', span: 4, group: true },
  { key: 'jurisdiction', label: 'JURISDICTION', required: true, hint: 'Where you are taxed, e.g. DE.', span: 4, group: true },
  { key: 'default_currency', label: 'DEFAULT CURRENCY', required: true, maxlength: 3, placeholder: 'EUR', hint: 'Three-letter code.', span: 4, group: true },
  { key: 'tax_id', label: 'VAT ID / TAX ID', hint: 'Your VAT ID with country prefix (e.g. DE000000000).', group: true },
  {
    key: 'tax_number',
    label: 'TAX NUMBER (STEUERNUMMER)',
    maxlength: 40,
    hint: 'National tax number issued by your tax office. Separate from the VAT ID; e-invoices need one of the two.',
    group: true,
  },
  { key: 'iban', label: 'IBAN', placeholder: 'DE89 3704 0044 0532 0130 00', hint: 'Spaces are fine; we store it without them.', autocomplete: 'off', span: 8, group: true },
  { key: 'bic', label: 'BIC / SWIFT', maxlength: 11, placeholder: 'COBADEFFXXX', hint: '8 or 11 characters.', autocomplete: 'off', span: 4, group: true },
  { key: 'payment_instructions', label: 'PAYMENT INSTRUCTIONS', wide: true, span: 12, multiline: true, hint: 'Free text printed on invoices, e.g. bank details or a payment note.' },
]

function fromProfile(p: BillingProfile): Form {
  return {
    legal_name: p.legal_name, trading_name: p.trading_name, contact_email: p.contact_email, contact_phone: p.contact_phone,
    address_line1: p.address_line1, address_line2: p.address_line2, address_city: p.address_city,
    address_region: p.address_region, address_postal: p.address_postal, address_country: p.address_country,
    jurisdiction: p.jurisdiction, default_currency: p.default_currency, tax_id: p.tax_id,
    tax_number: p.tax_number ?? '', iban: p.iban ?? '', bic: p.bic ?? '', payment_instructions: p.payment_instructions,
    entity_kind: p.entity_kind, tax_id_kind: p.tax_id_kind,
    vat_exempt_small_business: p.vat_exempt_small_business,
    default_vat_rate: bpsToPercent(p.default_vat_rate_bps),
  }
}

const form = reactive<Form>(fromProfile({
  id: '', legal_name: '', trading_name: '', entity_kind: 'individual', tax_id: '', tax_id_kind: '', contact_email: '',
  contact_phone: '', address_line1: '', address_line2: '', address_city: '', address_region: '', address_postal: '',
  address_country: '', jurisdiction: '', payment_instructions: '', default_currency: 'EUR', tax_number: '', iban: '',
  bic: '', vat_exempt_small_business: false, default_vat_rate_bps: 0, updated_at: '', created_at: '',
}))
const baseline = ref(JSON.stringify(form))
const saving = ref(false)
const saveError = ref('')
const saved = ref(false)
const fieldErrors = ref<Record<string, string>>({})
const root = ref<HTMLElement | null>(null)

function seed(p: BillingProfile): void {
  Object.assign(form, fromProfile(p))
  baseline.value = JSON.stringify(form)
}

onMounted(() => {
  if (profile.value) seed(profile.value)
  else void store.fetchProfile()
})
// The first successful load seeds the form; later store updates come from our own save.
watch(profile, (p) => { if (p && !dirty.value) seed(p) })

const dirty = computed(() => JSON.stringify(form) !== baseline.value)
const rateBps = computed(() => percentToBps(form.default_vat_rate))
const localErrors = computed(() => {
  const e: Record<string, string> = {}
  if (rateBps.value === null || rateBps.value > 10000) e.default_vat_rate_bps = 'Enter a percentage from 0 to 100, e.g. 19.'
  if (form.tax_number.length > 40) e.tax_number = 'Use at most 40 characters.'
  return e
})
const errors = computed(() => ({ ...fieldErrors.value, ...localErrors.value }))
const canSave = computed(() => !!profile.value && dirty.value && !saving.value && Object.keys(localErrors.value).length === 0)

function idFor(field: string): string {
  return `bp-${field}`
}

function body(): UpdateBillingProfileRequest {
  const t = (s: string) => s.trim()
  return {
    legal_name: t(form.legal_name), trading_name: t(form.trading_name), entity_kind: form.entity_kind,
    tax_id: t(form.tax_id), tax_id_kind: form.tax_id_kind, contact_email: t(form.contact_email),
    contact_phone: t(form.contact_phone), address_line1: t(form.address_line1), address_line2: t(form.address_line2),
    address_city: t(form.address_city), address_region: t(form.address_region), address_postal: t(form.address_postal),
    address_country: t(form.address_country).toUpperCase(), jurisdiction: t(form.jurisdiction),
    payment_instructions: form.payment_instructions, default_currency: t(form.default_currency).toUpperCase(),
    // The server removes spaces and upper-cases IBAN/BIC; send what was typed.
    tax_number: t(form.tax_number), iban: t(form.iban), bic: t(form.bic),
    vat_exempt_small_business: form.vat_exempt_small_business,
    default_vat_rate_bps: rateBps.value ?? 0,
    updated_at: profile.value!.updated_at,
  }
}

async function save(): Promise<void> {
  if (!canSave.value) return
  saving.value = true
  saveError.value = ''
  saved.value = false
  fieldErrors.value = {}
  try {
    seed(await store.updateProfile(body()))
    saved.value = true
  } catch (e) {
    const err = toFinanceError(e)
    if (err.code === 'validation_failed') {
      const all = parseValidationMessage(err.message)
      fieldErrors.value = all
      const first = Object.keys(all)[0]
      saveError.value = first ? `Check ${first.replace(/_/g, ' ')}: ${all[first]}` : err.message
      if (first) {
        await nextTick()
        root.value?.querySelector<HTMLElement>(`#${idFor(first === 'default_vat_rate_bps' ? 'default_vat_rate' : first)}`)?.focus()
      }
    } else if (err.code === 'conflict') {
      saveError.value = 'The billing profile changed elsewhere. Reload it, then re-apply your edits.'
    } else {
      saveError.value = err.message || 'Could not save the billing profile.'
    }
  } finally {
    saving.value = false
  }
}

async function reload(): Promise<void> {
  const p = await store.fetchProfile()
  if (p) {
    seed(p)
    saveError.value = ''
    fieldErrors.value = {}
  }
}

function onInput(key: TextKey, e: Event): void {
  let v = (e.target as HTMLInputElement | HTMLTextAreaElement).value
  if (key === 'address_country') v = v.toUpperCase().slice(0, 2)
  if (key === 'default_currency') v = v.toUpperCase().slice(0, 3)
  form[key] = v
  saved.value = false
}
</script>

<template>
  <div ref="root" class="bp">
    <p v-if="loading && !profile" class="bp-state" aria-busy="true">Loading billing profile…</p>
    <div v-else-if="loadError && !profile" class="bp-state" role="alert">
      <p>{{ loadError.code === 'not_found' ? 'No billing profile exists on this server yet.' : loadError.message }}</p>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" @click="reload">RETRY</button>
    </div>

    <form v-else class="bp-form" novalidate @submit.prevent="save">
      <div class="bp-grid">
        <div v-for="f in FIELDS" :key="f.key" :class="[`bp-s${f.span ?? 6}`, { 'bp-group': f.group }]">
          <label :for="idFor(f.key)" class="input-label bp-label">{{ f.label }}<span v-if="f.required" aria-hidden="true"> *</span></label>
          <textarea
            v-if="f.multiline"
            :id="idFor(f.key)"
            class="hud-textarea"
            rows="3"
            :value="form[f.key]"
            :aria-invalid="errors[f.key] ? 'true' : undefined"
            :aria-describedby="`${idFor(f.key)}-msg`"
            @input="onInput(f.key, $event)"
          />
          <input
            v-else
            :id="idFor(f.key)"
            class="hud-input"
            :type="f.type ?? 'text'"
            :value="form[f.key]"
            :maxlength="f.maxlength"
            :placeholder="f.placeholder"
            :autocomplete="f.autocomplete ?? 'off'"
            :required="f.required || undefined"
            :aria-invalid="errors[f.key] ? 'true' : undefined"
            :aria-describedby="`${idFor(f.key)}-msg`"
            @input="onInput(f.key, $event)"
          >
          <p :id="`${idFor(f.key)}-msg`" class="bp-msg" :class="{ 'bp-msg-error': !!errors[f.key] }">{{ errors[f.key] || f.hint }}</p>
        </div>

        <div class="bp-s4 bp-group">
          <label :for="idFor('entity_kind')" class="input-label bp-label">ENTITY TYPE</label>
          <select :id="idFor('entity_kind')" v-model="form.entity_kind" class="hud-input" :aria-invalid="errors.entity_kind ? 'true' : undefined">
            <option v-for="k in ENTITY_KINDS" :key="k.value" :value="k.value">{{ k.label }}</option>
          </select>
        </div>
        <div class="bp-s4 bp-group">
          <label :for="idFor('tax_id_kind')" class="input-label bp-label">TAX ID TYPE</label>
          <select :id="idFor('tax_id_kind')" v-model="form.tax_id_kind" class="hud-input" :aria-invalid="errors.tax_id_kind ? 'true' : undefined">
            <option v-for="k in TAX_ID_KINDS" :key="k.value" :value="k.value">{{ k.label }}</option>
          </select>
        </div>
        <div class="bp-s4 bp-group">
          <label :for="idFor('default_vat_rate')" class="input-label bp-label">DEFAULT VAT RATE (%)</label>
          <input
            :id="idFor('default_vat_rate')"
            v-model="form.default_vat_rate"
            class="hud-input"
            inputmode="decimal"
            autocomplete="off"
            :aria-invalid="errors.default_vat_rate_bps ? 'true' : undefined"
            :aria-describedby="`${idFor('default_vat_rate')}-msg`"
          >
          <p :id="`${idFor('default_vat_rate')}-msg`" class="bp-msg" :class="{ 'bp-msg-error': !!errors.default_vat_rate_bps }">{{ errors.default_vat_rate_bps || 'Domestic rate, e.g. 19.' }}</p>
        </div>
        <label class="bp-check">
          <input v-model="form.vat_exempt_small_business" type="checkbox" class="hud-check">
          <span class="input-label" style="margin:0;">SMALL-BUSINESS VAT EXEMPTION (KLEINUNTERNEHMER)</span>
        </label>
      </div>

      <div class="bp-save">
        <p class="bp-status" :class="{ 'bp-msg-error': !!saveError }" :role="saveError ? 'alert' : undefined" :aria-live="saveError ? undefined : 'polite'">
          {{ saveError || (saved ? 'Billing profile saved.' : dirty ? 'Unsaved changes.' : '') }}
        </p>
        <button v-if="saveError && /changed elsewhere/.test(saveError)" type="button" class="btn-hud btn-hud-ghost" @click="reload">
          RELOAD
        </button>
        <button type="submit" class="btn-hud btn-hud-cta" :disabled="!canSave">{{ saving ? 'SAVING…' : 'SAVE PROFILE' }}</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.bp-state { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; margin: 0; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface); }
.bp-state p { margin: 0; }
.bp { padding: 14px; background: var(--color-surface-container-low); border: 1px dashed color-mix(in srgb, var(--color-on-surface) 14%, transparent); }
.bp-form { display: flex; flex-direction: column; gap: 14px; }
/* 12-column grid: tight (8px) inside a cluster, a 12px release (.bp-group) between clusters. */
.bp-grid { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 8px 12px; }
.bp-grid > * { grid-column: span 6; min-width: 0; }
.bp-s3 { grid-column: span 3 !important; }
.bp-s4 { grid-column: span 4 !important; }
.bp-s5 { grid-column: span 5 !important; }
.bp-s6 { grid-column: span 6 !important; }
.bp-s8 { grid-column: span 8 !important; }
.bp-s12 { grid-column: 1 / -1 !important; }
.bp-group { margin-top: 10px; }
.bp-label { display: block; }
.bp-msg { margin: 3px 0 0; font-family: var(--font-data); font-size: 11px; line-height: 1.4; color: var(--color-tertiary); }
.bp-msg:empty { display: none; }
.bp-msg-error { color: var(--color-error); }
.bp-check { display: inline-flex; align-items: center; gap: 8px; min-height: 32px; grid-column: 1 / -1; margin-top: 10px; cursor: pointer; }
.bp-save { display: flex; align-items: center; justify-content: flex-end; gap: 12px; padding-top: 12px; border-top: 1px dashed color-mix(in srgb, var(--color-primary) 12%, transparent); }
.bp-status { margin: 0 auto 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .bp { padding: 12px; }
  .bp-grid { grid-template-columns: 1fr; }
  .bp-grid > *, .bp-s3, .bp-s4, .bp-s5, .bp-s6, .bp-s8, .bp-s12 { grid-column: 1 / -1 !important; }
  .bp-group { margin-top: 6px; }
  .bp-check { min-height: 44px; }
  .hud-input { min-height: 44px; }
  .bp-save { flex-wrap: wrap; }
  .bp-save .btn-hud { flex: 1; }
}
</style>
