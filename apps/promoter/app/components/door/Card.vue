<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { AlertTriangle, ArrowLeft, Ban, Check, EyeOff, Minus, Plus, ShieldAlert, UserX } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import type { ManagerPinVerifier } from '~/types/door'
import type { DoorBanEntry } from '~/utils/doorBan'
import type { SubjectView } from '~/utils/doorState'
import { verifyManagerPin } from '~/utils/doorPin'
import { shortDate } from '~/utils/privacy'
import { IMPORT_PRESETS } from '~/utils/attendeeImport'
import { timeLabel } from '~/utils/datetime'
import { lockSecondsLeft, REVEAL_HIDE_MS, revealLocked, triesLeft } from '~/utils/doorReveal'
import { admitReasons, confirmAllowed, deviceTag } from '~/utils/doorState'

/**
 * One guest or ticket at the door: list and entry terms, perks, heads in
 * and remaining, the +N stepper for partial arrivals, and ADMIT. Anything
 * unusual (already in, past the list cutoff, not marked going, a double
 * entry from another device) is shown as a full-width amber block and
 * arms the button first: a second tap admits anyway — but not a double tap
 * (a confirming tap within 700 ms of arming is ignored). Cancelled and
 * refunded tickets cannot be admitted.
 *
 * `admit` says whether it came from the keyboard, so the page only moves
 * focus back to the search field for keyboard users (a touch refocus pops
 * the phone keyboard over the next card).
 *
 * Ban list (P2.6): a possible match shows a quiet amber "MANAGER CHECK" —
 * never the words "ban list", since the guest may be looking at the
 * screen (staff are briefed on the event's DOOR tab). The reason is
 * revealed only with the manager PIN (checked offline against the bundle's
 * verifier) and ADMIT reads ASK A MANAGER until then. After the PIN the
 * manager decides deliberately: TURN AWAY (back to search), or ADMIT with a
 * second tap; HIDE REASON (or 30 s) takes the reason off the screen. Five
 * wrong PINs disable SHOW REASON on this device for a minute. Without a
 * manager PIN for the night, the match only arms a second tap.
 */
const props = withDefaults(defineProps<{
  view: SubjectView
  timezone: string
  selfDevice: string
  ban?: DoorBanEntry[]
  managerPin?: ManagerPinVerifier | null
}>(), { ban: () => [], managerPin: null })
const emit = defineEmits<{ admit: [count: number, keyboard: boolean], back: [] }>()
const door = useDoorStore()
const { revealGuard } = storeToRefs(door)

const v = computed(() => props.view)
const maxCount = computed(() => (v.value.remaining > 0 ? v.value.remaining : v.value.allowance))
const count = ref(1)
const armedAt = ref<number | null>(null)
const armed = computed(() => armedAt.value !== null)
const heading = ref<HTMLElement | null>(null)
watch(() => [v.value.key, v.value.remaining] as const, () => {
  count.value = Math.max(1, maxCount.value)
  armedAt.value = null
}, { immediate: true })

// ---------------------------------------------------------------- ban list
/** The reason is on screen (hidden again by HIDE REASON or after 30 s). */
const revealed = ref(false)
/** A manager typed the right PIN for this card: ADMIT no longer waits (hiding the reason keeps that). */
const approved = ref(false)
const pin = ref('')
const pinError = ref('')
const checking = ref(false)
const pinInput = ref<HTMLInputElement | null>(null)
const hideBtn = ref<HTMLButtonElement | null>(null)
const clock = useNow({ interval: 1000 })
let hideTimer: ReturnType<typeof setTimeout> | undefined
function clearHide() {
  clearTimeout(hideTimer)
  hideTimer = undefined
}
function hideReason() {
  clearHide()
  revealed.value = false
}
watch(() => v.value.key, () => {
  clearHide()
  revealed.value = false
  approved.value = false
  pin.value = ''
  pinError.value = ''
})
onBeforeUnmount(clearHide)
const banned = computed(() => props.ban.length > 0)
/** ADMIT waits for the manager PIN while a possible match is not yet approved. */
const banGate = computed(() => banned.value && !!props.managerPin && !approved.value)
const locked = computed(() => revealLocked(revealGuard.value, clock.value.getTime()))
const lockLeft = computed(() => lockSecondsLeft(revealGuard.value, clock.value.getTime()))

