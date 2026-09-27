<script setup lang="ts">
import { useNow, watchDebounced } from '@vueuse/core'
import { storeToRefs } from 'pinia'
import { usePrivacyStore } from '~/stores/privacy'
import { useSessionStore } from '~/stores/session'
import type { ApiError } from '~/types/event'
import type { RetentionPreview } from '~/types/privacy'
import {
  authIsStale, countsText, daysAfterEnd, daysAfterText, daysLabel, ERASED_ITEMS, isStepUp, KEPT_ITEMS, RETENTION_DEFAULT, RETENTION_MAX,
  RETENTION_MIN, RETENTION_PRESETS, retentionErrorText, retentionSaveButton, shortDate, signInAgainRoute, TRIGGER_LABEL, validateRetentionDays, wouldPurgeText,
} from '~/utils/privacy'

/**
 * DATA RETENTION (P2.5): how many days after an event its guest names and
 * contacts are kept, what is erased and what survives, the next erasures
 * and the last ones. Owners and admins change it (org.update); everyone
 * else sees it read-only.
 *
 * A shorter period can make ended events due at once. As the value
 * changes, the server's preview says which; the owner then has to tick
 * "I understand" and SAVE AND ERASE N (the save carries that count, and
 * the server wants a recent sign-in for it, like ERASE NOW).
 */
const props = defineProps<{ readOnly: boolean, timezone: string }>()
const store = usePrivacyStore()
const { retention, retentionLoading, retentionError } = storeToRefs(store)
const { me } = storeToRefs(useSessionStore())
const route = useRoute()
const router = useRouter()
const uid = useId()
const now = useNow({ interval: 60_000 })

type Choice = (typeof RETENTION_PRESETS)[number] | 'custom'
const choice = ref<Choice>(RETENTION_DEFAULT)
const custom = ref('')
const touched = ref(false)
const busy = ref(false)
const saved = ref('')
const savedEl = ref<HTMLElement | null>(null)
const saveError = ref<ApiError | null>(null)
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

// Back from SIGN IN AGAIN (?retention_days=N): drop the query, pick the value again.
// Dropping it may remount the page, so the value survives in useState.
const reopen = useState<number | null>('retention-reopen-days', () => null)
let unmounted = false
onBeforeUnmount(() => { unmounted = true })
onMounted(async () => {
  if (route.query.retention_days !== undefined) {
    reopen.value = validateRetentionDays(String(route.query.retention_days)).days
    const { retention_days: _drop, ...query } = route.query
    await router.replace({ query, hash: route.hash })
  }
  const want = reopen.value
  if (unmounted || want === null) return
  reopen.value = null
  if (!props.readOnly) syncFrom(want)
})

const parsed = computed(() => (choice.value === 'custom' ? validateRetentionDays(custom.value) : { days: choice.value, error: null }))
const fieldError = computed(() => (choice.value === 'custom' && touched.value ? parsed.value.error : null))
const changed = computed(() => parsed.value.days !== null && parsed.value.days !== retention.value?.retention_days)
const shorter = computed(() => changed.value && parsed.value.days! < (retention.value?.retention_days ?? RETENTION_DEFAULT))

watch([choice, custom], () => {
  saved.value = ''
  saveError.value = null
})
watch(choice, (c) => {
  if (c === 'custom') nextTick(() => customInput.value?.focus())
})

// ---------------------------------------------------------------- shorten preview
const preview = ref<{ days: number, result: RetentionPreview } | null>(null)
const previewing = ref(false)
const ack = ref(false)
let previewSeq = 0

async function loadPreview(days: number) {
  const seq = ++previewSeq
  previewing.value = true
  try {
    const result = await store.previewRetention(days)
    if (seq === previewSeq) preview.value = { days, result }
  } catch {
    // The save is still guarded by the server (409 retention_would_purge fills the warning in).
    if (seq === previewSeq) preview.value = null
  } finally {
    if (seq === previewSeq) previewing.value = false
  }
}
watch([() => parsed.value.days, shorter], () => {
  ack.value = false
  if (!shorter.value || props.readOnly) {
    previewSeq++
    previewing.value = false
    preview.value = null
    return
  }
  previewing.value = true
}, { immediate: true })
watchDebounced([() => parsed.value.days, shorter], ([days]) => {
  if (shorter.value && days !== null && !props.readOnly) void loadPreview(days)
}, { debounce: 300, immediate: true })

const erasing = computed(() => (preview.value && preview.value.days === parsed.value.days && shorter.value ? preview.value.result.count : 0))
const warning = computed(() => (erasing.value ? wouldPurgeText(parsed.value.days!, preview.value!.result) : ''))
const stale = computed(() => erasing.value > 0 && authIsStale(me.value?.auth_time, now.value.getTime()))
const signInAgain = computed(() => signInAgainRoute(`/settings?retention_days=${parsed.value.days ?? ''}#retention`, 'retention'))
const errorText = computed(() => (saveError.value ? retentionErrorText(saveError.value) : ''))
const stepUpError = computed(() => !!saveError.value && isStepUp(saveError.value.error))
const button = computed(() => retentionSaveButton({
  busy: busy.value, changed: changed.value, shorter: shorter.value, previewing: previewing.value, erasing: erasing.value, ack: ack.value, stale: stale.value,
}))

