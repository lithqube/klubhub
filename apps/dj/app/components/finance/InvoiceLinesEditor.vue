<script setup lang="ts">
// Line items of a draft invoice: description, whole-number quantity, unit and
// unit price (typed in major units, converted to minor units by string math).
// The per-line total and the subtotal here are previews; the server derives
// the real totals and the tax rate when the draft is saved.
import { computed, nextTick, ref } from 'vue'
import { UNIT_CODES, type UnitCode } from '../../types/finance'
import { fieldId, fieldMessageId } from '../../utils/invoiceFields'
import {
  emptyLine,
  lineProblems,
  lineTotalMinor,
  MAX_LINES,
  MAX_LINE_DESCRIPTION,
  MAX_LINE_QUANTITY,
  previewSubtotalMinor,
  type LineDraft,
  type LineProblems,
} from '../../utils/invoiceLines'
import { currencyDigits, formatMinor } from '../../utils/money'

const props = withDefaults(defineProps<{
  modelValue: LineDraft[]
  currency: string
  disabled?: boolean
  /** Server messages keyed by full path ("lines[0].quantity", "lines"). */
  errors?: Record<string, string>
  /** Show local problems on every row (after a save attempt), not only touched ones. */
  showAllProblems?: boolean
}>(), { disabled: false, errors: () => ({}), showAllProblems: false })

const emit = defineEmits<{ 'update:modelValue': [value: LineDraft[]] }>()

const root = ref<HTMLElement | null>(null)
const announcement = ref('')
const touched = ref(new Set<string>())

const problems = computed(() => lineProblems(props.modelValue, props.currency))
const fmt = (minor: number) => formatMinor(minor, props.currency)
const subtotal = computed(() => previewSubtotalMinor(props.modelValue, props.currency))
const onlyOne = computed(() => props.modelValue.length <= 1)
const atMax = computed(() => props.modelValue.length >= MAX_LINES)
const pricePlaceholder = computed(() => (currencyDigits(props.currency) === 0 ? '80' : '80.00'))

type Col = keyof LineProblems
const SERVER_PATH: Record<Col, string> = { description: 'description', quantity: 'quantity', unit: 'unit_minor' }

function path(i: number, col: string): string {
  return `lines[${i}].${col}`
}

/** Server message wins (it is the latest word); otherwise a local one once touched. */
function problemFor(i: number, col: Col): string {
  const server = props.errors[path(i, SERVER_PATH[col])]
  if (server) return server
  const local = problems.value[i]?.[col]
  if (!local) return ''
  return props.showAllProblems || touched.value.has(`${props.modelValue[i]?.key}.${col}`) ? local : ''
}

function touch(i: number, col: Col): void {
  touched.value = new Set(touched.value).add(`${props.modelValue[i]?.key}.${col}`)
}

function describedBy(i: number, col: Col, hint: boolean): string | undefined {
  return problemFor(i, col) || hint ? fieldMessageId(path(i, col === 'unit' ? 'unit_minor' : col)) : undefined
}

function patch(i: number, change: Partial<LineDraft>): void {
  emit('update:modelValue', props.modelValue.map((l, idx) => (idx === i ? { ...l, ...change } : l)))
}

function onInput(i: number, key: 'description' | 'quantity' | 'unit', e: Event): void {
  patch(i, { [key]: (e.target as HTMLInputElement).value } as Partial<LineDraft>)
}

function onUnitCode(i: number, e: Event): void {
  patch(i, { unit_code: (e.target as HTMLSelectElement).value as UnitCode })
}

async function add(): Promise<void> {
  if (atMax.value || props.disabled) return
  const next = [...props.modelValue, emptyLine()]
  emit('update:modelValue', next)
  announcement.value = `Line ${next.length} added.`
  await nextTick()
  root.value?.querySelector<HTMLElement>(`#${fieldId(path(next.length - 1, 'description'))}`)?.focus()
}

