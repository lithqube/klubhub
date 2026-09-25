<script setup lang="ts">
// "Bill a gig": pick the gig, confirm the due date, CREATE DRAFT. Currency
// comes from the gig; customer, tax treatment and supply date are pre-filled
// by the API and edited afterwards in the detail sheet.
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import type { Invoice } from '../../types/finance'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import { addDays, toApiTime } from '../../utils/invoiceDisplay'
import { buildGigChoices } from '../../utils/invoiceGigChoices'
import { fieldId, fieldMessageId, inlineErrorMessage, prefixError } from '../../utils/invoiceFields'
import { useDialogFocus } from '../../utils/dialogFocus'
import Dialog from '../ui/dialog/Dialog.vue'
import DialogContent from '../ui/dialog/DialogContent.vue'
import DialogTitle from '../ui/dialog/DialogTitle.vue'
import DialogDescription from '../ui/dialog/DialogDescription.vue'
import InvoiceGigPicker from './InvoiceGigPicker.vue'

const props = defineProps<{ open: boolean; presetGigId?: string | null }>()
const emit = defineEmits<{ 'update:open': [open: boolean]; created: [inv: Invoice] }>()

const store = useInvoiceStore()
const { gigOptions, gigsLoading, gigsError, activeInvoiceByGig, listLoaded } = storeToRefs(store)

const gigId = ref<string | null>(null)
const dueDate = ref('')
const prefix = ref('INV')
const showAdvanced = ref(false)
const creating = ref(false)
const error = ref('')
const titleEl = ref<InstanceType<typeof DialogTitle> | null>(null)

const choices = computed(() => buildGigChoices(gigOptions.value, activeInvoiceByGig.value))
const chosen = computed(() => choices.value.find((c) => c.id === gigId.value) ?? null)

function reset(): void {
  gigId.value = props.presetGigId ?? null
  prefix.value = 'INV'
  showAdvanced.value = false
  error.value = ''
  dueDate.value = chosen.value?.date ? addDays(chosen.value.date, 14) : ''
}

watch(() => props.open, (o) => {
  if (!o) return
  reset()
  void store.fetchGigOptions()
  if (!listLoaded.value) void store.fetchInvoices()
}, { immediate: true })

// Due date follows the chosen gig (gig date + 14 days) until edited.
watch(chosen, (c) => { if (c?.date) dueDate.value = addDays(c.date, 14) })

const gigProblem = computed(() => {
  if (!gigId.value) return 'Choose the gig to bill.'
  if (chosen.value?.disabledReason) return `${chosen.value.disabledReason}.`
  return ''
})
const prefixProblem = computed(() => prefixError(prefix.value))
/** A preset gig we could not load details for can still be billed; the API checks it. */
const presetOnly = computed(() => !!gigId.value && !chosen.value && gigId.value === props.presetGigId)
const canCreate = computed(() =>
  !creating.value && !!gigId.value && (presetOnly.value || !chosen.value?.disabledReason) && !prefixProblem.value,
)

const { onOpenAutoFocus, onCloseAutoFocus } = useDialogFocus(() => props.open, titleEl)

