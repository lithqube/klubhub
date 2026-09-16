<script setup lang="ts">
import { ref, watch } from 'vue'
import { useEpkStore } from '~/stores/epk'

const store = useEpkStore()

// saveStatus is a plain ref on the store (not storeToRefs — direct store property access pattern per EPK convention)
const saveStatus = computed(() => store.saveStatus)

// Fade "SAVED" indicator after 2s
const showSaved = ref(false)
let savedTimer: ReturnType<typeof setTimeout> | null = null

watch(saveStatus, (val) => {
  if (val === 'saved') {
    showSaved.value = true
    if (savedTimer) clearTimeout(savedTimer)
    savedTimer = setTimeout(() => {
      showSaved.value = false
    }, 2000)
  } else {
    showSaved.value = false
    if (savedTimer) clearTimeout(savedTimer)
  }
})
</script>

<template>
  <div data-testid="epk-page-header" class="flex items-start justify-between gap-4">
    <!-- Title block -->
    <div>
      <h1 class="font-command font-bold text-on-surface text-2xl uppercase tracking-command">
        EPK
      </h1>
      <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase mt-0.5">
        ELECTRONIC PRESS KIT
      </p>
    </div>

    <!-- Save status indicator -->
    <div data-testid="save-status-indicator" class="flex items-center pt-1">
      <span
        v-if="saveStatus === 'saving'"
        class="font-terminal tracking-terminal text-tertiary text-xs uppercase"
      >
        SAVING...
      </span>
      <span
        v-else-if="saveStatus === 'saved' && showSaved"
        class="font-terminal tracking-terminal text-primary text-xs uppercase"
      >
        SAVED
      </span>
      <span
        v-else-if="saveStatus === 'error'"
        class="font-terminal tracking-terminal text-error text-xs uppercase"
      >
        SAVE FAILED
      </span>
    </div>
  </div>
</template>
