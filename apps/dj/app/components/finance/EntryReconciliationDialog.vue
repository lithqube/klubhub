<script setup lang="ts">
// Reconciliation panel (FIN-04 / FIN-05). Fetches /api/v1/finance/reconciliations
// for one gig and renders the user prompt. Also accepts a metadata blob the
// gig update handler routes in from gig finance_reconciliation.
//
// Allowed actions per reason:
//   fee_changed     → update | keep
//   currency_changed→ update | keep
//   payment_reversed→ delete | void | keep
//
// The component owns the action buttons; the page reacts to the emitted
// event and dispatches into the earnings store.
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import Dialog from '#kui/components/ui/dialog/Dialog.vue'
import DialogContent from '#kui/components/ui/dialog/DialogContent.vue'
import DialogTitle from '#kui/components/ui/dialog/DialogTitle.vue'
import DialogDescription from '#kui/components/ui/dialog/DialogDescription.vue'
import { useEarningsStore } from '../../stores/earnings'
import { formatMinor } from '../../utils/money'
import { useDialogFocus } from '../../utils/dialogFocus'
import type {
  EntryReconciliation,
  GigFinanceReconciliation,
  ReconciliationAction,
  ReconciliationReason,
} from '../../types/finance'

const props = defineProps<{
  /** Active gig id — drives the on-mount fetch + refresh. */
  gigId: string | null
  /** Optional inline reconciliation metadata from the gig update response. */
  metadata?: GigFinanceReconciliation | null
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  resolved: [rec: EntryReconciliation | { id: string; resolution: ReconciliationAction }]
  dismissed: []
}>()

// Local UI shape: only `id`, `entry_id`, `reason`, `allowed_actions`,
// and `updated_at` are mandatory. `gig_amount_minor`, `gig_currency`,
// `gig_payment_status`, `entry_updated_at` come from the durable row when
// available — inline `GigFinanceReconciliation` payloads only carry the
// token + decision metadata.
interface ResolvedReconciliation {
  id: string
  entry_id: string
  reason: ReconciliationReason
  allowed_actions: ReconciliationAction[]
  gig_amount_minor?: number
  gig_currency?: string
  gig_payment_status?: string
  entry_updated_at?: string
  updated_at: string
}

const store = useEarningsStore()
const { reconciliationByGig, reconciliationErrors } = storeToRefs(store)

const open = defineModel<boolean>('open', { default: false })
const titleEl = ref<InstanceType<typeof DialogTitle> | null>(null)
const { onOpenAutoFocus, onCloseAutoFocus } = useDialogFocus(open, titleEl)

const reasonCopy: Record<ReconciliationReason, { title: string; body: string }> = {
  fee_changed: {
    title: 'FEE CHANGED',
    body: 'The gig fee changed since the income entry was created. Update the entry to match the latest amount, or keep the original.',
  },
  currency_changed: {
    title: 'CURRENCY CHANGED',
    body: 'The gig currency changed since the income entry was created. Update the entry to match the latest currency, or keep the original.',
  },
  payment_reversed: {
    title: 'GIG NO LONGER PAID',
    body: 'This gig is no longer marked paid. Delete the income entry, void it to keep it on the books, or keep it active.',
  },
}

const labelFor = (a: ReconciliationAction): string => {
  switch (a) {
    case 'update': return 'UPDATE INCOME'
    case 'delete': return 'DELETE INCOME'
    case 'void':   return 'VOID INCOME'
    case 'keep':   return 'KEEP ORIGINAL'
  }
}

const styleFor = (a: ReconciliationAction): string => {
  switch (a) {
    case 'update': return 'btn-hud-cta'
    case 'delete': return 'btn-hud-error'
    case 'void':   return 'btn-hud-violet'
    case 'keep':   return 'btn-hud'
  }
}

const busy = ref(false)
const busyAction = ref<ReconciliationAction | null>(null)
const error = ref('')

const rec = computed<ResolvedReconciliation | null>(() => {
  if (props.metadata && props.metadata.id) {
    const m = props.metadata as unknown as ResolvedReconciliation
    return m
  }
  if (!props.gigId) return null
  const fromServer = reconciliationByGig.value[props.gigId]
  return (fromServer as ResolvedReconciliation | null) ?? null
})

/**
 * Fetch the reconciliation row for `gigId` so `cfg` populates as soon as the
 * dialog is opened. This never sets `open` itself: the parent decides when
 * to show this dialog (it only mounts/passes `open` once there's actually
 * something to reconcile, whether from a fresh gig-update response or a
 * durable GET) and this component just loads the data for that decision.
 */
