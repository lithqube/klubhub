<script setup lang="ts">
// "Bill a gig": pick the gig, confirm the due date, CREATE DRAFT. INLINE in the
// Invoices section, above the list (no modal), like the earnings entry composer.
// Currency comes from the gig; customer, tax treatment and supply date are
// pre-filled by the API and edited afterwards in the detail sheet.
//
// The host (pages/finance.vue) mounts it while the invoice store says a
// composer is open and unmounts it on `close`, so every opening starts fresh.
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import type { Invoice } from '../../types/finance'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import { addDays, toApiTime } from '../../utils/invoiceDisplay'
import { buildGigChoices } from '../../utils/invoiceGigChoices'
import { fieldId, fieldMessageId, inlineErrorMessage, prefixError } from '../../utils/invoiceFields'
import { focusWhenReady } from '../../utils/dialogFocus'
import InvoiceGigPicker from './InvoiceGigPicker.vue'

const props = defineProps<{
  presetGigId?: string | null
  /** Bumped by the store each time a composer is (re)requested: pull focus back to the first field. */
  focusSeq?: number
}>()
const emit = defineEmits<{ close: []; created: [inv: Invoice] }>()

const store = useInvoiceStore()
const { gigOptions, gigsLoading, gigsError, activeInvoiceByGig, listLoaded } = storeToRefs(store)

const root = ref<HTMLElement | null>(null)
const gigId = ref<string | null>(props.presetGigId ?? null)
const dueDate = ref('')
const prefix = ref('INV')
const showAdvanced = ref(false)
const creating = ref(false)
const error = ref('')

const choices = computed(() => buildGigChoices(gigOptions.value, activeInvoiceByGig.value))
const chosen = computed(() => choices.value.find((c) => c.id === gigId.value) ?? null)

function focusFirst(): void {
  focusWhenReady(computed(() => root.value?.querySelector<HTMLElement>('#inv-create-gig') ?? null))
}

onMounted(() => {
  dueDate.value = chosen.value?.date ? addDays(chosen.value.date, 14) : ''
  void store.fetchGigOptions()
  if (!listLoaded.value) void store.fetchInvoices()
  focusFirst()
})

// Asking again for another gig (a different "bill this gig" entry point)
// re-aims the open composer instead of silently keeping the old choice.
watch(() => props.presetGigId, (id) => { if (id) gigId.value = id })
watch(() => props.focusSeq, focusFirst)

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
    emit('close')
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

function onEscape(e: KeyboardEvent): void {
  // Escape inside the gig list closes that list first (the picker handles it).
  if (e.defaultPrevented) return
  e.stopPropagation()
  emit('close')
}
</script>

