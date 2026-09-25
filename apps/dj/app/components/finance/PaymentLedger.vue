<script setup lang="ts">
// Money recorded against an issued or paid invoice. Pending rows can be
// marked received; new rows go through PaymentForm.
import { computed, nextTick, ref } from 'vue'
import type { Invoice, Payment } from '../../types/finance'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import { formatMinor } from '../../utils/money'
import { shortDate } from '../../utils/invoiceDisplay'
import { inlineErrorMessage } from '../../utils/invoiceFields'
import { useToast } from '#kui/components/ui/toast/use-toast'
import PaymentForm from './PaymentForm.vue'

const props = withDefaults(defineProps<{ invoice: Invoice; payments: Payment[]; locked?: boolean }>(), { locked: false })

const store = useInvoiceStore()
const { toast } = useToast()

const formOpen = ref(false)
const busyId = ref<string | null>(null)
const error = ref('')
const addBtn = ref<HTMLButtonElement | null>(null)

const rows = computed(() => [...props.payments].sort((a, b) => a.created_at.localeCompare(b.created_at)))

const STATUS_BADGE: Record<Payment['status'], string> = {
  pending: 'badge-draft',
  completed: 'badge-ready',
  failed: 'badge-failed',
  refunded: 'badge-archived',
}

function signed(p: Payment): string {
  const v = formatMinor(p.amount_minor, p.currency || props.invoice.currency)
  return p.kind === 'refund' ? `−${v}` : v
}

async function markReceived(p: Payment): Promise<void> {
  busyId.value = p.id
  error.value = ''
  try {
    await store.markPaymentReceived(p)
    toast({ title: 'Payment received', description: `${signed(p)} marked as received.` })
  } catch (e) {
    error.value = inlineErrorMessage(toFinanceError(e)) ?? 'This payment changed elsewhere. Showing the latest; try again.'
  } finally {
    busyId.value = null
  }
}

async function closeForm(): Promise<void> {
  formOpen.value = false
  await nextTick()
  addBtn.value?.focus()
}

function onSaved(p: Payment): void {
  toast({ title: 'Payment recorded', description: `${signed(p)} ${p.kind} saved.` })
  void closeForm()
}
</script>

<template>
  <section class="led" aria-labelledby="inv-led-title">
    <div class="led-head">
      <h3 id="inv-led-title" class="section-lbl led-title">PAYMENTS</h3>
      <button
        v-if="!formOpen && !locked"
        ref="addBtn"
        type="button"
        class="btn-hud btn-hud-ghost btn-hud-sm led-add"
        @click="formOpen = true"
      >
        + RECORD PAYMENT
      </button>
    </div>

    <p v-if="error" class="led-error" role="alert">{{ error }}</p>

    <ul v-if="rows.length" class="led-list">
      <li v-for="p in rows" :key="p.id" class="led-row" :class="p.status === 'pending' ? 'accent-bar-draft' : p.status === 'completed' ? 'accent-bar-ready' : 'accent-bar-archived'">
        <div class="led-info">
          <div class="led-kind">{{ p.kind.toUpperCase() }}<template v-if="p.method"> · {{ p.method }}</template></div>
          <div class="led-meta">
            {{ p.received_at ? shortDate(p.received_at) : 'NOT RECEIVED YET' }}<template v-if="p.reference"> · {{ p.reference }}</template>
          </div>
        </div>
        <div class="led-amount">{{ signed(p) }}</div>
        <span class="badge-hud" :class="STATUS_BADGE[p.status]">{{ p.status.toUpperCase() }}</span>
        <button
          v-if="p.status === 'pending' && !locked"
          type="button"
          class="btn-hud btn-hud-ghost btn-hud-xs led-mark"
          :disabled="busyId === p.id"
          :aria-label="`Mark ${p.kind} of ${signed(p)} received`"
          @click="markReceived(p)"
        >
          {{ busyId === p.id ? '…' : 'MARK RECEIVED' }}
        </button>
      </li>
    </ul>
    <p v-else-if="!formOpen" class="led-empty">No money recorded yet.</p>

    <p v-if="locked && rows.length" class="led-empty">This invoice no longer takes payments; money received stays on record here.</p>

    <PaymentForm
      v-if="formOpen && !locked"
      :invoice="invoice"
      :payments="payments"
      @saved="onSaved"
      @cancel="closeForm"
    />
  </section>
</template>

<style scoped>
.led { display: flex; flex-direction: column; gap: 8px; }
.led-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.led-title { margin: 0; font-weight: 600; }
.led-list { list-style: none; margin: 0; padding: 0; background: var(--color-surface-container-low); }
.led-row { display: flex; align-items: center; gap: 10px; padding: 8px 12px; border-bottom: 1px dashed color-mix(in srgb, var(--color-on-surface) 10%, transparent); }
.led-row:last-child { border-bottom: 0; }
.led-info { flex: 1; min-width: 0; }
.led-kind { font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
.led-meta { font-family: var(--font-terminal); font-size: 8px; letter-spacing: .05em; text-transform: uppercase; color: var(--color-tertiary); margin-top: 2px; }
.led-amount { flex-shrink: 0; font-family: var(--font-command); font-size: 13px; font-weight: 700; letter-spacing: -.02em; color: var(--color-on-surface); }
.led-empty { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.led-error { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
@media (max-width: 768px) {
  .led-row { flex-wrap: wrap; }
  .led-add, .led-mark { min-height: 44px; height: 44px; }
  .led-mark { width: 100%; }
}
</style>
