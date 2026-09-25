<script setup lang="ts">
// Records a deposit, payment or refund against an issued/paid invoice.
// The amount is checked against the balance before saving (SAVE stays
// disabled and the reason is announced); the API re-checks on save.
import { computed, ref, watch } from 'vue'
import type { Invoice, Payment, PaymentKind } from '../../types/finance'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import { minorToDecimalString, formatMinor } from '../../utils/money'
import { todayIso, toApiTime } from '../../utils/invoiceDisplay'
import { PAYMENT_KINDS, checkPaymentAmount } from '../../utils/paymentForm'
import { inlineErrorMessage } from '../../utils/invoiceFields'

const props = defineProps<{ invoice: Invoice; payments: Payment[] }>()
const emit = defineEmits<{ saved: [payment: Payment]; cancel: [] }>()

const store = useInvoiceStore()

function defaultKind(inv: Invoice): PaymentKind {
  // Only issued invoices owe money (§4.1); on a paid invoice the likely entry is a refund.
  if (inv.outstanding_minor === 0 && inv.received_minor > 0) return 'refund'
  return inv.received_minor > 0 || inv.pending_minor > 0 ? 'payment' : 'deposit'
}

const kind = ref<PaymentKind>(defaultKind(props.invoice))
const amountText = ref('')
const received = ref(true)
const receivedAt = ref(todayIso())
const method = ref('bank transfer')
const reference = ref('')
const saving = ref(false)
const serverError = ref('')

const check = computed(() => checkPaymentAmount(amountText.value, kind.value, props.invoice, props.payments))
const canSave = computed(() => check.value.valid && !saving.value && (!received.value || !!receivedAt.value))
const liveMessage = computed(() => serverError.value || check.value.message)

watch([kind, amountText], () => { serverError.value = '' })

function fillRemaining(): void {
  amountText.value = minorToDecimalString(check.value.limit, props.invoice.currency)
}

async function save(): Promise<void> {
  if (!canSave.value || check.value.amount === null) return
  saving.value = true
  serverError.value = ''
  try {
    const p = await store.createPayment(props.invoice.id, {
      kind: kind.value,
      amount_minor: check.value.amount,
      method: method.value.trim(),
      reference: reference.value.trim(),
      received_at: received.value ? toApiTime(receivedAt.value) : null,
      received: received.value,
    })
    amountText.value = ''
    emit('saved', p)
  } catch (e) {
    serverError.value = inlineErrorMessage(toFinanceError(e)) ?? 'This invoice changed elsewhere. Review the balance and try again.'
  } finally {
    saving.value = false
  }
}

const limitLabel = computed(() =>
  kind.value === 'refund'
    ? `RECEIVED ${formatMinor(check.value.limit, props.invoice.currency)}`
    : `OUTSTANDING ${formatMinor(check.value.limit, props.invoice.currency)}`,
)
</script>

<template>
  <form class="pf" novalidate aria-labelledby="inv-pf-title" @submit.prevent="save">
    <h4 id="inv-pf-title" class="section-lbl pf-title">RECORD MONEY</h4>

    <div role="radiogroup" aria-label="Kind" class="pf-kinds">
      <label v-for="k in PAYMENT_KINDS" :key="k.value" class="pf-kind" :class="{ 'pf-kind-on': kind === k.value }">
        <input v-model="kind" type="radio" name="inv-pf-kind" :value="k.value" class="sr-only">
        <span>{{ k.label }}</span>
      </label>
    </div>

    <div>
      <div class="pf-amount-head">
        <label for="inv-pf-amount" class="input-label" style="margin:0;">AMOUNT ({{ invoice.currency }})</label>
        <span class="section-lbl">{{ limitLabel }}</span>
      </div>
      <div class="pf-amount-row">
        <input
          id="inv-pf-amount"
          v-model="amountText"
          class="hud-input"
          inputmode="decimal"
          autocomplete="off"
          :placeholder="minorToDecimalString(check.limit, invoice.currency)"
          :aria-invalid="liveMessage ? 'true' : undefined"
          aria-describedby="inv-pf-msg"
        >
        <button
          type="button"
          class="btn-hud btn-hud-ghost btn-hud-sm pf-fill"
          :disabled="check.limit === 0"
          @click="fillRemaining"
        >
          FILL REMAINING
        </button>
      </div>
      <p id="inv-pf-msg" class="pf-msg" :class="{ 'pf-msg-error': !!liveMessage }" aria-live="polite">{{ liveMessage }}</p>
    </div>

    <label class="pf-check">
      <input v-model="received" type="checkbox">
      <span class="input-label" style="margin:0;">ALREADY RECEIVED</span>
    </label>
    <p class="pf-msg" style="margin-top:-6px;">
      {{ received ? 'Saved as completed and counted as received.' : 'Saved as pending until you mark it received.' }}
    </p>

    <div class="pf-grid">
      <div v-if="received">
        <label for="inv-pf-date" class="input-label pf-label">RECEIVED ON</label>
        <input id="inv-pf-date" v-model="receivedAt" type="date" class="hud-input" required>
      </div>
      <div>
        <label for="inv-pf-method" class="input-label pf-label">METHOD</label>
        <input id="inv-pf-method" v-model="method" class="hud-input" autocomplete="off">
      </div>
      <div>
        <label for="inv-pf-ref" class="input-label pf-label">REFERENCE</label>
        <input id="inv-pf-ref" v-model="reference" class="hud-input" autocomplete="off">
      </div>
    </div>

    <div class="pf-actions">
      <button type="button" class="btn-hud btn-hud-ghost" @click="emit('cancel')">CANCEL</button>
      <button type="submit" class="btn-hud btn-hud-cta" :disabled="!canSave">
        {{ saving ? 'SAVING…' : 'SAVE' }}
      </button>
    </div>
  </form>
</template>

<style scoped>
.pf { display: flex; flex-direction: column; gap: 10px; padding: 12px 14px; background: var(--color-surface-container-low); border-left: 2px solid var(--color-primary); }
.pf-title { margin: 0; font-weight: 600; }
.pf-kinds { display: flex; gap: 4px; flex-wrap: wrap; }
.pf-kind { display: inline-flex; align-items: center; justify-content: center; min-height: 32px; padding: 0 12px; cursor: pointer; border: 1px dashed color-mix(in srgb, var(--color-on-surface) 25%, transparent); font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.pf-kind-on { border-style: solid; border-color: var(--color-primary); color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 8%, transparent); }
.pf-kind:focus-within { outline: 2px solid var(--color-primary); outline-offset: 1px; }
.pf-amount-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; margin-bottom: 4px; }
.pf-amount-row { display: flex; gap: 8px; }
.pf-fill { height: 40px; min-height: 40px; }
.pf-msg { margin: 4px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.pf-msg:empty { display: none; }
.pf-msg-error { color: var(--color-error); }
.pf-check { display: inline-flex; align-items: center; gap: 8px; min-height: 32px; cursor: pointer; }
.pf-check input { width: 16px; height: 16px; accent-color: var(--color-primary); }
.pf-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
.pf-label { display: block; }
.pf-actions { display: flex; justify-content: flex-end; gap: 8px; }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .pf-grid { grid-template-columns: 1fr; }
  .pf-kind, .pf-check, .pf-fill, .hud-input { min-height: 44px; }
  .pf-kind { flex: 1; }
  .pf-actions .btn-hud { flex: 1; }
}
</style>
