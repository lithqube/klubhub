<script setup lang="ts">
function handleError(error: Error) {
  console.error('[KlubHub] Unhandled error:', error)
}
</script>

<template>
  <div class="bg-surface min-h-screen font-data text-on-surface flex">
    <!-- Sidebar navigation (fixed 280px) -->
    <TheNav />

    <!-- Main content area: grows to fill remaining width, padded bottom for status bar -->
    <div class="flex-1 flex flex-col min-h-screen pb-10">
      <!-- Page content with error boundary -->
      <NuxtErrorBoundary @error="handleError">
        <NuxtPage />
        <template #error="{ error, clearError }">
          <div class="flex items-center justify-center min-h-[60vh] p-8">
            <div class="glass-panel-heavy luminous-threshold p-8 max-w-lg w-full space-y-4">
              <h1 class="font-command tracking-command text-error text-2xl font-bold uppercase">
                SYSTEM ERROR
              </h1>
              <p class="font-terminal tracking-terminal text-on-surface-variant text-xs uppercase leading-relaxed">
                {{ error.message || 'An unexpected error occurred. Please reinitialize.' }}
              </p>
              <button
                class="gradient-cta text-on-primary px-6 py-3 font-terminal tracking-terminal text-xs uppercase font-semibold w-full hover:shadow-glow-primary-strong transition-shadow duration-200"
                @click="clearError()"
              >
                REINITIALIZE
              </button>
            </div>
          </div>
        </template>
      </NuxtErrorBoundary>
    </div>

    <!-- Toast notification system — ClientOnly prevents SSR hydration mismatch -->
    <ClientOnly>
      <Toaster />
    </ClientOnly>

    <!-- Bottom status bar (fixed, full width) -->
    <TheStatusBar />
  </div>
</template>
