<script setup lang="ts">
// "Send by email": mails the issued invoice to its customer with the PDF and,
// when the invoice has one, the e-invoice XML attached. Sending is outward
// facing and cannot be undone, so it takes a second, explicit click that names
// the recipient. A delivery that fails is kept and retried by the server, which
// the result says rather than calling it an error.
import { ref } from 'vue'
import type { Invoice, InvoiceEmailResult, IssueProblem } from '../../types/finance'
import { FinanceApiError, useInvoiceStore } from '../../stores/invoice'
import { invoiceNumberLabel } from '../../utils/invoiceDisplay'

const props = defineProps<{ invoice: Invoice }>()
const store = useInvoiceStore()

const open = ref(false)
const to = ref(props.invoice.customer.email ?? '')
const withEInvoice = ref(true)
const phase = ref<'idle' | 'confirm' | 'sending' | 'done'>('idle')
const result = ref<InvoiceEmailResult | null>(null)
const unavailable = ref(false)
const failure = ref('')
const problems = ref<IssueProblem[]>([])

function reset(): void {
  phase.value = 'idle'
  result.value = null
  failure.value = ''
  problems.value = []
}

function toggle(): void {
  open.value = !open.value
  if (!open.value) reset()
}

function ask(): void {
  reset()
  if (!to.value.trim()) {
    failure.value = 'Enter an email address to send to.'
    return
  }
  phase.value = 'confirm'
}

async function send(): Promise<void> {
  phase.value = 'sending'
  failure.value = ''
  problems.value = []
  try {
    const res = await store.sendInvoiceEmail(props.invoice.id, {
      to_email: to.value.trim(),
      // Unticked sends the PDF alone; ticked attaches the e-invoice when there is one.
      ...(withEInvoice.value ? {} : { include_einvoice: false }),
    })
    unavailable.value = res === null
    result.value = res
    phase.value = 'done'
  } catch (e) {
    failure.value = e instanceof Error ? e.message : 'Could not send the email.'
    if (e instanceof FinanceApiError) problems.value = e.problems
    phase.value = 'idle'
  }
}
</script>

<template>
  <section class="iem" :aria-label="`Send ${invoiceNumberLabel(invoice)} by email`">
    <button type="button" class="iem-toggle" :aria-expanded="open" @click="toggle">
      <span class="section-lbl">SEND BY EMAIL</span>
      <span class="iem-chev" aria-hidden="true">{{ open ? '−' : '+' }}</span>
    </button>

    <div v-if="open" class="iem-body">
      <p v-if="unavailable" class="iem-note" role="status">Email isn't enabled on this server.</p>

      <template v-else>
        <label class="iem-field">
          <span class="section-lbl">TO</span>
          <input
            v-model="to"
            type="email"
            class="input-hud"
            autocomplete="off"
            :disabled="phase === 'sending'"
            @input="reset"
          >
        </label>
        <label class="iem-check">
          <input v-model="withEInvoice" type="checkbox" :disabled="phase === 'sending'" @change="reset">
          <span>Attach the e-invoice (XML) when this invoice has one</span>
        </label>
        <p class="iem-note">The PDF is always attached.</p>

        <div v-if="phase === 'idle'" class="iem-actions">
          <button type="button" class="btn-hud btn-hud-sm" @click="ask">SEND…</button>
        </div>
        <div v-else-if="phase === 'confirm'" class="iem-actions" role="group" aria-label="Confirm sending">
          <span class="iem-note">Send {{ invoiceNumberLabel(invoice) }} to <strong>{{ to.trim() }}</strong>?</span>
          <button type="button" class="btn-hud btn-hud-sm" @click="send">SEND NOW</button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" @click="reset">CANCEL</button>
        </div>
        <p v-else-if="phase === 'sending'" class="iem-note" role="status">Sending…</p>

        <p v-if="phase === 'done' && result" class="iem-note" role="status">
          <template v-if="result.status === 'sent'">
            Sent to {{ result.to_email }} with {{ result.attachment_ids.length }}
            {{ result.attachment_ids.length === 1 ? 'file' : 'files' }}.
            <template v-if="result.reply_to"> Replies go to {{ result.reply_to }}.</template>
          </template>
          <template v-else>
            Not delivered yet. It is saved and will be retried automatically.
          </template>
        </p>

        <div v-if="failure" role="alert">
          <p class="iem-error">{{ failure }}</p>
          <ul v-if="problems.length" class="iem-problems">
            <li v-for="p in problems" :key="p.field + p.message">{{ p.message }}</li>
          </ul>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.iem { border: 1px dashed var(--color-outline-variant); }
.iem-toggle { display: flex; width: 100%; align-items: center; justify-content: space-between; padding: 8px 12px; background: none; border: 0; cursor: pointer; color: var(--color-on-surface); min-height: 36px; }
.iem-chev { font-family: var(--font-terminal); font-size: 14px; }
.iem-body { display: flex; flex-direction: column; gap: 10px; padding: 0 12px 12px; }
.iem-field { display: flex; flex-direction: column; gap: 4px; }
.iem-check { display: flex; align-items: center; gap: 8px; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
.iem-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.iem-note { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.iem-note strong { color: var(--color-on-surface); font-weight: 600; overflow-wrap: anywhere; }
.iem-error { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.iem-problems { margin: 6px 0 0; padding-left: 16px; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
@media (max-width: 768px) {
  .iem-toggle { min-height: 44px; }
  .iem-actions .btn-hud { min-height: 44px; }
}
</style>
