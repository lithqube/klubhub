<script setup lang="ts">
// No props — pure layout wrapper with slot
</script>

<template>
  <!-- Panel background: bg-surface-container (opaque, NOT glass per mockup spec) -->
  <div class="bg-surface-container">
    <!-- Panel header: label left + live render indicator right -->
    <div class="flex justify-between items-center px-4 py-2 border-b border-outline-variant/20">
      <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
        REALTIME_OUTPUT_PREVIEW
      </span>
      <span class="flex items-center gap-1.5">
        <!-- Red dot live indicator — rounded-full is intentional (tiny indicator circle) -->
        <span class="w-2 h-2 rounded-full bg-error" aria-hidden="true" />
        <span class="font-terminal tracking-terminal text-error text-xs uppercase">
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
