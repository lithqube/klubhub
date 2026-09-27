<script setup lang="ts">
import type { ApiError } from '~/types/event'
import { confirmMatches, isStepUp, purgeErrorText } from '~/utils/privacy'

/**
 * ERASE NOW (P2.5): plain consequences, the event title typed to confirm,
 * and the refusals the API can give — a stale sign-in (step-up) links to
 * sign in again and come back here; missing two-factor links to account
 * security. The parent runs the erase and passes back its error.
 */
const props = defineProps<{ title: string, personalRows: number | null, busy: boolean, error: ApiError | null }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ confirm: [typed: string] }>()

const route = useRoute()
const uid = useId()
const typed = ref('')
const input = ref<HTMLInputElement | null>(null)
watch(open, (o) => {
  if (o) typed.value = ''
})

const matches = computed(() => confirmMatches(typed.value, props.title))
const errorText = computed(() => (props.error ? purgeErrorText(props.error) : ''))
const stepUp = computed(() => !!props.error && isStepUp(props.error.error))
const signInAgain = computed(() => ({ path: '/login', query: { next: route.fullPath } }))

function submit() {
  if (!matches.value || props.busy) {
    input.value?.focus()
    return
  }
  emit('confirm', typed.value.trim())
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="erase-dialog" @open-auto-focus.prevent="input?.focus()">
      <DialogTitle class="erase-title">ERASE GUEST DATA NOW?</DialogTitle>
      <DialogDescription class="erase-lead">
        {{ title }}: this can't be undone.
        <template v-if="personalRows">It erases the names and contacts of {{ personalRows }} {{ personalRows === 1 ? 'guest or ticket holder' : 'guests and ticket holders' }}.</template>
      </DialogDescription>

      <div class="erase-cols">
        <section :aria-labelledby="`${uid}-gone`">
          <h3 :id="`${uid}-gone`" class="erase-h erase-h-gone">ERASED FOR GOOD</h3>
          <ul>
            <li>Guest names, emails, phone numbers and notes</li>
            <li>Ticket holder and buyer names, emails and ticket barcodes</li>
            <li>Submitter contacts and this event's door PINs</li>
          </ul>
        </section>
        <section :aria-labelledby="`${uid}-kept`">
          <h3 :id="`${uid}-kept`" class="erase-h">KEPT</h3>
          <ul>
            <li>Counts, statuses and check-ins, shown as "Erased guest"</li>
            <li>The report: check-in curve, walk-ups, numbers by list and submitter</li>
          </ul>
        </section>
      </div>
      <p class="erase-after">Afterwards the guest export and list-backs stop working, and nobody can be added or imported for this event.</p>

      <form novalidate class="erase-form" @submit.prevent="submit">
        <label :for="`${uid}-confirm`" class="erase-label">Type the event title <strong>{{ title }}</strong> to confirm</label>
        <input
          :id="`${uid}-confirm`" ref="input" v-model="typed" class="hud-input" autocomplete="off" autocapitalize="off" spellcheck="false"
          :aria-invalid="!!error && !stepUp" :aria-describedby="errorText ? `${uid}-err` : undefined" data-testid="erase-confirm-input"
        >
        <p v-if="errorText" :id="`${uid}-err`" role="alert" class="erase-err" data-testid="erase-error">
          {{ errorText }}
          <NuxtLink v-if="stepUp" :to="signInAgain" class="erase-link">SIGN IN AGAIN →</NuxtLink>
          <NuxtLink v-else-if="error?.error === 'mfa_required'" to="/account/security" class="erase-link">ACCOUNT SECURITY →</NuxtLink>
        </p>
        <div class="erase-actions">
          <button type="button" class="btn-hud btn-hud-ghost erase-btn" @click="open = false">CANCEL</button>
          <button type="submit" class="btn-hud btn-hud-error erase-btn" :disabled="!matches || busy" :aria-busy="busy">
            {{ busy ? 'ERASING…' : 'ERASE NOW' }}
          </button>
        </div>
      </form>
    </DialogContent>
  </Dialog>
</template>

<style>
/* Unscoped: the dialog is teleported. Fits a 375 px phone and scrolls inside. */
.erase-dialog {
  width: calc(100vw - 32px);
  max-width: 560px;
  max-height: calc(100dvh - 32px);
  overflow-y: auto;
  display: grid;
  gap: 12px;
  padding: 20px;
}
/* The shared dialog's close button: a real 44 px target. */
.erase-dialog > button.absolute {
  top: 6px;
  right: 6px;
  width: 44px;
  height: 44px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.erase-title {
  margin: 0;
  padding-right: 40px;
  font-family: var(--font-command);
  font-size: 16px;
  letter-spacing: .06em;
  color: var(--color-error);
}
.erase-lead, .erase-after {
  margin: 0;
  font-size: 13px;
  color: var(--color-on-surface);
}
.erase-cols {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
}
@media (min-width: 560px) {
  .erase-cols { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
.erase-cols section {
  padding: 8px 10px;
  background: var(--color-surface-container);
}
.erase-cols ul {
  margin: 4px 0 0;
  padding-left: 16px;
  list-style: disc;
  font-size: 12px;
  color: var(--color-on-surface-variant);
}
.erase-h {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .07em;
  color: var(--color-tertiary);
}
.erase-h-gone { color: var(--color-error); }
.erase-form {
  display: grid;
  gap: 8px;
}
.erase-label {
  font-size: 13px;
  overflow-wrap: anywhere;
}
.erase-err {
  margin: 0;
  font-size: 13px;
  color: var(--color-error);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
}
.erase-link {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-primary);
}
.erase-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.erase-btn {
  min-height: 44px;
  font-size: 11px;
}
</style>
