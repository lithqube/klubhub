<script setup lang="ts">
// One confirmation dialog for every irreversible invoice action. Copy and
// button style come from the ACTIONS table; credit and correct ask for a
// reason (kept on the credit note).
import { computed, ref, watch } from 'vue'
import type { Invoice, InvoiceConfirmAction } from '../../types/finance'
import { formatMinor } from '../../utils/money'
import { invoiceNumberLabel } from '../../utils/invoiceDisplay'
import { useDialogFocus } from '../../utils/dialogFocus'
import Dialog from '#kui/components/ui/dialog/Dialog.vue'
import DialogContent from '#kui/components/ui/dialog/DialogContent.vue'
import DialogTitle from '#kui/components/ui/dialog/DialogTitle.vue'
import DialogDescription from '#kui/components/ui/dialog/DialogDescription.vue'

const props = withDefaults(defineProps<{
  open: boolean
  action: InvoiceConfirmAction | null
  invoice: Invoice | null
  busy?: boolean
  error?: string
}>(), { busy: false, error: '' })

const emit = defineEmits<{ 'update:open': [open: boolean]; confirm: [reason: string] }>()

const reason = ref('')
const titleEl = ref<InstanceType<typeof DialogTitle> | null>(null)
const REASON_MAX = 1000

watch(() => props.open, (o) => { if (o) reason.value = '' })

const number = computed(() => (props.invoice ? invoiceNumberLabel(props.invoice) : ''))

const cfg = computed(() => {
  const n = number.value
  switch (props.action) {
    case 'issue':
      return {
        title: 'ISSUE INVOICE?',
        body: 'Locks the amount, customer and your billing details and assigns the invoice number. Nothing is sent to the promoter.',
        confirm: 'ISSUE', busyLabel: 'ISSUING…', btn: 'btn-hud-cta', back: 'GO BACK', reason: false,
      }
    case 'cancel':
      return {
        title: 'CANCEL DRAFT?',
        body: 'The draft is voided and no invoice number is used. You can create a new invoice for this gig afterwards.',
        confirm: 'CANCEL DRAFT', busyLabel: 'CANCELLING…', btn: 'btn-hud-error', back: 'KEEP DRAFT', reason: false,
      }
    case 'credit':
      return {
        title: 'ISSUE CREDIT NOTE?',
        body: `Reverses ${n} in full with a numbered credit note. ${n} stays on record as CREDITED and can no longer take payments.`,
        confirm: 'ISSUE CREDIT NOTE', busyLabel: 'ISSUING…', btn: 'btn-hud-error', back: 'GO BACK', reason: true,
      }
    case 'correct':
      return {
        title: 'CORRECT INVOICE?',
        body: `Issues a credit note for ${n} and opens a new draft copy you can fix and issue. ${n} stays on record as CORRECTED.`,
        confirm: 'CREDIT & CREATE DRAFT', busyLabel: 'CORRECTING…', btn: 'btn-hud-violet', back: 'GO BACK', reason: true,
      }
    case 'pay': {
      const out = props.invoice ? formatMinor(props.invoice.outstanding_minor, props.invoice.currency) : ''
      return {
        title: 'MARK AS PAID?',
        body: `${out} is still outstanding in the payment ledger. Marking ${n} as paid closes it without recording that money.`,
        confirm: 'MARK AS PAID', busyLabel: 'SAVING…', btn: 'btn-hud-cta', back: 'GO BACK', reason: false,
      }
    }
    default:
      return { title: '', body: '', confirm: 'CONFIRM', busyLabel: '…', btn: 'btn-hud-cta', back: 'GO BACK', reason: false }
  }
})

const reasonError = computed(() => {
  if (!cfg.value.reason) return ''
  if (reason.value.trim() === '') return 'Add a reason; it is kept on the credit note.'
  if (reason.value.length > REASON_MAX) return `Keep the reason under ${REASON_MAX} characters.`
  return ''
})

const canConfirm = computed(() => !props.busy && !reasonError.value)

const { onOpenAutoFocus, onCloseAutoFocus } = useDialogFocus(() => props.open, titleEl)

function confirm(): void {
  if (canConfirm.value) emit('confirm', reason.value.trim())
}
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-w-[440px] w-[calc(100%-32px)] bg-surface-container" @open-auto-focus="onOpenAutoFocus" @close-auto-focus="onCloseAutoFocus">
      <DialogTitle ref="titleEl" tabindex="-1" class="icd-title">{{ cfg.title }}</DialogTitle>
      <DialogDescription class="icd-body">{{ cfg.body }}</DialogDescription>

      <form class="icd-form" novalidate @submit.prevent="confirm">
        <div v-if="cfg.reason">
          <label for="inv-confirm-reason" class="input-label" style="display:block;">REASON *</label>
          <textarea
            id="inv-confirm-reason"
            v-model="reason"
            class="hud-textarea"
            rows="3"
            required
            :aria-invalid="reason && reasonError ? 'true' : undefined"
            aria-describedby="inv-confirm-reason-msg"
          />
          <p id="inv-confirm-reason-msg" class="icd-msg">{{ reasonError || 'Kept on the credit note.' }}</p>
        </div>

        <p v-if="error" class="icd-msg icd-error" role="alert">{{ error }}</p>

        <div class="icd-actions">
          <button type="button" class="btn-hud btn-hud-ghost" @click="emit('update:open', false)">{{ cfg.back }}</button>
          <button type="submit" class="btn-hud" :class="cfg.btn" :disabled="!canConfirm">
            {{ busy ? cfg.busyLabel : cfg.confirm }}
          </button>
        </div>
      </form>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
.icd-title { font-size: 14px; outline: none; }
.icd-body { margin-top: 8px; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.icd-form { display: flex; flex-direction: column; gap: 12px; margin-top: 16px; }
.icd-msg { margin: 4px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.icd-error { color: var(--color-error); }
.icd-actions { display: flex; justify-content: flex-end; gap: 8px; }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .icd-actions { flex-direction: column-reverse; }
  .icd-actions .btn-hud { width: 100%; }
}
</style>
