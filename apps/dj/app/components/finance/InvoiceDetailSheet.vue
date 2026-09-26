<script setup lang="ts">
// Invoice detail: right-hand sheet on desktop, full-height bottom sheet on
// phones. Open/close and the loaded invoice live in the invoice store, so
// any entry point (finance list, gig form, gig actions) drives the same sheet.
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useMediaQuery } from '@vueuse/core'
import { DialogDescription } from 'radix-vue'
import type { Invoice, InvoiceConfirmAction } from '../../types/finance'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import {
  DISPLAY_STATUS_BADGE,
  invoiceDisplayStatus,
  invoiceNumberLabel,
  shortDate,
  statusLabel,
} from '../../utils/invoiceDisplay'
import { inlineErrorMessage } from '../../utils/invoiceFields'
import { focusWhenReady, useDialogFocus } from '../../utils/dialogFocus'
import { useToast } from '#kui/components/ui/toast/use-toast'
import Sheet from '#kui/components/ui/sheet/Sheet.vue'
import DialogTitle from '#kui/components/ui/dialog/DialogTitle.vue'
import InvoiceActionsMenu from './InvoiceActionsMenu.vue'
import InvoiceDraftEditor from './InvoiceDraftEditor.vue'
import InvoiceTotals from './InvoiceTotals.vue'
import InvoiceBalance from './InvoiceBalance.vue'
import InvoicePartySummary from './InvoicePartySummary.vue'
import PaymentLedger from './PaymentLedger.vue'
import InvoiceConfirmDialog from './InvoiceConfirmDialog.vue'

const store = useInvoiceStore()
const { detailOpen, detailId, current, lines, payments, currentLoading, currentError, notice, invoices, gigInvoices, gigById } =
  storeToRefs(store)
const { toast } = useToast()

const isMobile = useMediaQuery('(max-width: 768px)')
const titleEl = ref<InstanceType<typeof DialogTitle> | null>(null)

const inv = computed(() => (current.value && current.value.id === detailId.value ? current.value : null))
const number = computed(() => (inv.value ? invoiceNumberLabel(inv.value) : 'INVOICE'))
const display = computed(() => (inv.value ? invoiceDisplayStatus(inv.value) : 'draft'))
const isCreditNote = computed(() => inv.value?.kind === 'credit_note')
const takesPayments = computed(() => inv.value?.kind === 'invoice' && (inv.value.status === 'issued' || inv.value.status === 'paid'))
const gigLabel = computed(() => (inv.value ? gigById.value[inv.value.gig_id]?.label ?? 'VIEW GIG' : ''))

// ── Related documents (credit note, replacement, original) ──
const known = computed(() => {
  const map = new Map<string, Invoice>()
  for (const i of invoices.value) map.set(i.id, i)
  if (inv.value) for (const i of gigInvoices.value[inv.value.gig_id] ?? []) map.set(i.id, i)
  return map
})
const creditNote = computed(() =>
  inv.value ? [...known.value.values()].find((i) => i.kind === 'credit_note' && i.credits_invoice_id === inv.value!.id) ?? null : null,
)
const replacement = computed(() => (inv.value?.replaced_by_invoice_id ? known.value.get(inv.value.replaced_by_invoice_id) ?? null : null))
const original = computed(() => (inv.value?.credits_invoice_id ? known.value.get(inv.value.credits_invoice_id) ?? null : null))

watch(() => inv.value?.id, () => {
  const i = inv.value
  if (!i) return
  // Loading swaps the skeleton for content; if the focus trap parked focus
  // on the panel itself meanwhile, move it to the title.
  if (document.activeElement?.getAttribute('role') === 'dialog') focusWhenReady(titleEl)
  if (i.kind === 'credit_note' || i.status === 'credited' || i.status === 'corrected') {
    store.fetchInvoicesForGig(i.gig_id).catch(() => undefined)
  }
  if (!store.gigsLoaded) void store.fetchGigOptions()
})

// Title gets focus on open and when another invoice is shown in place
// (related-document links, correct → replacement); the opener gets it back.
const { onOpenAutoFocus, onCloseAutoFocus } = useDialogFocus(detailOpen, titleEl, detailId)

function openRelated(id: string): void {
  void store.openDetail(id)
}

// ── Actions ──
const confirmOpen = ref(false)
const confirmAction = ref<InvoiceConfirmAction | null>(null)
const busy = ref(false)
const confirmError = ref('')
const actionError = ref('')