async function reveal() {
  if (locked.value) return
  pinError.value = ''
  checking.value = true
  const ok = await verifyManagerPin(pin.value.trim(), props.managerPin)
  checking.value = false
  pin.value = ''
  door.revealAttempt(ok)
  if (!ok) {
    pinError.value = revealLocked(revealGuard.value, Date.now())
      ? 'Wrong manager PIN. Too many tries: SHOW REASON is off for a minute.'
      : `Wrong manager PIN. ${triesLeft(revealGuard.value)} ${triesLeft(revealGuard.value) === 1 ? 'try' : 'tries'} left before a 1-minute pause.`
    nextTick(() => pinInput.value?.focus())
    return
  }
  approved.value = true
  revealed.value = true
  clearHide()
  hideTimer = setTimeout(() => {
    hideTimer = undefined
    revealed.value = false
  }, REVEAL_HIDE_MS)
  nextTick(() => hideBtn.value?.focus())
}
const until = (iso: string) => shortDate(iso, props.timezone)

// A new card takes focus on its name, so screen readers start there.
watch(() => v.value.key, () => nextTick(() => heading.value?.focus({ preventScroll: false })))
onMounted(() => heading.value?.focus())

const reasons = computed(() => {
  const r: { code: string, text: string }[] = admitReasons(v.value)
  // A possible match is always one more thing to confirm deliberately:
  // without a manager PIN tonight, and after the manager saw the reason.
  if (banned.value && !banGate.value) r.unshift({ code: 'ban', text: 'MANAGER CHECK' })
  return r
})
const needsSecondTap = computed(() => reasons.value.length > 0)
/** The amber block lists the other reasons; the match has its own block above. */
const otherReasons = computed(() => reasons.value.filter(r => r.code !== 'ban'))
const primary = computed(() => {
  const r = otherReasons.value[0]
  if (!r) return ''
  const last = v.value.lastIn
  if (r.code === 'already_in' && last) return `ALREADY IN · ${timeLabel(last.at, props.timezone)} · ${deviceTag(last.device_id, props.selfDevice)}`
  return r.text
})
const more = computed(() => reasons.value.length - 1)
const moreOthers = computed(() => otherReasons.value.length - 1)
/** The first tap's label: the match reads ASK A MANAGER until a manager has approved it. */
const checkText = computed(() => {
  const r = reasons.value[0]
  if (!r) return ''
  if (r.code === 'ban') return approved.value ? 'MANAGER DECIDED? TAP TO ADMIT' : 'ASK A MANAGER'
  return `CHECK: ${r.text}`
})

const terms = computed(() => v.value.list?.entry_terms ?? null)
const priceText = computed(() => {
  if (!terms.value) return ''
  return terms.value.price_mode === 'reduced' ? terms.value.reduced_price_text || 'REDUCED' : 'FREE ENTRY'
})
const sourceLabel = (s: string) => IMPORT_PRESETS.find(p => p.id === s)?.label ?? s.toUpperCase()

function step(d: 1 | -1) {
  count.value += d
  armedAt.value = null
}

function admit(e: MouseEvent) {
  if (v.value.blocked || banGate.value) return
  if (needsSecondTap.value) {
    if (armedAt.value === null) {
      armedAt.value = Date.now()
      return
    }
    if (!confirmAllowed(armedAt.value, Date.now())) return
  }
  armedAt.value = null
  emit('admit', count.value, e.detail === 0)
}
</script>

