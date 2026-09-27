<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import { timeLabel } from '~/utils/datetime'

/** Sync state at a glance: offline, queued count, last sync. Tap to sync now. */
const store = useDoorStore()
const { queued, offline, syncing, lastSyncAt, syncError, sessionEnded, bundle } = storeToRefs(store)

const tz = computed(() => bundle.value?.event.timezone ?? 'UTC')
const state = computed(() => {
  if (sessionEnded.value) return { tone: 'failed', text: `SIGNED OUT · ${queued.value} QUEUED` }
  if (offline.value) return { tone: 'archived', text: queued.value ? `OFFLINE · ${queued.value} QUEUED` : 'OFFLINE' }
  if (syncError.value) return { tone: 'failed', text: `SYNC ERROR · ${queued.value} QUEUED` }
  if (syncing.value) return { tone: 'pending', text: 'SYNCING…' }
  if (queued.value) return { tone: 'pending', text: `${queued.value} QUEUED` }
  return { tone: 'ready', text: lastSyncAt.value ? `SYNCED ${timeLabel(lastSyncAt.value, tz.value)}` : 'SYNCED' }
})
</script>

<template>
  <button
    type="button" class="sync btn-hud btn-hud-ghost" :class="`accent-bar-${state.tone}`" data-testid="door-sync"
    :aria-label="`Sync status: ${state.text}. Sync now`" @click="store.sync()"
  >
    <span role="status">{{ state.text }}</span>
  </button>
</template>

<style scoped>
.sync {
  min-height: 56px;
  height: auto;
  padding: 0 12px;
  font-size: 10px;
  white-space: nowrap;
}
</style>
