<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { Clock, ShieldCheck } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { usePrivacyStore } from '~/stores/privacy'
import { useSessionStore } from '~/stores/session'
import type { ApiError, PromoterEvent } from '~/types/event'
import { countsText, privacyBanner } from '~/utils/privacy'

/**
 * The quiet retention banner of the GUESTS and REPORT tabs (P2.5): after
 * the event ends it says when guest names and contacts are erased, then
 * that they were. Owners and admins can ERASE NOW (typed title + step-up).
 * The page loads the event's purge state; `purged` fires after an erase so
 * it can reload its tables.
 */
const props = defineProps<{ event: Pick<PromoterEvent, 'id' | 'title' | 'ends_at' | 'timezone'> }>()
const emit = defineEmits<{ purged: [] }>()

const store = usePrivacyStore()
const { events } = storeToRefs(store)
const { canManageOrg } = storeToRefs(useSessionStore())
const now = useNow({ interval: 60_000 })

const privacy = computed(() => events.value[props.event.id] ?? null)
const banner = computed(() => privacyBanner(props.event, privacy.value, now.value.getTime()))
const canErase = computed(() => canManageOrg.value && (banner.value.state === 'scheduled' || banner.value.state === 'due'))

const open = ref(false)
const busy = ref(false)
const error = ref<ApiError | null>(null)
const done = ref('')
const doneEl = ref<HTMLElement | null>(null)

function start() {
  error.value = null
  open.value = true
}

async function erase(typed: string) {
  busy.value = true
  error.value = null
  try {
    const r = await store.purgeEvent(props.event.id, typed)
    open.value = false
    done.value = r.counts
      ? `Erased: ${countsText(r.counts)}. Counts, check-ins and the report stay.`
      : 'Erased. Counts, check-ins and the report stay.'
    emit('purged')
    // After the dialog has handed focus back (its trigger is gone by then).
    setTimeout(() => doneEl.value?.focus(), 60)
  } catch (e) {
    const err = e as ApiError
    error.value = err
    // Someone (or the schedule) got there first: show the erased state.
    if (err.error === 'event_purged') {
      await store.fetchEvent(props.event.id)
      emit('purged')
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="banner.state !== 'none'" class="privacy-banner glass" :class="`privacy-${banner.state}`" data-testid="privacy-banner">
    <component :is="banner.state === 'purged' ? ShieldCheck : Clock" class="ic" aria-hidden="true" />
    <div class="body">
      <p class="txt" data-testid="privacy-banner-text">{{ banner.text }}</p>
      <p v-if="done" ref="doneEl" role="status" tabindex="-1" class="done" data-testid="privacy-erased">{{ done }}</p>
      <div class="acts">
        <NuxtLink to="/settings#retention" class="link">RETENTION SETTINGS →</NuxtLink>
        <button v-if="canErase" type="button" class="btn-hud btn-hud-ghost erase" data-testid="erase-now" @click="start">ERASE NOW</button>
      </div>
    </div>
    <EventEraseDialog
      v-if="canErase || open" v-model:open="open" :title="event.title" :personal-rows="privacy?.personal_rows ?? null" :busy="busy" :error="error"
      @confirm="erase"
    />
  </div>
</template>

<style scoped>
.privacy-banner {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 14px;
  border-left: 3px solid var(--color-tertiary);
  min-width: 0;
}
.privacy-due { border-left-color: var(--color-status-archived); }
.privacy-purged { border-left-color: var(--color-primary-dim); }
.ic {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  margin-top: 2px;
  color: var(--color-tertiary);
}
.body {
  display: grid;
  gap: 4px;
  min-width: 0;
  flex: 1;
}
.txt {
  margin: 0;
  font-size: 13px;
  color: var(--color-on-surface);
  overflow-wrap: anywhere;
}
.done {
  margin: 0;
  font-size: 13px;
  color: var(--color-primary);
}
.done:focus { outline: none; }
.done:focus-visible { outline: 1px solid var(--color-primary); }
.acts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 12px;
}
.link {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-primary);
}
.erase {
  min-height: 44px;
  font-size: 11px;
  color: var(--color-error);
}
</style>