<template>
  <article class="card glass" :class="v.blocked ? 'accent-bar-failed' : needsSecondTap || banned ? 'accent-bar-archived' : 'accent-bar-ready'" :aria-labelledby="`card-${v.key}`">
    <button type="button" class="btn-hud btn-hud-ghost back" @click="emit('back')">
      <ArrowLeft style="width:16px;height:16px;" aria-hidden="true" /> BACK TO SEARCH
    </button>

    <h2 :id="`card-${v.key}`" ref="heading" class="name" tabindex="-1">{{ v.name }}</h2>
    <p class="meta">
      <template v-if="v.guest">
        GUEST · {{ v.list?.name ?? 'LIST' }}<template v-if="v.guest.plus_n"> · +{{ v.guest.plus_n }}</template>
      </template>
      <template v-else-if="v.ticket">
        TICKET · {{ v.ticket.ticket_type || 'TICKET' }} · {{ sourceLabel(v.ticket.source) }} · order {{ v.ticket.order_ref }}
      </template>
    </p>

    <!-- No live region: the card's heading takes focus, and screen reader users read on from there. -->
    <section v-if="banned" class="banblock" :aria-labelledby="`ban-${v.key}`" data-testid="door-ban">
      <p :id="`ban-${v.key}`" class="bantitle">
        <ShieldAlert style="width:13px;height:13px;flex-shrink:0;" aria-hidden="true" /> MANAGER CHECK
      </p>
      <template v-if="!revealed">
        <p v-if="approved" class="bantext" data-testid="door-ban-hidden">Reason hidden. The manager PIN shows it again.</p>
        <form v-if="managerPin" class="reveal" novalidate @submit.prevent="reveal">
          <label :for="`ban-pin-${v.key}`" class="section-lbl">MANAGER PIN TO SEE THE REASON</label>
          <div class="reveal-row">
            <input
              :id="`ban-pin-${v.key}`" ref="pinInput" v-model="pin" class="hud-input pin" type="password" inputmode="numeric" pattern="[0-9]*"
              maxlength="6" autocomplete="off" :disabled="locked" :aria-invalid="!!pinError"
              :aria-describedby="locked ? `ban-pin-l-${v.key}` : pinError ? `ban-pin-e-${v.key}` : undefined" data-testid="door-ban-pin"
            >
            <button type="submit" class="btn-hud btn-hud-ghost reveal-btn" :disabled="checking || locked || pin.length < 6" data-testid="door-ban-reveal">
              {{ checking ? 'CHECKING…' : locked ? `WAIT ${lockLeft} S` : 'SHOW REASON' }}
            </button>
          </div>
          <p v-if="locked" :id="`ban-pin-l-${v.key}`" class="pin-err" data-testid="door-ban-locked">Too many wrong PINs. SHOW REASON works again in {{ lockLeft }} s.</p>
          <p v-else-if="pinError" :id="`ban-pin-e-${v.key}`" role="alert" class="pin-err">{{ pinError }}</p>
        </form>
        <p v-else class="bantext" data-testid="door-ban-no-pin">
          No manager PIN tonight, so the reason can't be shown here. Ask the manager on duty: they decide. A manager PIN can be generated in the
          event's DOOR tab in the admin.
        </p>
      </template>
      <template v-else>
        <ul class="banlist" aria-label="Matching entries" data-testid="door-ban-reasons">
          <li v-for="e in ban" :key="e.id">
            <strong>{{ e.name }}</strong> · {{ e.reason }}
            <span v-if="e.note" class="bannote">{{ e.note }}</span>
            <span class="bannote">ON THE LIST UNTIL {{ until(e.expires_at).toUpperCase() }} · The manager decides: TURN AWAY, or ADMIT with two taps.</span>
          </li>
        </ul>
      </template>
      <div v-if="approved || !managerPin" class="ban-acts">
        <button type="button" class="btn-hud btn-hud-ghost ban-act" data-testid="door-ban-turn-away" @click="emit('back')">
          <UserX style="width:18px;height:18px;" aria-hidden="true" /> TURN AWAY
        </button>
        <button v-if="revealed" ref="hideBtn" type="button" class="btn-hud btn-hud-ghost ban-act" data-testid="door-ban-hide" @click="hideReason">
          <EyeOff style="width:18px;height:18px;" aria-hidden="true" /> HIDE REASON
        </button>
      </div>
    </section>

    <p v-if="v.blocked" role="alert" class="block">
      <Ban style="width:18px;height:18px;flex-shrink:0;" aria-hidden="true" />
      DO NOT ADMIT · TICKET {{ v.blocked.toUpperCase() }}. {{ v.blocked === 'refunded' ? 'The buyer got their money back.' : 'The order was cancelled.' }}
    </p>
    <div v-else-if="primary" class="warnblock" data-testid="door-warning">
      <p class="primary">
        <AlertTriangle style="width:20px;height:20px;flex-shrink:0;" aria-hidden="true" /> {{ primary }}
      </p>
      <ul v-if="moreOthers > 0" class="flags" aria-label="Also check">
        <li v-for="r in otherReasons.slice(1)" :key="r.code">{{ r.text }}</li>
      </ul>
    </div>

    <div v-if="terms" class="terms">
      <span class="data-frag">{{ priceText }}</span>
      <span v-if="terms.cutoff_at" class="data-frag" :style="v.pastCutoff ? 'color:var(--color-status-archived);' : ''">
        {{ v.pastCutoff ? 'PAST CUTOFF' : 'CUTOFF' }} {{ timeLabel(terms.cutoff_at, timezone) }}
      </span>
      <span v-for="p in terms.perks" :key="p" class="data-frag">{{ p.toUpperCase() }}</span>
    </div>
    <p v-if="v.guest?.note" class="note">{{ v.guest.note }}</p>

    <p class="heads" data-testid="door-heads">
      IN {{ v.heads }} OF {{ v.allowance }} · <strong>{{ v.remaining }} REMAINING</strong>
    </p>

    <div v-if="!v.blocked" class="stepper" role="group" aria-label="Heads arriving now">
      <button type="button" class="btn-hud btn-hud-ghost step" :disabled="count <= 1" aria-label="One fewer" @click="step(-1)">
        <Minus style="width:20px;height:20px;" aria-hidden="true" />
      </button>
      <output class="n" aria-live="polite" :aria-label="`${count} arriving now`">{{ count }}</output>
      <button type="button" class="btn-hud btn-hud-ghost step" :disabled="count >= maxCount" aria-label="One more" @click="step(1)">
        <Plus style="width:20px;height:20px;" aria-hidden="true" />
      </button>
    </div>

    <button
      type="button" class="btn-hud admit"
      :class="v.blocked ? 'btn-hud-ghost' : armed ? 'btn-hud-error' : needsSecondTap ? 'btn-hud-ghost check' : 'btn-hud-cta'"
      :disabled="!!v.blocked || banGate" data-testid="door-admit" @click="admit"
    >
      <Check v-if="!needsSecondTap && !v.blocked && !banGate" style="width:20px;height:20px;" aria-hidden="true" />
      <template v-if="v.blocked">CANNOT ADMIT</template>
      <template v-else-if="banGate">ASK A MANAGER</template>
      <template v-else-if="armed">TAP AGAIN · ADMIT {{ count }}<span v-if="more" class="more">+{{ more }} MORE</span></template>
      <template v-else-if="needsSecondTap">{{ checkText }}</template>
      <template v-else>ADMIT {{ count }}</template>
    </button>
  </article>
