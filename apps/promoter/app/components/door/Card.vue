<script setup lang="ts">
import { AlertTriangle, ArrowLeft, Ban, Check, Minus, Plus } from 'lucide-vue-next'
import type { SubjectView } from '~/utils/doorState'
import { IMPORT_PRESETS } from '~/utils/attendeeImport'
import { timeLabel } from '~/utils/datetime'
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
 */
const props = defineProps<{ view: SubjectView, timezone: string, selfDevice: string }>()
const emit = defineEmits<{ admit: [count: number, keyboard: boolean], back: [] }>()

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

// A new card takes focus on its name, so screen readers start there.
watch(() => v.value.key, () => nextTick(() => heading.value?.focus({ preventScroll: false })))
onMounted(() => heading.value?.focus())

const reasons = computed(() => admitReasons(v.value))
const needsSecondTap = computed(() => reasons.value.length > 0)
const primary = computed(() => {
  const r = reasons.value[0]
  if (!r) return ''
  const last = v.value.lastIn
  if (r.code === 'already_in' && last) return `ALREADY IN · ${timeLabel(last.at, props.timezone)} · ${deviceTag(last.device_id, props.selfDevice)}`
  return r.text
})
const more = computed(() => reasons.value.length - 1)

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
  if (v.value.blocked) return
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
  <article class="card glass" :class="v.blocked ? 'accent-bar-failed' : needsSecondTap ? 'accent-bar-archived' : 'accent-bar-ready'" :aria-labelledby="`card-${v.key}`">
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

    <p v-if="v.blocked" role="alert" class="block">
      <Ban style="width:18px;height:18px;flex-shrink:0;" aria-hidden="true" />
      DO NOT ADMIT · TICKET {{ v.blocked.toUpperCase() }}. {{ v.blocked === 'refunded' ? 'The buyer got their money back.' : 'The order was cancelled.' }}
    </p>
    <div v-else-if="primary" class="warnblock" data-testid="door-warning">
      <p class="primary">
        <AlertTriangle style="width:20px;height:20px;flex-shrink:0;" aria-hidden="true" /> {{ primary }}
      </p>
      <ul v-if="more" class="flags" aria-label="Also check">
        <li v-for="r in reasons.slice(1)" :key="r.code">{{ r.text }}</li>
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
      :disabled="!!v.blocked" data-testid="door-admit" @click="admit"
    >
      <Check v-if="!needsSecondTap && !v.blocked" style="width:20px;height:20px;" aria-hidden="true" />
      <template v-if="v.blocked">CANNOT ADMIT</template>
      <template v-else-if="armed">TAP AGAIN · ADMIT {{ count }}<span v-if="more" class="more">+{{ more }} MORE</span></template>
      <template v-else-if="needsSecondTap">CHECK: {{ reasons[0]!.text }}</template>
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
.more {
  flex-basis: 100%;
  font-size: 11px;
  opacity: .85;
}
</style>