async function remove(i: number): Promise<void> {
  if (onlyOne.value || props.disabled) return
  const next = props.modelValue.filter((_, idx) => idx !== i)
  emit('update:modelValue', next)
  announcement.value = `Line ${i + 1} removed. ${next.length} ${next.length === 1 ? 'line' : 'lines'} left.`
  // Keep keyboard users in the list: land on the row that took this one's place.
  await nextTick()
  const target = Math.min(i, next.length - 1)
  root.value?.querySelector<HTMLElement>(`#${fieldId(path(target, 'description'))}`)?.focus()
}

function totalText(l: LineDraft): string {
  const t = lineTotalMinor(l.quantity, l.unit, props.currency)
  return t === null ? '—' : fmt(t)
}
</script>

<template>
  <fieldset ref="root" class="lines" :disabled="disabled">
    <legend class="section-lbl lines-legend">LINES</legend>

    <p v-if="errors.lines" class="lines-msg lines-msg-error" role="alert">{{ errors.lines }}</p>

    <ol class="lines-list">
      <li v-for="(l, i) in modelValue" :key="l.key" class="line" :aria-label="`Line ${i + 1}`">
        <div class="line-desc">
          <label :for="fieldId(path(i, 'description'))" class="input-label line-label">DESCRIPTION {{ i + 1 }} *</label>
          <input
            :id="fieldId(path(i, 'description'))"
            class="hud-input"
            :value="l.description"
            :maxlength="MAX_LINE_DESCRIPTION"
            autocomplete="off"
            :aria-invalid="problemFor(i, 'description') ? 'true' : undefined"
            :aria-describedby="describedBy(i, 'description', false)"
            @input="onInput(i, 'description', $event)"
            @blur="touch(i, 'description')"
          >
          <p v-if="problemFor(i, 'description')" :id="fieldMessageId(path(i, 'description'))" class="lines-msg lines-msg-error">
            {{ problemFor(i, 'description') }}
          </p>
        </div>

        <div class="line-qty">
          <label :for="fieldId(path(i, 'quantity'))" class="input-label line-label">QTY *</label>
          <input
            :id="fieldId(path(i, 'quantity'))"
            class="hud-input"
            type="text"
            inputmode="numeric"
            :value="l.quantity"
            autocomplete="off"
            :aria-invalid="problemFor(i, 'quantity') ? 'true' : undefined"
            :aria-describedby="describedBy(i, 'quantity', false)"
            @input="onInput(i, 'quantity', $event)"
            @blur="touch(i, 'quantity')"
          >
          <p v-if="problemFor(i, 'quantity')" :id="fieldMessageId(path(i, 'quantity'))" class="lines-msg lines-msg-error">
            {{ problemFor(i, 'quantity') }}
          </p>
        </div>

        <div class="line-unit-code">
          <label :for="fieldId(path(i, 'unit_code'))" class="input-label line-label">UNIT</label>
          <select
            :id="fieldId(path(i, 'unit_code'))"
            class="hud-input"
            :value="l.unit_code"
            :aria-invalid="errors[path(i, 'unit_code')] ? 'true' : undefined"
            :aria-describedby="errors[path(i, 'unit_code')] ? fieldMessageId(path(i, 'unit_code')) : undefined"
            @change="onUnitCode(i, $event)"
          >
            <option v-for="u in UNIT_CODES" :key="u.value" :value="u.value" :selected="u.value === l.unit_code">{{ u.label }}</option>
          </select>
          <p v-if="errors[path(i, 'unit_code')]" :id="fieldMessageId(path(i, 'unit_code'))" class="lines-msg lines-msg-error">
            {{ errors[path(i, 'unit_code')] }}
          </p>
        </div>

        <div class="line-price">
          <label :for="fieldId(path(i, 'unit_minor'))" class="input-label line-label">UNIT PRICE ({{ currency }}) *</label>
          <input
            :id="fieldId(path(i, 'unit_minor'))"
            class="hud-input"
            type="text"
            inputmode="decimal"
            :value="l.unit"
            :placeholder="pricePlaceholder"
            autocomplete="off"
            :aria-invalid="problemFor(i, 'unit') ? 'true' : undefined"
            :aria-describedby="describedBy(i, 'unit', false)"
            @input="onInput(i, 'unit', $event)"
            @blur="touch(i, 'unit')"
          >
          <p v-if="problemFor(i, 'unit')" :id="fieldMessageId(path(i, 'unit_minor'))" class="lines-msg lines-msg-error">
            {{ problemFor(i, 'unit') }}
          </p>
        </div>

        <div class="line-total">
          <span :id="`${fieldId(path(i, 'total'))}-lbl`" class="input-label line-label">LINE TOTAL</span>
          <output class="line-total-val" :aria-labelledby="`${fieldId(path(i, 'total'))}-lbl`" :data-testid="`line-total-${i}`">{{ totalText(l) }}</output>
        </div>

        <button
          type="button"
          class="btn-hud btn-hud-ghost btn-hud-sm line-remove"
          :disabled="onlyOne"
          :aria-label="`Remove line ${i + 1}`"
          :title="onlyOne ? 'An invoice needs at least one line' : `Remove line ${i + 1}`"
          @click="remove(i)"
        >
          <span aria-hidden="true">✕</span>
        </button>
      </li>
    </ol>

    <div class="lines-foot">
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" :disabled="atMax" @click="add">
        + ADD LINE
      </button>
      <p class="lines-sub">
        <span>Subtotal (preview)</span>
        <strong data-testid="lines-subtotal">{{ fmt(subtotal) }}</strong>
      </p>
    </div>
    <p class="lines-msg">
      Quantity is a whole number (1 to {{ MAX_LINE_QUANTITY.toLocaleString('en') }}). Tax and totals are calculated by the server
      when you save; the figures here are only a preview. {{ atMax ? `Maximum of ${MAX_LINES} lines reached.` : '' }}
    </p>
    <p class="sr-only" role="status" aria-live="polite">{{ announcement }}</p>
  </fieldset>