async function save() {
  touched.value = true
  const days = parsed.value.days
  if (days === null) {
    customInput.value?.focus()
    return
  }
  if (button.value.disabled) return
  busy.value = true
  saveError.value = null
  saved.value = ''
  const confirm = erasing.value
  try {
    await store.saveRetention(days, confirm || undefined)
    syncFrom(days)
    saved.value = `Guest data is now kept ${daysLabel(days)} after each event.`
      + (confirm ? ` ${confirm} ended ${confirm === 1 ? 'event is' : 'events are'} erased within the hour.` : '')
      + ' The dates below are updated.'
    nextTick(() => savedEl.value?.focus())
  } catch (e) {
    const err = e as ApiError
    saveError.value = err
    if (err.error === 'retention_would_purge') {
      // The server counted differently (an event ended or was erased meanwhile): show its list.
      const d = err.detail as Partial<RetentionPreview> | undefined
      preview.value = { days, result: { would_purge: Array.isArray(d?.would_purge) ? d.would_purge : [], count: Number(d?.count) || 0 } }
      ack.value = false
    }
  } finally {
    busy.value = false
  }
}

const zone = (tz?: string) => tz || props.timezone
const date = (iso: string, tz?: string) => shortDate(iso, zone(tz), now.value.getTime())
const ended = (iso: string) => Date.parse(iso) <= now.value.getTime()
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
            Counted from the end of each event. Erasing runs every hour; owners and admins can also erase an ended event now from its GUESTS or REPORT tab.
          </p>
        </fieldset>

        <div v-if="warning" role="alert" class="warn-box" data-testid="retention-warning">
          <p class="warn-txt">{{ warning }}</p>
          <p v-if="stale" class="warn-txt" data-testid="retention-stale">
            For safety, erasing needs a recent sign-in. Sign in again and you'll come straight back here.
            <NuxtLink :to="signInAgain" class="link">SIGN IN AGAIN →</NuxtLink>
          </p>
          <label v-else class="ack">
            <input v-model="ack" type="checkbox" data-testid="retention-ack">
            I understand
          </label>
        </div>

        <p v-if="saved" ref="savedEl" tabindex="-1" class="ok" data-testid="retention-saved">
          <span class="ok-lbl">SAVED</span> {{ saved }}
        </p>
        <p v-if="errorText" role="alert" class="err" data-testid="retention-save-error">
          {{ errorText }}
          <NuxtLink v-if="stepUpError" :to="signInAgain" class="link">SIGN IN AGAIN →</NuxtLink>
        </p>
        <div v-if="!readOnly" class="row" style="justify-content:flex-end;">
          <button
            type="submit" class="btn-hud act" :class="button.danger ? 'btn-hud-error' : 'btn-hud-cta'" :disabled="button.disabled" :aria-busy="busy || previewing"
            data-testid="retention-save"
          >
            {{ button.label }}
          </button>
        </div>
      </form>

      <div class="cols">
        <section class="glass panel" :aria-labelledby="`${uid}-gone`">
          <h3 :id="`${uid}-gone`" class="lbl">ERASED</h3>
          <ul class="list-txt">
            <li v-for="item in ERASED_ITEMS" :key="item">{{ item }}</li>
          </ul>
        </section>
        <section class="glass panel" :aria-labelledby="`${uid}-kept`">
          <h3 :id="`${uid}-kept`" class="lbl">KEPT</h3>
          <ul class="list-txt">
            <li v-for="item in KEPT_ITEMS" :key="item">{{ item }}</li>
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
            <span class="when">{{ date(u.purge_after, u.timezone).toUpperCase() }}<template v-if="ended(u.purge_after)"> · DUE</template></span>
            <NuxtLink :to="`/events/${u.event_id}/guests`" class="ev">{{ u.title }}</NuxtLink>
            <span class="meta">{{ ended(u.ends_at) ? `ended ${date(u.ends_at, u.timezone)}` : `ends ${date(u.ends_at, u.timezone)}` }} · {{ daysAfterText(daysAfterEnd(u.ends_at, u.purge_after), ended(u.ends_at)) }}</span>
          </li>
        </ul>
      </section>

      <section class="glass panel" aria-labelledby="retention-recent-h">
        <h3 id="retention-recent-h" class="lbl">RECENTLY ERASED</h3>
        <p v-if="!retention.recent.length" class="hint" style="margin:0;">Nothing erased yet.</p>
        <ul v-else class="rows" data-testid="retention-recent">
          <li v-for="r in retention.recent" :key="r.event_id">
            <span class="when">{{ date(r.purged_at, r.timezone).toUpperCase() }}</span>
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
  font-size: 13px;
  color: var(--color-on-surface);
}
.ok:focus { outline: none; }
.ok:focus-visible { outline: 1px solid var(--color-primary); }
.ok-lbl {
  margin-right: 6px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  color: var(--color-primary);
}
.warn-box {
  display: grid;
  gap: 6px;
  padding: 10px 12px;
  border-left: 3px solid var(--color-error);
  background: var(--color-surface-container);
}
.warn-txt {
  margin: 0;
  font-size: 13px;
  color: var(--color-on-surface);
  overflow-wrap: anywhere;
}
.ack {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 44px;
  font-size: 13px;
  cursor: pointer;
}
.link {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  margin-left: 6px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
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
