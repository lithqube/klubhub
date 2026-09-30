<script setup lang="ts">
// RiderPageHeader — title block + save status indicator with fade
// timer. Mirrors EpkPageHeader.vue.

import { computed, ref, watch } from 'vue'
import { useRiderStore } from '~/stores/rider'

const store = useRiderStore()
const saveStatus = computed(() => store.saveStatus)

const showSaved = ref(false)
let savedTimer: ReturnType<typeof setTimeout> | null = null

watch(saveStatus, (val) => {
  if (val === 'saved') {
    showSaved.value = true
    if (savedTimer) clearTimeout(savedTimer)
    savedTimer = setTimeout(() => { showSaved.value = false }, 2000)
  } else {
    showSaved.value = false
    if (savedTimer) clearTimeout(savedTimer)
  }
})
</script>

<template>
  <div data-testid="rider-page-header" class="flex items-start justify-between gap-4">
    <div>
      <h1 class="font-command font-bold text-on-surface text-2xl uppercase tracking-command">
        RIDER
      </h1>
      <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase mt-0.5">
        TEMPLATES & ATTACHMENTS
      </p>
    </div>
    <div data-testid="rider-save-status" class="flex items-center pt-1">
      <span
        v-if="saveStatus === 'saving'"
        class="font-terminal tracking-terminal text-tertiary text-xs uppercase"
      >SAVING…</span>
      <span
        v-else-if="saveStatus === 'saved' && showSaved"
        class="font-terminal tracking-terminal text-primary text-xs uppercase"
      >SAVED</span>
      <span
        v-else-if="saveStatus === 'error'"
        class="font-terminal tracking-terminal text-error text-xs uppercase"
      >SAVE FAILED</span>
    </div>
  </div>
</template>