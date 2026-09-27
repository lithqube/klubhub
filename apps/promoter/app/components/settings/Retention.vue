<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { usePrivacyStore } from '~/stores/privacy'
import type { ApiError } from '~/types/event'
import {
  countsText, daysAfterText, daysAfterEnd, daysLabel, RETENTION_DEFAULT, RETENTION_MAX, RETENTION_MIN, RETENTION_PRESETS,
  retentionErrorText, shortDate, TRIGGER_LABEL, validateRetentionDays,
} from '~/utils/privacy'

/**
 * DATA RETENTION (P2.5): how many days after an event its guest names and
 * contacts are kept, what is erased and what survives, the next erasures
 * and the last ones. Owners and admins change it (org.update); everyone
 * else sees it read-only.
 */
const props = defineProps<{ readOnly: boolean, timezone: string }>()
const store = usePrivacyStore()
const { retention, retentionLoading, retentionError } = storeToRefs(store)
const uid = useId()

type Choice = (typeof RETENTION_PRESETS)[number] | 'custom'
const choice = ref<Choice>(RETENTION_DEFAULT)
const custom = ref('')
const touched = ref(false)
const busy = ref(false)
const saved = ref('')
const saveError = ref('')
const customInput = ref<HTMLInputElement | null>(null)

function syncFrom(days: number) {
  const preset = (RETENTION_PRESETS as readonly number[]).includes(days)
  choice.value = preset ? (days as Choice) : 'custom'
  // A preset leaves the custom field as typed (so this never looks like an edit).
  if (!preset) custom.value = String(days)
  touched.value = false
}
watch(() => retention.value?.retention_days, (d) => {
  if (d !== undefined && !busy.value) syncFrom(d)
}, { immediate: true })

const parsed = computed(() => (choice.value === 'custom' ? validateRetentionDays(custom.value) : { days: choice.value, error: null }))
const fieldError = computed(() => (choice.value === 'custom' && touched.value ? parsed.value.error : null))
const changed = computed(() => parsed.value.days !== null && parsed.value.days !== retention.value?.retention_days)

watch([choice, custom], () => {
  saved.value = ''
  saveError.value = ''
})
watch(choice, (c) => {
  if (c === 'custom') nextTick(() => customInput.value?.focus())
})

async function save() {
  touched.value = true
  const days = parsed.value.days
  if (days === null) {
    customInput.value?.focus()
    return
  }
  busy.value = true
  saveError.value = ''
  saved.value = ''
  try {
    await store.saveRetention(days)
    syncFrom(days)
    saved.value = `SAVED · GUEST DATA IS NOW KEPT ${daysLabel(days).toUpperCase()} AFTER EACH EVENT. DATES BELOW ARE UPDATED.`
  } catch (e) {
    saveError.value = retentionErrorText(e as ApiError)
  } finally {
    busy.value = false
  }
}

const date = (iso: string) => shortDate(iso, props.timezone)
const now = Date.now()
</script>

