<script setup lang="ts">
const { initTheme, theme } = useTheme()

// Same theme bootstrap as KlubHub DJ: the inline script avoids a flash of the
// wrong theme; the reactive htmlAttrs keep data-theme in sync after hydration.
// nuxt-security adds a per-request nonce to this inline script (CSP).
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

useHead({ titleTemplate: (t?: string) => (t ? `${t} — KlubHub Promoter` : 'KlubHub Promoter') })

onMounted(() => initTheme())

function handleError(error: Error) {
  console.error('[KlubHub Promoter] Unhandled error:', error)
}
</script>

<template>
  <div class="hud-bg min-h-dvh font-data text-on-surface flex" style="position:relative;">
    <TheNav />

    <div class="flex-1 flex flex-col min-h-dvh overflow-hidden pb-16 lg:pb-0" style="position:relative;z-index:1;">
      <TheMobileHeader />

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

    <ClientOnly>
      <Toaster />
    </ClientOnly>

    <TheBottomNav />
  </div>
</template>