</template>

<style scoped>
.lines { border: 0; padding: 0; margin: 0; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
.lines-legend { padding: 0; margin-bottom: 4px; color: var(--color-on-surface); font-weight: 700; letter-spacing: .08em; }
.lines-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
.line:focus-within { border-left-color: var(--color-primary); }
.line {
  display: grid;
  grid-template-columns: 70px minmax(110px, 1fr) minmax(90px, 1fr) auto auto;
  grid-template-areas:
    'desc desc desc desc desc'
    'qty unit price total remove';
  gap: 8px 10px;
  align-items: start;
  padding: 10px;
  background: var(--color-surface-container-low);
  border-left: 2px solid var(--color-outline-variant);
}
.line-desc { grid-area: desc; min-width: 0; }
.line-qty { grid-area: qty; min-width: 0; }
.line-unit-code { grid-area: unit; min-width: 0; }
.line-price { grid-area: price; min-width: 0; }
.line-total { grid-area: total; min-width: 70px; text-align: right; }
.line-remove { grid-area: remove; align-self: end; min-width: 32px; }
.line-label { display: block; }
.line-total-val { display: block; min-height: 32px; line-height: 32px; font-family: var(--font-command); font-weight: 700; letter-spacing: -.02em; font-size: 12px; font-variant-numeric: tabular-nums; color: var(--color-on-surface); }
.lines-msg { margin: 3px 0 0; font-family: var(--font-data); font-size: 11px; line-height: 1.4; color: var(--color-tertiary); }
.lines-msg-error { color: var(--color-error); }
.lines-foot { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.lines-sub { margin: 0; display: flex; gap: 8px; align-items: baseline; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.lines-sub strong { font-family: var(--font-command); letter-spacing: -.02em; color: var(--color-on-surface); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .line {
    grid-template-columns: 1fr 1fr;
    grid-template-areas: 'desc desc' 'qty unit' 'price total' 'remove remove';
  }
  .line-total { text-align: left; }
  .line-remove { justify-self: end; min-height: 44px; min-width: 44px; }
  .hud-input, .lines-foot .btn-hud { min-height: 44px; }
}
</style>