function requestAction(action: InvoiceConfirmAction): void {
  actionError.value = ''
  if (action === 'pay' && inv.value && inv.value.outstanding_minor === 0) {
    void run('pay', '')
    return
  }
  confirmError.value = ''
  confirmAction.value = action
  // Let the dropdown finish closing (and return focus) before the dialog
  // traps focus, so the dialog remembers the trigger for focus restore.
  setTimeout(() => { confirmOpen.value = true }, 0)
}

async function run(action: InvoiceConfirmAction, reason: string): Promise<void> {
  const i = inv.value
  if (!i || busy.value) return
  busy.value = true
  confirmError.value = ''
  const label = invoiceNumberLabel(i)
  try {
    switch (action) {
      case 'issue': {
        const issued = await store.issueInvoice(i.id)
        toast({ title: 'Invoice issued', description: `${invoiceNumberLabel(issued)} is issued. Nothing was sent to the promoter.` })
        break
      }
      case 'cancel':
        await store.cancelInvoice(i.id)
        toast({ title: 'Draft cancelled', description: 'No invoice number was used.' })
        break
      case 'pay':
        await store.markPaid(i.id)
        toast({ title: 'Marked as paid', description: `${label} is paid.` })
        break
      case 'credit': {
        const res = await store.issueCreditNote(i.id, reason)
        toast({ title: 'Credit note issued', description: `${invoiceNumberLabel(res.credit_note)} reverses ${label}.` })
        break
      }
      case 'correct':
        await store.correctInvoice(i.id, reason)
        toast({ title: 'Replacement draft ready', description: `${label} was credited. Edit the new draft and issue it.` })
        break
    }
    confirmOpen.value = false
  } catch (e) {
    const msg = inlineErrorMessage(toFinanceError(e))
    if (msg === null) {
      // 409: the sheet banner explains it and shows the latest version.
      confirmOpen.value = false
    } else if (confirmOpen.value) {
      confirmError.value = msg
      if (toFinanceError(e).code === 'not_issuable') confirmOpen.value = false
      actionError.value = msg
    } else {
      actionError.value = msg
    }
  } finally {
    busy.value = false
  }
}

function retry(): void {
  if (detailId.value) void store.fetchInvoice(detailId.value)
}
</script>

