<script setup lang="ts">
/**
 * TheMobileHeader — Mobile-only top brand bar
 *
 * UI/UX Laws applied:
 *   Law of Common Region — Brand + status grouped; theme toggle clearly separated
 *   Fitts's Law          — Theme toggle: min 44×44px tap area
 *   Hick's Law           — Only 2 elements visible: brand and one action (theme toggle)
 *   Aesthetic Usability  — Mirrors sidebar logo area, consistent brand voice
 *
 * Hidden on desktop (lg:hidden) — sidebar provides the same information.
 */
import { Sun, Moon } from 'lucide-vue-next'

const { isDark, toggleTheme } = useTheme()
</script>

<template>
  <header
    class="lg:hidden flex items-center justify-between px-4 py-3 bg-surface-container-low border-b border-outline-variant/20 flex-shrink-0"
    aria-label="KlubHub DJ brand bar"
  >
    <!-- Brand -->
    <div>
      <p class="font-command font-bold text-on-surface text-sm uppercase tracking-wide leading-none">
        KlubHub DJ
      </p>
      <div class="flex items-center gap-1.5 mt-0.5">
        <span class="w-1.5 h-1.5 rounded-full bg-green-400 flex-shrink-0" aria-hidden="true" />
        <span class="font-terminal tracking-terminal text-tertiary text-[9px] uppercase">
          STATUS: SYNCED
        </span>
      </div>
    </div>

    <!-- Theme toggle: Fitts's Law — min 44×44px target -->
    <button
      class="flex items-center justify-center w-11 h-11 text-tertiary hover:text-primary transition-colors duration-200"
      :aria-label="isDark ? 'Switch to Daytime HUD' : 'Switch to Kinetic HUD'"
      :aria-pressed="!isDark"
      @click="toggleTheme"
    >
      <component :is="isDark ? Sun : Moon" class="w-4 h-4" aria-hidden="true" />
    </button>
  </header>
</template>
