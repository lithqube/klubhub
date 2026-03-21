<script setup lang="ts">
import { computed } from 'vue'
import { useSocialStore } from '~/stores/social'

const store = useSocialStore()

const nextSlotDisplay = computed(() => {
  const now = new Date()
  const next = new Date(now)
  next.setHours(now.getHours() + 1, 0, 0, 0)
  return next.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).toUpperCase()
})
</script>

<template>
  <div
    data-testid="next-slot-card"
    class="border-2 border-dashed border-outline-variant/40 bg-transparent min-h-[240px] flex flex-col items-center justify-center gap-3 cursor-pointer hover:border-primary/40 transition-colors"
    @click="store.openComposePanel()"
  >
    <span class="text-2xl text-on-surface-dim">+</span>
    <div class="text-center px-4">
      <p class="font-terminal tracking-terminal text-xs text-primary uppercase">
        {{ nextSlotDisplay }}
      </p>
      <p class="font-terminal tracking-terminal text-xs text-on-surface-dim uppercase mt-1">
        SCHEDULE NEW ACTIVITY
      </p>
    </div>
  </div>
</template>
