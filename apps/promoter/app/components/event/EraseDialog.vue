<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useSessionStore } from '~/stores/session'
import type { ApiError } from '~/types/event'
import { EVENT_GRACE_HOURS } from '~/utils/doorState'
import { authIsStale, confirmMatches, ERASED_ITEMS, isStepUp, KEPT_ITEMS, purgeErrorText, signInAgainRoute } from '~/utils/privacy'

/**
 * ERASE NOW (P2.5): plain consequences, the event title typed to confirm
 * (case, dashes, quotes and spacing do not matter) and the refusals the
 * API can give. Erasing needs a sign-in in the last 15 minutes: when the
 * session is older the dialog starts with SIGN IN AGAIN, which comes back
 * here (?erase=1) with the dialog open. The parent runs the erase.
 *
 * purgeOn: when the schedule would erase it anyway (formatted); null once
 * it is already due. endsAt: a night that ended less than
 * EVENT_GRACE_HOURS ago may still have door devices with unsynced adds.
 */
const props = defineProps<{
  title: string
  personalRows: number | null
  busy: boolean
  error: ApiError | null
  purgeOn: string | null
  endsAt: string
}>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ confirm: [typed: string] }>()

const route = useRoute()
const { me } = storeToRefs(useSessionStore())
const uid = useId()
const typed = ref('')
const input = ref<HTMLInputElement | null>(null)
const signInLink = ref<{ $el?: HTMLElement } | null>(null)
/** Checked when the dialog opens, so it never switches step while typing. */
const needsSignIn = ref(false)
const recentlyEnded = ref(false)
watch(open, (o) => {
  if (!o) return
  typed.value = ''
  const now = Date.now()
  needsSignIn.value = authIsStale(me.value?.auth_time, now)
  recentlyEnded.value = now - Date.parse(props.endsAt) < EVENT_GRACE_HOURS * 3_600_000
}, { immediate: true })

const matches = computed(() => confirmMatches(typed.value, props.title))
const errorText = computed(() => (props.error ? purgeErrorText(props.error) : ''))
const stepUp = computed(() => needsSignIn.value || (!!props.error && isStepUp(props.error.error)))
const signInAgain = computed(() => signInAgainRoute(`${route.path}?erase=1`, 'erase'))
const describedBy = computed(() => [typed.value ? `${uid}-hint` : '', errorText.value ? `${uid}-err` : ''].filter(Boolean).join(' ') || undefined)

function focusFirst() {
  if (needsSignIn.value) signInLink.value?.$el?.focus?.()
  else input.value?.focus()
}

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
    <DialogContent class="erase-dialog" @open-auto-focus.prevent="focusFirst">
      <DialogTitle class="erase-title">ERASE GUEST DATA NOW?</DialogTitle>
      <DialogDescription class="erase-lead">
        {{ title }}: this can't be undone.
        <template v-if="personalRows">It erases the names and contacts of {{ personalRows }} {{ personalRows === 1 ? 'guest or ticket holder' : 'guests and ticket holders' }}.</template>
        <template v-else-if="personalRows === 0">No guest or ticket holder names are left; it clears any submitter contacts and door PINs, and nobody can be added afterwards.</template>
      </DialogDescription>

      <template v-if="needsSignIn">
        <p class="erase-lead" data-testid="erase-sign-in-first">
          For safety, erasing needs a recent sign-in. Sign in again and you'll come straight back here.
        </p>
        <div class="erase-actions">
          <button type="button" class="btn-hud btn-hud-ghost erase-btn" @click="open = false">CANCEL</button>
          <NuxtLink ref="signInLink" :to="signInAgain" class="btn-hud btn-hud-cta erase-btn" data-testid="erase-sign-in-again">SIGN IN AGAIN</NuxtLink>
        </div>
      </template>

      <template v-else>
        <p class="erase-lead" data-testid="erase-when">
          <template v-if="purgeOn">This happens automatically on {{ purgeOn }}. Erasing now only brings it forward.</template>
          <template v-else>This is already due and happens automatically within the hour. Erasing now only brings it forward.</template>
        </p>
        <p v-if="recentlyEnded" class="erase-warn" data-testid="erase-door-warning">
          This night ended less than {{ EVENT_GRACE_HOURS }} hours ago. Door devices that haven't synced yet lose their walk-ups and door adds.
        </p>

        <div class="erase-cols">
          <section :aria-labelledby="`${uid}-gone`">
            <h3 :id="`${uid}-gone`" class="erase-h erase-h-gone">ERASED FOR GOOD</h3>
            <ul>
              <li v-for="item in ERASED_ITEMS" :key="item">{{ item }}</li>
            </ul>
          </section>
          <section :aria-labelledby="`${uid}-kept`">
            <h3 :id="`${uid}-kept`" class="erase-h">KEPT</h3>
            <ul>
              <li v-for="item in KEPT_ITEMS" :key="item">{{ item }}</li>
            </ul>
          </section>
        </div>
        <p class="erase-after">Afterwards the guest export and list-backs stop working, and nobody can be added or imported for this event.</p>

        <form novalidate class="erase-form" @submit.prevent="submit">
          <label :for="`${uid}-confirm`" class="erase-label">Type the event title <strong>{{ title }}</strong> to confirm</label>
          <input
            :id="`${uid}-confirm`" ref="input" v-model="typed" class="hud-input" autocomplete="off" autocapitalize="off" spellcheck="false"
            :aria-invalid="!!typed && !matches" :aria-describedby="describedBy" data-testid="erase-confirm-input"
          >
          <p v-if="typed" :id="`${uid}-hint`" class="erase-hint" :class="{ 'erase-hint-ok': matches }" aria-live="polite" data-testid="erase-match">
            {{ matches ? 'Title matches.' : 'Not matching yet — check spelling and spaces.' }}
          </p>
          <p v-if="errorText" :id="`${uid}-err`" role="alert" class="erase-err" data-testid="erase-error">
            {{ errorText }}
            <NuxtLink v-if="stepUp" :to="signInAgain" class="erase-link">SIGN IN AGAIN →</NuxtLink>
          </p>
          <div class="erase-actions">
            <button type="button" class="btn-hud btn-hud-ghost erase-btn" @click="open = false">CANCEL</button>
            <button type="submit" class="btn-hud btn-hud-error erase-btn" :disabled="!matches || busy" :aria-busy="busy">
              {{ busy ? 'ERASING…' : 'ERASE NOW' }}
            </button>
          </div>
        </form>
      </template>
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
.erase-warn {
  margin: 0;
  padding: 8px 10px;
  font-size: 13px;
  border-left: 3px solid var(--color-status-archived);
  background: var(--color-surface-container);
}
.erase-hint {
  margin: 0;
  font-size: 12px;
  color: var(--color-on-surface-variant);
}
.erase-hint-ok { color: var(--color-primary); }
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
  display: inline-flex;
  align-items: center;
}
</style>