<template>
  <section ref="root" class="icf" aria-labelledby="icf-title" @keydown.esc="onEscape">
    <header class="icf-head">
      <h3 id="icf-title" class="icf-title">NEW INVOICE</h3>
      <p class="icf-desc">Creates a DRAFT. Nothing is sent to the promoter.</p>
    </header>

    <form class="icf-form" novalidate @submit.prevent="create">
      <div class="icf-grid">
        <div class="icf-wide">
          <label for="inv-create-gig" class="input-label icf-label">GIG *</label>
          <InvoiceGigPicker
            v-model="gigId"
            input-id="inv-create-gig"
            :choices="choices"
            described-by="inv-create-gig-msg"
            :invalid="!!gigId && !!gigProblem && !presetOnly"
          />
          <p id="inv-create-gig-msg" class="icf-msg" :class="{ 'icf-error': !!gigId && !!gigProblem && !presetOnly }" aria-live="polite">
            <template v-if="gigsLoading && !gigOptions.length">Loading gigs…</template>
            <template v-else-if="gigsError">
              {{ gigsError }}
              <button type="button" class="icf-link" @click="store.fetchGigOptions(true)">RETRY</button>
            </template>
            <template v-else-if="presetOnly">Billing the gig you came from.</template>
            <template v-else>{{ gigId ? gigProblem : 'Confirmed, advanced and played gigs are listed first.' }}</template>
          </p>
        </div>

        <div class="icf-f icf-f-cur">
          <span class="input-label icf-label">CURRENCY</span>
          <p class="icf-locked" aria-describedby="inv-create-cur-msg">{{ chosen?.currency ?? '—' }}</p>
          <p id="inv-create-cur-msg" class="icf-msg">Locked to the gig fee currency.</p>
        </div>
        <div class="icf-f icf-f-fee">
          <span class="input-label icf-label">FEE</span>
          <p class="icf-locked">{{ chosen?.feeLabel ?? '—' }}</p>
        </div>
        <div class="icf-f icf-f-due">
          <label :for="fieldId('create.due_at')" class="input-label icf-label">DUE DATE</label>
          <input :id="fieldId('create.due_at')" v-model="dueDate" type="date" class="hud-input">
          <p class="icf-msg">Defaults to 14 days after the gig.</p>
        </div>
      </div>

      <div>
        <button
          type="button"
          class="icf-more"
          :aria-expanded="showAdvanced"
          aria-controls="inv-create-advanced"
          @click="showAdvanced = !showAdvanced"
        >
          <span aria-hidden="true">{{ showAdvanced ? '−' : '+' }}</span> ADVANCED
        </button>
        <div v-show="showAdvanced" id="inv-create-advanced" style="margin-top:8px;">
          <label :for="fieldId('create.number_prefix')" class="input-label icf-label">NUMBER PREFIX</label>
          <input
            :id="fieldId('create.number_prefix')"
            v-model="prefix"
            class="hud-input icf-prefix"
            maxlength="20"
            autocomplete="off"
            :aria-invalid="prefixProblem ? 'true' : undefined"
            :aria-describedby="fieldMessageId('create.number_prefix')"
          >
          <p :id="fieldMessageId('create.number_prefix')" class="icf-msg" :class="{ 'icf-error': !!prefixProblem }">
            {{ prefixProblem || `The number is assigned at issue, e.g. ${(prefix || 'INV').toUpperCase()}-0001-${chosen?.currency ?? 'EUR'}.` }}
          </p>
        </div>
      </div>

      <p v-if="error" class="icf-msg icf-error" role="alert">{{ error }}</p>

      <div class="icf-actions">
        <button type="button" class="btn-hud btn-hud-ghost" @click="emit('close')">CANCEL</button>
        <button type="submit" class="btn-hud btn-hud-cta" :disabled="!canCreate">
          {{ creating ? 'CREATING…' : 'CREATE DRAFT' }}
        </button>
      </div>
    </form>
  </section>
</template>

<style scoped>
/* Flat plane + dashed line (DESIGN.md), the same as the entry composer: a form, not floating chrome. */
.icf { display: flex; flex-direction: column; min-width: 0; margin-bottom: 8px; padding: 14px 16px 16px; background: var(--color-surface-container-low); border: 1px dashed color-mix(in srgb, var(--color-on-surface) 22%, transparent); }
.icf-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 12px; }
.icf-title { margin: 0; font-family: var(--font-command); font-size: 14px; font-weight: 700; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); outline: none; }
.icf-desc { margin: 0; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.icf-form { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
/* 12-column grid: gig on its own row, then currency, fee and due date share one. */
.icf-grid { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 8px 12px; }
.icf-wide { grid-column: 1 / -1; min-width: 0; }
.icf-f { min-width: 0; }
.icf-f-cur { grid-column: span 3; }
.icf-f-fee { grid-column: span 4; }
.icf-f-due { grid-column: span 5; }
.icf-label { display: block; margin-bottom: 4px; }
.icf-locked { margin: 0; min-height: 40px; display: flex; align-items: center; padding: 0 12px; background: var(--color-surface-container); font-family: var(--font-command); font-size: 13px; font-weight: 700; letter-spacing: -.02em; color: var(--color-on-surface); }
.icf-msg { margin: 3px 0 0; font-family: var(--font-data); font-size: 11px; line-height: 1.4; color: var(--color-tertiary); }
.icf-msg:empty { display: none; }
.icf-error { color: var(--color-error); }
.icf-link { margin-left: 6px; padding: 0; background: none; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-primary); text-decoration: underline; }
.icf-more { display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 0; background: transparent; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.icf-more:hover, .icf-more:focus-visible { color: var(--color-primary); outline: none; }
.icf-prefix { max-width: 240px; }
.icf-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; padding-top: 14px; border-top: 1px dashed color-mix(in srgb, var(--color-on-surface) 14%, transparent); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 900px) {
  .icf-f-cur { grid-column: span 5; }
  .icf-f-fee { grid-column: span 7; }
  .icf-f-due { grid-column: 1 / -1; }
}
@media (max-width: 768px) {
  .icf-more, .hud-input, .icf-link { min-height: 44px; }
  .icf-actions { flex-direction: column-reverse; }
  .icf-actions .btn-hud { width: 100%; }
}
</style>
