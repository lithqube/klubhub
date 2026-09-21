<script setup lang="ts">
const { initTheme, theme } = useTheme()

/**
 * Reactive htmlAttrs — keeps data-theme in sync with the theme ref on every
 * navigation. Without this, Nuxt's head manager would reset the attribute
 * back to the SSR default ('dark') whenever it re-evaluates on route change.
 *
 * The inline script still runs first for FOUC prevention; this computed
 * binding takes over as the source of truth once Vue is hydrated.
 */
useHead(computed(() => ({
  htmlAttrs: { 'data-theme': theme.value },
  script: [
    {
      innerHTML: `(function(){
        var k='klubhub-theme';
        var s=localStorage.getItem(k);
        var os=window.matchMedia('(prefers-color-scheme: light)').matches?'light':'dark';
        document.documentElement.setAttribute('data-theme', s||os);
      })();`,
      tagPosition: 'head',
    },
  ],
})))

onMounted(() => initTheme())

// The app shell lives here rather than in a layout, so `layout: false` on a
// page cannot remove it. Screenshot targets under /render must be a bare
// canvas, otherwise Playwright captures the nav and status bar too.
const route = useRoute()
const isBareRoute = computed(() => route.path.startsWith('/render/'))

function handleError(error: Error) {
  console.error('[KlubHub] Unhandled error:', error)
}
</script>

<template>
  <!--
    Root layout: horizontal flex (sidebar + main column).
    min-h-dvh: avoids 100vh mobile-browser-chrome bug (UX law: viewport-units).
  -->
  <NuxtPage v-if="isBareRoute" />

  <div v-else class="hud-bg min-h-dvh font-data text-on-surface flex" style="position:relative;">

    <!-- Desktop sidebar (hidden on mobile via TheNav's own hidden lg:flex) -->
    <TheNav />

    <!-- Main column: full-width on mobile, fills remainder on desktop -->
    <div class="flex-1 flex flex-col min-h-dvh overflow-hidden pb-16 lg:pb-10" style="position:relative;z-index:1;">
      <!--
        pb-16 (64px)  → clears TheBottomNav on mobile   (Fitts's Law: content not under tap targets)
        lg:pb-10      → clears TheStatusBar on desktop
      -->

      <!-- Mobile brand bar: logo + status + theme toggle (lg:hidden) -->
      <TheMobileHeader />

      <!-- Page content with error boundary -->
      <NuxtErrorBoundary @error="handleError">
        <NuxtPage />
        <template #error="{ error, clearError }">
          <div class="flex items-center justify-center min-h-[60vh] p-4 md:p-8">
            <div class="glass-panel-heavy luminous-threshold p-6 md:p-8 max-w-lg w-full space-y-4">
              <h1 class="font-command tracking-command text-error text-xl md:text-2xl font-bold uppercase">
                SYSTEM ERROR
              </h1>
              <p class="font-terminal tracking-terminal text-on-surface-variant text-xs uppercase leading-relaxed">
                {{ error.message || 'An unexpected error occurred. Please reinitialize.' }}
              </p>
              <!-- Fitts's Law: full-width button, min-h-[44px] -->
              <button
                class="gradient-cta text-on-primary px-6 py-3 min-h-[44px] font-terminal tracking-terminal text-xs uppercase font-semibold w-full hover:shadow-glow-primary-strong transition-shadow duration-200"
                @click="clearError()"
              >
                REINITIALIZE
              </button>
            </div>
          </div>
        </template>
      </NuxtErrorBoundary>
    </div>

    <!-- Toast notifications — ClientOnly prevents SSR hydration mismatch -->
    <ClientOnly>
      <Toaster />
    </ClientOnly>

    <!-- Desktop status bar (hidden on mobile — Hick's Law: reduce noise) -->
    <TheStatusBar />

    <!-- Mobile bottom navigation (hidden on desktop — Jakob's Law: familiar mobile pattern) -->
    <TheBottomNav />

  </div>
</template>
