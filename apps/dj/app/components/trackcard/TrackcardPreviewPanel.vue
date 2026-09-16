<script setup lang="ts">
// No props — pure layout wrapper with slot
</script>

<template>
  <!-- Panel background: bg-surface-container (opaque, NOT glass per mockup spec) -->
  <div class="bg-surface-container">
    <!-- Panel header: label left + live render indicator right -->
    <div class="flex justify-between items-center px-4 py-2 border-b border-dashed border-outline-variant/40">
      <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
        REALTIME_OUTPUT_PREVIEW
      </span>
      <span class="flex items-center gap-1.5">
        <span class="pulse-dot" aria-hidden="true" />
        <span class="font-terminal tracking-terminal text-primary text-xs uppercase">
          LIVE_RENDER
        </span>
      </span>
    </div>
    <!-- Preview content slot -->
    <div class="p-4">
      <slot />
    </div>
  </div>
</template>
