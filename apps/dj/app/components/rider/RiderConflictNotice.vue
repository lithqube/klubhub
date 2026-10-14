<script setup lang="ts">
// RiderConflictNotice — shown by an editor when its autosave was rejected as
// stale (409): the template/attachment was saved from another tab or device
// after this one loaded it. Mirrors the gig/finance conflict copy: the user's
// edits were not saved, and they choose to reload the latest (which discards
// their unsaved edits). Nothing is overwritten automatically.

import { computed, ref } from 'vue'
import { useRiderAutosave } from '~/composables/useRiderAutosave'
import { useRiderStore } from '~/stores/rider'

const props = defineProps<{ kind: 'template' | 'attachment'; id: string }>()

const store = useRiderStore()
const { discard } = useRiderAutosave()

const conflicted = computed(() => store.conflicts.includes(`${props.kind}:${props.id}`))
const reloading = ref(false)
const error = ref('')

async function onReload(): Promise<void> {
  reloading.value = true
  error.value = ''
  try {
    await discard({ kind: props.kind, id: props.id })
  } catch {
    error.value = 'Could not reload the latest version. Check your connection and try again.'
  } finally {
    reloading.value = false
  }
}
</script>

<template>
  <div
    v-if="conflicted"
    role="alert"
    data-testid="rider-conflict-notice"
    class="glass-panel p-4 space-y-2"
    style="border-left:3px solid var(--color-error);"
  >
    <p class="text-xs tracking-terminal text-error uppercase font-terminal">CHANGED ELSEWHERE</p>
    <p class="text-sm text-on-surface">
      This {{ kind }} was saved from another tab or device, so your latest edits were not saved.
      Reload the latest version to keep working. This discards your unsaved edits.
    </p>
    <p v-if="error" class="text-xs text-error" data-testid="rider-conflict-error">{{ error }}</p>
    <button
      type="button"
      class="btn-hud"
      :disabled="reloading"
      data-testid="rider-conflict-reload"
      @click="onReload"
    >{{ reloading ? 'RELOADING…' : 'RELOAD LATEST (DISCARD MY EDITS)' }}</button>
  </div>
</template>