async function create(): Promise<void> {
  if (!canCreate.value || !gigId.value) return
  creating.value = true
  error.value = ''
  try {
    const inv = await store.createInvoice({
      gig_id: gigId.value,
      due_at: toApiTime(dueDate.value) ?? undefined,
      number_prefix: prefix.value.trim().toUpperCase(),
    })
    emit('created', inv)
    emit('update:open', false)
  } catch (e) {
    const err = toFinanceError(e)
    if (err.code === 'bad_state') {
      error.value = 'This gig already has an active invoice. The list is refreshed; open it from there.'
      void store.fetchInvoices()
    } else if (err.code === 'validation_failed' && err.field === 'number_prefix') {
      showAdvanced.value = true
      error.value = err.message
    } else {
      error.value = inlineErrorMessage(err) ?? err.message
    }
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-w-[480px] w-[calc(100%-32px)] bg-surface-container max-h-[90vh] overflow-y-auto" @open-auto-focus="onOpenAutoFocus" @close-auto-focus="onCloseAutoFocus">
      <DialogTitle ref="titleEl" tabindex="-1" class="icr-title">NEW INVOICE</DialogTitle>
      <DialogDescription class="icr-desc">Creates a DRAFT. Nothing is sent to the promoter.</DialogDescription>

      <form class="icr-form" novalidate @submit.prevent="create">
        <div>
          <label for="inv-create-gig" class="input-label icr-label">GIG *</label>
          <InvoiceGigPicker
            v-model="gigId"
            input-id="inv-create-gig"
            :choices="choices"
            described-by="inv-create-gig-msg"
            :invalid="!!gigId && !!gigProblem && !presetOnly"
          />
          <p id="inv-create-gig-msg" class="icr-msg" :class="{ 'icr-error': !!gigId && !!gigProblem && !presetOnly }" aria-live="polite">
            <template v-if="gigsLoading && !gigOptions.length">Loading gigs…</template>
            <template v-else-if="gigsError">
              {{ gigsError }}
              <button type="button" class="icr-link" @click="store.fetchGigOptions(true)">RETRY</button>
            </template>
            <template v-else-if="presetOnly">Billing the gig you came from.</template>
            <template v-else>{{ gigId ? gigProblem : 'Confirmed, advanced and played gigs are listed first.' }}</template>
          </p>
        </div>

        <div class="icr-grid">
          <div>
            <span class="input-label icr-label">CURRENCY</span>
            <p class="icr-locked" :aria-describedby="'inv-create-cur-msg'">{{ chosen?.currency ?? '—' }}</p>
            <p id="inv-create-cur-msg" class="icr-msg">Locked to the gig fee currency.</p>
          </div>
          <div>
            <span class="input-label icr-label">FEE</span>
            <p class="icr-locked">{{ chosen?.feeLabel ?? '—' }}</p>
          </div>
          <div class="icr-wide">
            <label :for="fieldId('create.due_at')" class="input-label icr-label">DUE DATE</label>
            <input :id="fieldId('create.due_at')" v-model="dueDate" type="date" class="hud-input">
            <p class="icr-msg">Defaults to 14 days after the gig.</p>
          </div>
        </div>

        <div>
          <button
            type="button"
            class="icr-more"
            :aria-expanded="showAdvanced"
            aria-controls="inv-create-advanced"
            @click="showAdvanced = !showAdvanced"
          >
            <span aria-hidden="true">{{ showAdvanced ? '−' : '+' }}</span> ADVANCED
          </button>
          <div v-show="showAdvanced" id="inv-create-advanced" style="margin-top:8px;">
            <label :for="fieldId('create.number_prefix')" class="input-label icr-label">NUMBER PREFIX</label>
            <input
              :id="fieldId('create.number_prefix')"
              v-model="prefix"
              class="hud-input"
              maxlength="20"
              autocomplete="off"
              :aria-invalid="prefixProblem ? 'true' : undefined"
              :aria-describedby="fieldMessageId('create.number_prefix')"
            >
            <p :id="fieldMessageId('create.number_prefix')" class="icr-msg" :class="{ 'icr-error': !!prefixProblem }">
              {{ prefixProblem || `The number is assigned at issue, e.g. ${(prefix || 'INV').toUpperCase()}-0001-${chosen?.currency ?? 'EUR'}.` }}
            </p>
          </div>
        </div>

        <p v-if="error" class="icr-msg icr-error" role="alert">{{ error }}</p>

        <div class="icr-actions">
          <button type="button" class="btn-hud btn-hud-ghost" @click="emit('update:open', false)">CANCEL</button>
          <button type="submit" class="btn-hud btn-hud-cta" :disabled="!canCreate">
            {{ creating ? 'CREATING…' : 'CREATE DRAFT' }}
          </button>
        </div>
      </form>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
.icr-title { font-size: 14px; outline: none; }
.icr-desc { margin-top: 6px; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.icr-form { display: flex; flex-direction: column; gap: 14px; margin-top: 16px; }
.icr-label { display: block; }
.icr-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 12px; }
.icr-wide { grid-column: 1 / -1; }
.icr-locked { margin: 0; min-height: 40px; display: flex; align-items: center; padding: 0 12px; background: var(--color-surface-container-low); font-family: var(--font-command); font-size: 13px; font-weight: 700; letter-spacing: -.02em; color: var(--color-on-surface); }
.icr-msg { margin: 4px 0 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.icr-msg:empty { display: none; }
.icr-error { color: var(--color-error); }
.icr-link { margin-left: 6px; padding: 0; background: none; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-primary); text-decoration: underline; }
.icr-more { display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 0; background: transparent; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.icr-more:hover, .icr-more:focus-visible { color: var(--color-primary); outline: none; }
.icr-actions { display: flex; justify-content: flex-end; gap: 8px; }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .icr-grid { grid-template-columns: 1fr; }
  .icr-more, .hud-input, .icr-link { min-height: 44px; }
  .icr-actions { flex-direction: column-reverse; }
  .icr-actions .btn-hud { width: 100%; }
}
</style>