const fetchAndOpen = async (): Promise<void> => {
  if (!props.gigId) return
  await store.fetchReconciliationForGig(props.gigId)
}

watch(() => props.gigId, (id) => { if (id) void fetchAndOpen() }, { immediate: true })

const cfg = computed(() => {
  const r = rec.value
  if (!r) return null
  // The inline GigFinanceReconciliation payload doesn't carry the gig's
  // current amount / currency / payment status — only the reconciliation
  // row token + decision metadata. Pull those details from the store when
  // they aren't part of the inline payload.
  const fallbackGig = props.gigId ? (store.reconciliationByGig[props.gigId] as ResolvedReconciliation | null) : null
  const amount = r.gig_amount_minor ?? fallbackGig?.gig_amount_minor ?? null
  const currency = r.gig_currency ?? fallbackGig?.gig_currency ?? 'EUR'
  const status = r.gig_payment_status ?? fallbackGig?.gig_payment_status ?? ''
  return {
    title: reasonCopy[r.reason as ReconciliationReason].title,
    body: reasonCopy[r.reason as ReconciliationReason].body,
    gigAmount: amount != null ? formatMinor(amount, currency) : '—',
    gigCurrency: currency,
    gigStatus: status,
    allowed: r.allowed_actions,
  }
})

async function resolve(action: ReconciliationAction): Promise<void> {
  const r = rec.value
  if (!r) return
  busy.value = true
  busyAction.value = action
  error.value = ''
  try {
    const updated = await store.resolveReconciliation(r.id, {
      action,
      updated_at: r.updated_at,
    } as never, props.gigId ?? undefined)
    emit('resolved', updated as EntryReconciliation)
    open.value = false
    if (props.gigId) store.clearReconciliationForGig(props.gigId)
  } catch (e) {
    error.value = (e as { message?: string })?.message ?? 'Could not resolve the reconciliation.'
  } finally {
    busy.value = false
    busyAction.value = null
  }
}

function dismiss(): void {
  open.value = false
  emit('dismissed')
}

const openManually = async (): Promise<void> => {
  if (!props.gigId) return
  await fetchAndOpen()
}
defineExpose({ openManually })
</script>

<template>
  <Dialog :open="open" @update:open="(v) => { open = v; if (!v) dismiss() }">
    <DialogContent
      v-if="open"
      class="max-w-[440px] w-[calc(100%-32px)] bg-surface-container"
      @open-auto-focus="onOpenAutoFocus"
      @close-auto-focus="onCloseAutoFocus"
    >
      <DialogTitle ref="titleEl" tabindex="-1" class="rec-title">
        {{ cfg?.title || 'FINANCE RECONCILIATION' }}
      </DialogTitle>
      <DialogDescription class="rec-body">
        <template v-if="cfg">
          <p>{{ cfg.body }}</p>
          <p class="rec-snapshot">
            Latest gig snapshot: <strong>{{ cfg.gigAmount }}</strong> · {{ cfg.gigCurrency }}
          </p>
        </template>
        <template v-else>Reconciliation data is unavailable right now.</template>
      </DialogDescription>

      <div v-if="cfg" class="rec-actions">
        <button
          v-for="a in cfg.allowed"
          :key="a"
          type="button"
          class="btn-hud"
          :class="styleFor(a)"
          :disabled="busy"
          @click="resolve(a)"
        >
          {{ busy && busyAction === a ? 'SAVING…' : labelFor(a) }}
        </button>
        <button type="button" class="btn-hud btn-hud-ghost rec-back" @click="dismiss">DECIDE LATER</button>
      </div>
      <p v-if="error" class="rec-error" role="alert">{{ error }}</p>
      <p v-else-if="reconciliationErrors[gigId ?? '']" class="rec-error" role="alert">
        {{ reconciliationErrors[gigId ?? '']?.message }}
      </p>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
.rec-title { font-size: 14px; outline: none; }
.rec-body { margin-top: 8px; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); display: flex; flex-direction: column; gap: 10px; }
.rec-snapshot { margin: 0; padding: 8px 12px; background: color-mix(in srgb, var(--color-primary) 8%, transparent); border-left: 3px solid var(--color-primary); font-family: var(--font-command); font-size: 12px; color: var(--color-on-surface); }
.rec-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; }
.rec-back { width: 100%; margin-top: 6px; }
.rec-error { margin: 12px 0 0; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .rec-actions { flex-direction: column; }
  .rec-actions .btn-hud { width: 100%; }
}
</style>
