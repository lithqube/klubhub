<script setup lang="ts">
// What still blocks issuing a draft (GET /invoices/{id}/issue-check, or the
// problems of a 422 not_issuable). Each fixable item moves focus to its
// field; ISSUE stays disabled until the server says the draft is ready.
import { computed } from 'vue'
import type { IssueCheck } from '../../types/finance'
import { problemTarget } from '../../utils/invoiceFields'

const props = withDefaults(defineProps<{
  check: IssueCheck | null
  loading?: boolean
  /** Unsaved edits: the check describes the saved draft, not the form. */
  dirty?: boolean
  busy?: boolean
}>(), { loading: false, dirty: false, busy: false })

const emit = defineEmits<{ focus: [field: string]; issue: [] }>()

const problems = computed(() => props.check?.problems ?? [])
const ready = computed(() => !!props.check?.ready && problems.value.length === 0)
const canIssue = computed(() => ready.value && !props.dirty && !props.busy && !props.loading)

const blockedReason = computed(() => {
  if (props.loading && !props.check) return 'Checking the draft…'
  if (props.dirty) return 'Save your changes before issuing.'
  if (!props.check) return 'Could not check this draft. Save it to check again.'
  if (!ready.value) return `${problems.value.length} item${problems.value.length === 1 ? '' : 's'} to fix before issuing.`
  return ''
})

const HINTS = {
  billing_profile: 'Set in your billing profile.',
  gig: 'Comes from the gig.',
} as const
</script>

<template>
  <section class="chk" aria-labelledby="inv-chk-title">
    <div class="chk-head">
      <h3 id="inv-chk-title" class="section-lbl chk-title">ISSUE CHECK</h3>
      <span class="badge-hud" :class="ready ? 'badge-ready' : 'badge-draft'">
        {{ ready ? 'READY' : 'NOT READY' }}
      </span>
    </div>

    <ul v-if="problems.length" class="chk-list">
      <li v-for="(p, i) in problems" :key="`${p.field}-${i}`" class="chk-item">
        <button
          v-if="problemTarget(p.field) === 'field'"
          type="button"
          class="chk-fix"
          @click="emit('focus', p.field)"
        >
          <span aria-hidden="true" class="chk-mark">!</span>
          <span>{{ p.message }}</span>
          <span class="chk-go">FIX</span>
        </button>
        <p v-else class="chk-fix chk-static">
          <span aria-hidden="true" class="chk-mark">!</span>
          <span>{{ p.message }} <span class="chk-hint">{{ HINTS[problemTarget(p.field) as 'billing_profile' | 'gig'] }}</span></span>
        </p>
      </li>
    </ul>

    <p class="chk-status" aria-live="polite">{{ blockedReason }}</p>

    <button
      type="button"
      class="btn-hud btn-hud-cta chk-issue"
      :disabled="!canIssue"
      :aria-describedby="blockedReason ? undefined : 'inv-chk-copy'"
      @click="emit('issue')"
    >
      {{ busy ? 'ISSUING…' : 'ISSUE INVOICE' }}
    </button>
    <p id="inv-chk-copy" class="chk-copy">
      Issuing locks the amount, customer and your billing details and assigns the invoice number.
    </p>
  </section>
</template>

<style scoped>
.chk { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px; background: var(--color-surface-container-low); border: 1px dashed color-mix(in srgb, var(--color-primary) 20%, transparent); }
.chk-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.chk-title { margin: 0; font-weight: 600; }
.chk-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
.chk-fix { display: flex; width: 100%; align-items: flex-start; gap: 8px; padding: 6px 8px; margin: 0; background: transparent; border: 0; text-align: left; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
button.chk-fix { cursor: pointer; }
button.chk-fix:hover, button.chk-fix:focus-visible { background: color-mix(in srgb, var(--color-primary) 6%, transparent); outline: none; }
button.chk-fix:focus-visible { box-shadow: inset 2px 0 0 var(--color-primary); }
.chk-mark { flex-shrink: 0; width: 14px; height: 14px; display: inline-flex; align-items: center; justify-content: center; background: var(--color-error); color: var(--color-on-error); font-family: var(--font-terminal); font-size: 9px; font-weight: 700; }
.chk-go { margin-left: auto; font-family: var(--font-terminal); font-size: 8px; letter-spacing: .06em; color: var(--color-primary); }
.chk-hint { color: var(--color-on-surface-variant); }
.chk-status { margin: 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.chk-status:empty { display: none; }
.chk-issue { align-self: flex-start; }
.chk-issue:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
.chk-copy { margin: 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
@media (max-width: 768px) {
  .chk-fix { min-height: 44px; align-items: center; }
  .chk-issue { align-self: stretch; }
}
</style>
