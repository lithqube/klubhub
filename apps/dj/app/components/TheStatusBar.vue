<script setup lang="ts">
// Status items: hardcoded for Phase 1.5; future phases will pass dynamic data via props or store
const statusItems = [
  { label: 'API_LATENCY', value: '-- ms' },
  { label: 'STORAGE', value: 'NOMINAL' },
  { label: 'SYNC_STATUS', value: 'IDLE' },
]
</script>

<template>
  <!-- Fixed bottom bar: desktop only — Hick's Law: hide technical metrics on mobile -->
  <!-- TheBottomNav replaces this area on mobile -->
  <footer class="fixed bottom-0 left-0 right-0 bg-surface-container-high border-t border-outline-variant/20 z-10 hidden lg:block">
    <div class="flex items-center gap-8 px-6 py-2">
      <div
        v-for="item in statusItems"
        :key="item.label"
        class="flex flex-col items-center min-w-[80px]"
      >
        <span class="font-terminal tracking-terminal text-tertiary text-[8px] uppercase">
          {{ item.label }}
        </span>
        <span class="font-command text-on-surface font-bold text-sm tabular-nums">
          {{ item.value }}
        </span>
      </div>
      <!-- Right-align: version tag -->
      <div class="ml-auto">
        <span class="font-terminal tracking-terminal text-tertiary text-[8px] uppercase">
          KLUBHUB_DJ v1.0
        </span>
      </div>
    </div>
  </footer>
</template>
