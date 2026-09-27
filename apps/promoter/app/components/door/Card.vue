<script setup lang="ts">
import { AlertTriangle, ArrowLeft, Ban, Check, Minus, Plus } from 'lucide-vue-next'
import type { SubjectView } from '~/utils/doorState'
import { IMPORT_PRESETS } from '~/utils/attendeeImport'
import { timeLabel } from '~/utils/datetime'

/**
 * One guest or ticket at the door: list and entry terms, perks, heads in
 * and remaining, the +N stepper for partial arrivals, and ADMIT. Anything
 * unusual (past the list cutoff, allowance used up, not marked going, a
 * double entry from another device) arms the button first: a second tap
 * admits anyway. Cancelled and refunded tickets cannot be admitted.
 */
const props = defineProps<{ view: SubjectView, timezone: string }>()
const emit = defineEmits<{ admit: [count: number], back: [] }>()

const v = computed(() => props.view)
const maxCount = computed(() => (v.value.remaining > 0 ? v.value.remaining : v.value.allowance))
const count = ref(1)
const armed = ref(false)
watch(() => [v.value.key, v.value.remaining] as const, () => {
  count.value = Math.max(1, maxCount.value)
  armed.value = false
}, { immediate: true })

const reasons = computed(() => {
  const r: string[] = []
  if (v.value.pastCutoff) r.push('PAST CUTOFF')
  if (v.value.remaining === 0) r.push(v.value.heads ? 'ALREADY IN' : 'NO HEADS LEFT')
  if (v.value.warning) r.push(v.value.warning)
  if (v.value.conflict) r.push('DOUBLE ENTRY FLAGGED')
  return r
})
const needsSecondTap = computed(() => reasons.value.length > 0)

const terms = computed(() => v.value.list?.entry_terms ?? null)
const priceText = computed(() => {
  if (!terms.value) return ''
  return terms.value.price_mode === 'reduced' ? terms.value.reduced_price_text || 'REDUCED' : 'FREE ENTRY'
})
const sourceLabel = (s: string) => IMPORT_PRESETS.find(p => p.id === s)?.label ?? s.toUpperCase()

function admit() {
  if (v.value.blocked) return
  if (needsSecondTap.value && !armed.value) {
    armed.value = true
    return
  }
  armed.value = false
  emit('admit', count.value)
}
</script>

<template>
  <article class="card glass" :class="v.blocked ? 'accent-bar-failed' : needsSecondTap ? 'accent-bar-archived' : 'accent-bar-ready'" :aria-labelledby="`card-${v.key}`">
    <button type="button" class="btn-hud btn-hud-ghost back" @click="emit('back')">
      <ArrowLeft style="width:16px;height:16px;" aria-hidden="true" /> BACK TO SEARCH
    </button>

    <h2 :id="`card-${v.key}`" class="name">{{ v.name }}</h2>
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

    <div v-if="terms" class="terms">
      <span class="data-frag">{{ priceText }}</span>
      <span v-if="terms.cutoff_at" class="data-frag" :style="v.pastCutoff ? 'color:var(--color-status-archived);' : ''">
        {{ v.pastCutoff ? 'PAST CUTOFF' : 'CUTOFF' }} {{ timeLabel(terms.cutoff_at, timezone) }}
      </span>
      <span v-for="p in terms.perks" :key="p" class="data-frag">{{ p.toUpperCase() }}</span>
    </div>
    <p v-if="v.guest?.note" class="note">{{ v.guest.note }}</p>

    <ul v-if="reasons.length && !v.blocked" class="flags" aria-label="Check before admitting">
      <li v-for="r in reasons" :key="r"><AlertTriangle style="width:14px;height:14px;" aria-hidden="true" /> {{ r }}</li>
    </ul>

    <p class="heads" data-testid="door-heads">
      IN {{ v.heads }} OF {{ v.allowance }} · <strong>{{ v.remaining }} REMAINING</strong>
    </p>

    <div v-if="!v.blocked" class="stepper" role="group" aria-label="Heads arriving now">
      <button type="button" class="btn-hud btn-hud-ghost step" :disabled="count <= 1" aria-label="One fewer" @click="count--; armed = false">
        <Minus style="width:20px;height:20px;" aria-hidden="true" />
      </button>
      <output class="n" aria-live="polite" :aria-label="`${count} arriving now`">{{ count }}</output>
      <button type="button" class="btn-hud btn-hud-ghost step" :disabled="count >= maxCount" aria-label="One more" @click="count++; armed = false">
        <Plus style="width:20px;height:20px;" aria-hidden="true" />
      </button>
    </div>

    <button
      type="button" class="btn-hud admit" :class="v.blocked ? 'btn-hud-ghost' : armed ? 'btn-hud-error' : 'btn-hud-cta'"
      :disabled="!!v.blocked" data-testid="door-admit" @click="admit"
    >
      <Check v-if="!armed && !v.blocked" style="width:20px;height:20px;" aria-hidden="true" />
      <template v-if="v.blocked">CANNOT ADMIT</template>
      <template v-else-if="armed">{{ reasons[0] }} — TAP AGAIN TO ADMIT {{ count }}</template>
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
.flags {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
  color: var(--color-status-archived);
  font-family: var(--font-terminal);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .05em;
}
.flags li {
  display: flex;
  align-items: center;
  gap: 6px;
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
}
</style>
