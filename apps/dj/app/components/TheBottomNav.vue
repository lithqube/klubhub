<script setup lang="ts">
/**
 * TheBottomNav — Mobile-only bottom navigation
 *
 * UI/UX Laws applied:
 *   Jakob's Law       — Familiar bottom-nav pattern; users know how this works
 *   Fitts's Law       — min-h-[56px] per item, generous tap area
 *   Miller's Law      — Exactly 5 items (7±2 cognitive limit)
 *   Hick's Law        — Fewer choices than sidebar (no support/logout/settings at this level)
 *   Law of Proximity  — Icon + label grouped; active indicator visually attached to item
 *
 * Hidden on desktop (lg:hidden) — sidebar takes over.
 * Safe-area padding prevents content hiding behind gesture bar (iOS) and nav bar (Android).
 */
import {
  LayoutDashboard,
  Layers,
  Send,
  CalendarDays,
  Banknote,
} from 'lucide-vue-next'

const route = useRoute()

const primaryNav = [
  { label: 'DASH',    icon: LayoutDashboard, to: '/'          },
  { label: 'TRACKS',  icon: Layers,           to: '/tracklist' },
  { label: 'SOCIAL',  icon: Send,             to: '/social'    },
  { label: 'GIGS',    icon: CalendarDays,     to: '/gigs'      },
  { label: 'FINANCE', icon: Banknote,         to: '/finance'   },
]

function isActive(to: string) {
  if (to === '/') return route.path === '/'
  return route.path.startsWith(to)
}
</script>

<template>
  <nav
    class="fixed bottom-0 left-0 right-0 z-20 lg:hidden bg-surface-container-low border-t border-outline-variant/20"
    style="padding-bottom: env(safe-area-inset-bottom, 0px)"
    aria-label="Primary navigation"
  >
    <div class="flex items-stretch">
      <NuxtLink
        v-for="item in primaryNav"
        :key="item.to"
        :to="item.to"
        class="relative flex-1 flex flex-col items-center justify-center gap-1 py-3 min-h-[56px] transition-colors duration-200"
        :class="isActive(item.to)
          ? 'text-primary'
          : 'text-tertiary'"
        :aria-label="item.label"
        :aria-current="isActive(item.to) ? 'page' : undefined"
      >
        <!-- Active indicator: top accent bar (Gestalt continuity) -->
        <div
          class="absolute top-0 left-1/2 -translate-x-1/2 w-8 h-0.5 transition-all duration-200"
          :class="isActive(item.to) ? 'gradient-cta' : 'bg-transparent'"
          aria-hidden="true"
        />
        <component
          :is="item.icon"
          class="w-5 h-5 flex-shrink-0"
          aria-hidden="true"
        />
        <span class="font-terminal tracking-terminal text-[9px] uppercase leading-none">
          {{ item.label }}
        </span>
      </NuxtLink>
    </div>
  </nav>
</template>