<template>
  <Sheet
    :open="detailOpen"
    :side="isMobile ? 'bottom' : 'right'"
    :class="isMobile ? 'h-[100dvh] w-full overflow-y-auto p-4' : 'w-full sm:max-w-xl overflow-y-auto p-6'"
    @update:open="store.setDetailOpen"
    @open-auto-focus="onOpenAutoFocus"
    @close-auto-focus="onCloseAutoFocus"
  >
    <div class="ids">
      <header class="ids-head">
        <div class="ids-title-row">
          <DialogTitle ref="titleEl" tabindex="-1" class="ids-title">{{ number }}</DialogTitle>
          <span v-if="inv" class="badge-hud" :class="DISPLAY_STATUS_BADGE[display]">{{ statusLabel(inv) }}</span>
          <span v-if="isCreditNote" class="badge-hud badge-published">CREDIT NOTE</span>
        </div>
        <div v-if="inv" class="ids-sub">
          <NuxtLink
            :to="{ path: '/gigs', query: { gig: inv.gig_id } }"
            class="ids-link"
            @click="store.setDetailOpen(false)"
          >
            GIG: {{ gigLabel }}
          </NuxtLink>
          <span class="section-lbl">{{ inv.issued_at ? `ISSUED ${shortDate(inv.issued_at)}` : `CREATED ${shortDate(inv.created_at)}` }}</span>
          <span v-if="inv.due_at && inv.kind === 'invoice'" class="section-lbl">DUE {{ shortDate(inv.due_at) }}</span>
          <div class="ids-menu"><InvoiceActionsMenu :invoice="inv" @action="requestAction" /></div>
        </div>
      </header>
      <DialogDescription class="sr-only">Invoice details, billing and payments.</DialogDescription>

      <div v-if="notice" class="ids-banner" role="status">
        <span>{{ notice }}</span>
        <button type="button" class="ids-banner-x" aria-label="Dismiss notice" @click="store.clearNotice()">✕</button>
      </div>
      <p v-if="actionError" class="ids-error" role="alert">{{ actionError }}</p>

      <div v-if="!inv && currentLoading" class="ids-skel" aria-busy="true" aria-label="Loading invoice">
        <div v-for="n in 4" :key="n" class="ids-skel-bar" />
      </div>

      <div v-else-if="!inv && currentError" class="ids-state" role="alert">
        <p>{{ currentError.code === 'not_found' ? 'This invoice no longer exists.' : currentError.message }}</p>
        <button v-if="currentError.code !== 'not_found'" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" @click="retry">RETRY</button>
      </div>

      <template v-else-if="inv">
        <!-- Links between credited/corrected invoices and their documents -->
        <div v-if="inv.status === 'credited' || inv.status === 'corrected' || isCreditNote" class="ids-related">
          <p v-if="inv.status === 'credited'">
            Reversed by credit note
            <button v-if="creditNote" type="button" class="ids-link" @click="openRelated(creditNote.id)">{{ invoiceNumberLabel(creditNote) }}</button>
            <span v-else>(loading…)</span>. Read-only.
          </p>
          <p v-if="inv.status === 'corrected'">
            Corrected: credited by
            <button v-if="creditNote" type="button" class="ids-link" @click="openRelated(creditNote.id)">{{ invoiceNumberLabel(creditNote) }}</button>
            <span v-else>a credit note</span>
            and replaced by
            <button v-if="inv.replaced_by_invoice_id" type="button" class="ids-link" @click="openRelated(inv.replaced_by_invoice_id)">
              {{ replacement ? invoiceNumberLabel(replacement) : 'the new draft' }}
            </button>. Read-only.
          </p>
          <p v-if="isCreditNote && inv.credits_invoice_id">
            Credits
            <button type="button" class="ids-link" @click="openRelated(inv.credits_invoice_id)">{{ original ? invoiceNumberLabel(original) : 'the original invoice' }}</button>.
          </p>
        </div>

        <template v-if="inv.status === 'draft'">
          <InvoiceDraftEditor :invoice="inv" :issuing="busy && confirmAction === 'issue'" @issue="requestAction('issue')" />
          <InvoiceTotals :invoice="inv" :lines="lines" />
        </template>

        <template v-else>
          <InvoiceBalance v-if="takesPayments" :invoice="inv" />
          <InvoicePartySummary :party="inv.customer" label="BILL TO" />
          <InvoiceTotals :invoice="inv" :lines="lines" />
          <PaymentLedger v-if="takesPayments" :invoice="inv" :payments="payments" />
          <PaymentLedger v-else-if="!isCreditNote && payments.length" :invoice="inv" :payments="payments" locked />
        </template>
      </template>
    </div>

    <InvoiceConfirmDialog
      v-model:open="confirmOpen"
      :action="confirmAction"
      :invoice="inv"
      :busy="busy"
      :error="confirmError"
      @confirm="run(confirmAction!, $event)"
    />
  </Sheet>
</template>

<style scoped>
.ids { display: flex; flex-direction: column; gap: 18px; padding-bottom: 24px; }
.ids-head { display: flex; flex-direction: column; gap: 6px; padding-right: 32px; }
.ids-title-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.ids-title { font-size: 16px; outline: none; overflow-wrap: anywhere; }
.ids-sub { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 12px; }
.ids-menu { margin-left: auto; }
.ids-link { padding: 0; background: none; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--color-primary); text-decoration: underline; text-underline-offset: 2px; }
.ids-banner { display: flex; align-items: flex-start; gap: 8px; padding: 10px 12px; background: color-mix(in srgb, var(--color-secondary) 10%, transparent); border-left: 3px solid var(--color-secondary); font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
.ids-banner span { flex: 1; }
.ids-banner-x { background: none; border: 0; cursor: pointer; color: var(--color-on-surface-variant); min-width: 24px; min-height: 24px; }
.ids-error { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.ids-state { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface); }
.ids-state p { margin: 0; }
.ids-related { padding: 10px 12px; background: var(--color-surface-container-low); border-left: 3px solid var(--color-status-archived); font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
.ids-related p { margin: 0; }
.ids-skel { display: flex; flex-direction: column; gap: 10px; }
.ids-skel-bar { height: 44px; background: var(--color-surface-container-high); animation: ids-pulse 1.2s ease-in-out infinite; }
.ids-skel-bar:first-child { height: 120px; }
@keyframes ids-pulse { 50% { opacity: .5; } }
@media (prefers-reduced-motion: reduce) { .ids-skel-bar { animation: none; } }
@media (max-width: 768px) {
  .ids-link { min-height: 44px; display: inline-flex; align-items: center; }
  .ids-banner-x { min-width: 44px; min-height: 44px; }
}
</style>