</template>

<style scoped>
.card {
  padding: 14px;
  display: grid;
  gap: 10px;
  min-width: 0;
}
.back {
  justify-self: start;
  min-height: 56px;
  height: 56px;
  font-size: 10px;
}
.name {
  margin: 0;
  font-family: var(--font-command);
  font-size: 26px;
  font-weight: 700;
  line-height: 1.15;
  color: var(--color-on-surface);
  overflow-wrap: anywhere;
}
.meta {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  text-transform: uppercase;
  color: var(--color-on-surface-variant);
  overflow-wrap: anywhere;
}
.block {
  margin: 0;
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 10px 12px;
  background: var(--color-error-container);
  color: var(--color-on-surface);
  font-size: 15px;
  font-weight: 600;
}
.terms {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.terms .data-frag {
  font-size: 10px;
}
.note {
  margin: 0;
  font-size: 14px;
  color: var(--color-on-surface-variant);
}
.name:focus {
  outline: none;
}
.name:focus-visible {
  outline: 1px solid var(--color-primary);
  outline-offset: 2px;
}
.warnblock {
  display: grid;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid var(--color-status-archived);
  background: color-mix(in srgb, var(--color-status-archived) 16%, transparent);
  color: var(--color-status-archived);
}
.primary {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  font-family: var(--font-command);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: .03em;
  overflow-wrap: anywhere;
}
.flags {
  list-style: none;
  margin: 0;
  padding: 0 0 0 28px;
  display: grid;
  gap: 2px;
  font-family: var(--font-terminal);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .05em;
}
.heads {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 13px;
  letter-spacing: .05em;
  color: var(--color-on-surface);
}
.stepper {
  display: grid;
  grid-template-columns: 64px 1fr 64px;
  align-items: center;
  gap: 8px;
}
.step {
  height: 56px;
  min-height: 56px;
}
.n {
  text-align: center;
  font-family: var(--font-command);
  font-size: 32px;
  font-weight: 700;
  color: var(--color-on-surface);
}
.admit {
  min-height: 64px;
  height: auto;
  padding: 8px 12px;
  font-size: 14px;
  white-space: normal;
  text-align: center;
  flex-wrap: wrap;
}
.admit.check {
  border: 2px solid var(--color-status-archived);
  color: var(--color-status-archived);
}
.banblock {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border-left: 3px solid var(--color-status-archived);
  background: var(--color-surface-container);
}
.bantitle {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  font-family: var(--font-terminal);
  font-size: 13px;
  font-weight: 600;
  letter-spacing: .05em;
  color: var(--color-status-archived);
  overflow-wrap: anywhere;
}
.bantext {
  margin: 0;
  font-size: 14px;
  color: var(--color-on-surface);
}
.reveal {
  display: grid;
  gap: 4px;
}
.reveal-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.pin {
  flex: 1 1 120px;
  min-width: 0;
  height: 56px;
  font-size: 22px;
  letter-spacing: .3em;
}
.reveal-btn {
  min-height: 56px;
  height: 56px;
  font-size: 11px;
  flex: 1 1 140px;
}
.pin-err {
  margin: 0;
  font-size: 14px;
  color: var(--color-error);
}
.ban-acts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 8px;
}
.ban-act {
  min-height: 56px;
  height: 56px;
  font-size: 11px;
}
.banlist {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 6px;
  font-size: 15px;
  color: var(--color-on-surface);
  overflow-wrap: anywhere;
}
.bannote {
  display: block;
  font-size: 12px;
  color: var(--color-on-surface-variant);
}
.more {
  flex-basis: 100%;
  font-size: 11px;
  opacity: .85;
}
</style>