<template>
  <div class="space-y-3">
    <p v-if="retentionError && !retention" role="alert" class="glass accent-bar-failed row" style="padding:10px 14px;margin:0;font-size:13px;">
      COULDN'T LOAD THE RETENTION SETTINGS.
      <button type="button" class="btn-hud btn-hud-ghost act" :disabled="retentionLoading" @click="store.fetchRetention()">RETRY</button>
    </p>
    <p v-else-if="retentionLoading && !retention" role="status" class="data-frag" style="font-size:11px;">LOADING RETENTION…</p>

    <template v-if="retention">
      <p v-if="readOnly" class="glass" style="padding:10px 14px;font-size:13px;margin:0;" data-testid="retention-read-only">
        READ ONLY · Owners and admins decide how long guest data is kept.
      </p>

      <form class="glass panel" novalidate aria-labelledby="retention-period-h" @submit.prevent="save">
        <fieldset :disabled="readOnly" class="choices" :aria-describedby="`${uid}-hint`">
          <legend id="retention-period-h" class="lbl">KEEP GUEST NAMES AND CONTACTS FOR</legend>
          <div class="chips">
            <label v-for="d in RETENTION_PRESETS" :key="d" class="chip" :class="{ on: choice === d }">
              <input v-model="choice" type="radio" :name="`${uid}-days`" :value="d" class="sr-only">
              {{ daysLabel(d).toUpperCase() }}<template v-if="d === RETENTION_DEFAULT"> · DEFAULT</template>
            </label>
            <label class="chip" :class="{ on: choice === 'custom' }">
              <input v-model="choice" type="radio" :name="`${uid}-days`" value="custom" class="sr-only">
              CUSTOM
            </label>
          </div>
          <div v-if="choice === 'custom'" class="custom">
            <label :for="`${uid}-custom`" class="lbl">DAYS ({{ RETENTION_MIN }}–{{ RETENTION_MAX }})</label>
            <input
              :id="`${uid}-custom`" ref="customInput" v-model="custom" class="hud-input" inputmode="numeric" maxlength="3" style="width:120px;"
              :aria-invalid="!!fieldError" :aria-describedby="fieldError ? `${uid}-err` : undefined" data-testid="retention-custom"
              @blur="touched = true"
            >
            <span v-if="fieldError" :id="`${uid}-err`" class="err" data-testid="retention-error">{{ fieldError }}</span>
          </div>
          <p :id="`${uid}-hint`" class="hint">
            Counted from the end of each event. Erasing runs every hour; owners and admins can also erase an ended event now from its GUESTS tab.
          </p>
        </fieldset>

        <p v-if="saved" role="status" class="ok" data-testid="retention-saved">{{ saved }}</p>
        <p v-if="saveError" role="alert" class="err">{{ saveError }}</p>
        <div v-if="!readOnly" class="row" style="justify-content:flex-end;">
          <button type="submit" class="btn-hud btn-hud-cta act" :disabled="busy || !changed">{{ busy ? 'SAVING…' : 'SAVE RETENTION' }}</button>
        </div>
      </form>

      <div class="cols">
        <section class="glass panel" :aria-labelledby="`${uid}-gone`">
          <h3 :id="`${uid}-gone`" class="lbl">ERASED</h3>
          <ul class="list-txt">
            <li>Guest names, emails, phone numbers and notes</li>
            <li>Ticket holder and buyer names, emails and ticket barcodes</li>
            <li>Submitter contacts and the event's door PINs</li>
          </ul>
        </section>
        <section class="glass panel" :aria-labelledby="`${uid}-kept`">
          <h3 :id="`${uid}-kept`" class="lbl">KEPT</h3>
          <ul class="list-txt">
            <li>How many came: counts, statuses and check-ins (as "Erased guest")</li>
            <li>The report: check-in curve, walk-ups, numbers by list and submitter</li>
            <li>Lists, entry terms and submitter names</li>
          </ul>
        </section>
      </div>
      <p class="hint" style="margin:0;">
        Lists are name-only unless you turn on COLLECT EMAIL &amp; PHONE for a list. ID images are never stored.
      </p>

      <section class="glass panel" aria-labelledby="retention-upcoming-h">
        <h3 id="retention-upcoming-h" class="lbl">NEXT ERASURES</h3>
        <p v-if="!retention.upcoming.length" class="hint" style="margin:0;">No ended events are waiting to be erased.</p>
        <ul v-else class="rows" data-testid="retention-upcoming">
          <li v-for="u in retention.upcoming" :key="u.event_id">
            <span class="when">{{ date(u.purge_after).toUpperCase() }}<template v-if="Date.parse(u.purge_after) <= now"> · DUE</template></span>
            <NuxtLink :to="`/events/${u.event_id}/guests`" class="ev">{{ u.title }}</NuxtLink>
            <span class="meta">{{ Date.parse(u.ends_at) <= now ? `ended ${date(u.ends_at)}` : `ends ${date(u.ends_at)}` }} · {{ daysAfterText(daysAfterEnd(u.ends_at, u.purge_after)) }}</span>
          </li>
        </ul>
      </section>

      <section class="glass panel" aria-labelledby="retention-recent-h">
        <h3 id="retention-recent-h" class="lbl">RECENTLY ERASED</h3>
        <p v-if="!retention.recent.length" class="hint" style="margin:0;">Nothing erased yet.</p>
        <ul v-else class="rows" data-testid="retention-recent">
          <li v-for="r in retention.recent" :key="r.event_id">
            <span class="when">{{ date(r.purged_at).toUpperCase() }}</span>
            <NuxtLink :to="`/events/${r.event_id}/report`" class="ev">{{ r.title }}</NuxtLink>
            <span class="meta"><span class="trig">{{ TRIGGER_LABEL[r.trigger] ?? r.trigger.toUpperCase() }}</span> · {{ countsText(r.counts) }}</span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>

<style scoped>
.panel {
  padding: 14px;
  display: grid;
  gap: 10px;
  min-width: 0;
}
.lbl {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .07em;
  color: var(--color-tertiary);
}
.choices {
  border: 0;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 8px;
  min-width: 0;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.chip {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  padding: 0 12px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  color: var(--color-on-surface-variant);
  background: var(--color-surface-container);
  border: 1px dashed var(--color-outline);
  cursor: pointer;
}
.chip.on {
  color: var(--color-primary);
  border: 1px solid var(--color-primary-dim);
}
.chip:focus-within {
  outline: 1px solid var(--color-primary);
  outline-offset: 2px;
}
fieldset:disabled .chip { cursor: default; opacity: .75; }
.custom {
  display: grid;
  gap: 4px;
}
.hint {
  margin: 0;
  font-size: 12px;
  color: var(--color-on-surface-variant);
}
.err {
  margin: 0;
  font-size: 12px;
  color: var(--color-error);
}
.ok {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  color: var(--color-primary);
}
.row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.act {
  min-height: 44px;
  font-size: 11px;
}
.cols {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
}
@media (min-width: 800px) {
  .cols { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
.list-txt {
  margin: 0;
  padding-left: 18px;
  list-style: disc;
  font-size: 13px;
  color: var(--color-on-surface-variant);
}
.rows {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
}
.rows li {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr);
  gap: 0 10px;
  align-items: center;
  padding: 6px 10px;
  background: var(--color-surface-container);
  font-size: 13px;
}
.when {
  grid-row: span 2;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  color: var(--color-on-surface);
}
.ev {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--color-primary);
}
.meta {
  font-size: 12px;
  color: var(--color-on-surface-variant);
  overflow-wrap: anywhere;
}
.trig {
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
