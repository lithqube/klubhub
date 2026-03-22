<script setup lang="ts">
import {
  LayoutDashboard,
  Layers,
  Send,
  FileText,
  CalendarDays,
  Banknote,
  LifeBuoy,
  LogOut,
  Sun,
  Moon,
} from 'lucide-vue-next'

const route = useRoute()
const { isDark, toggleTheme } = useTheme()

const primaryNav = [
  { label: 'DASHBOARD',  icon: LayoutDashboard, to: '/' },
  { label: 'TRACKLIST',  icon: Layers,           to: '/tracklist' },
  { label: 'SOCIAL',     icon: Send,             to: '/social' },
  { label: 'EPK',        icon: FileText,         to: '/epk' },
  { label: 'GIGS',       icon: CalendarDays,     to: '/gigs' },
  { label: 'FINANCE',    icon: Banknote,         to: '/finance' },
]

function isActive(to: string) {
  if (to === '/') return route.path === '/'
  return route.path.startsWith(to)
}
</script>

<template>
  <!-- Fixed sidebar: 280px width, full height — desktop only (lg+) -->
  <!-- Hidden on mobile: TheBottomNav + TheMobileHeader take over -->
  <aside class="w-[280px] min-h-dvh bg-surface-container-low hidden lg:flex flex-col flex-shrink-0">

    <!-- Logo area -->
    <div class="px-6 py-5 border-b border-outline-variant/20">
      <p class="font-command font-bold text-on-surface text-lg uppercase tracking-wide">
        KlubHub DJ
      </p>
      <div class="flex items-center gap-2 mt-1">
        <span class="w-2 h-2 rounded-full bg-green-400" aria-hidden="true" />
        <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
          STATUS: SYNCED
        </span>
      </div>
    </div>

    <!-- Primary nav -->
    <nav class="flex-1 px-4 py-4 space-y-1" aria-label="Primary navigation">
      <NuxtLink
        v-for="item in primaryNav"
        :key="item.to"
        :to="item.to"
        class="flex items-center gap-3 px-4 py-3 transition-all duration-200"
        :class="isActive(item.to)
          ? 'nav-item-active'
          : 'nav-item-inactive text-tertiary hover:text-on-surface'"
      >
        <component :is="item.icon" class="w-4 h-4 flex-shrink-0" />
        <span class="font-terminal tracking-terminal text-xs uppercase">{{ item.label }}</span>
      </NuxtLink>
    </nav>

    <!-- Bottom: theme toggle + support + logout -->
    <div class="px-4 py-4 border-t border-outline-variant/20 space-y-1">

      <!-- Theme toggle: Daytime HUD ↔ Kinetic HUD -->
      <button
        class="w-full flex items-center gap-3 px-4 py-3 nav-item-inactive text-tertiary hover:text-primary transition-colors duration-200"
        :aria-label="isDark ? 'Switch to Daytime HUD (light mode)' : 'Switch to Kinetic HUD (dark mode)'"
        :aria-pressed="!isDark"
        @click="toggleTheme"
      >
        <component :is="isDark ? Sun : Moon" class="w-4 h-4 flex-shrink-0" />
        <span class="font-terminal tracking-terminal text-xs uppercase">
          {{ isDark ? 'DAYTIME HUD' : 'KINETIC HUD' }}
        </span>
      </button>

      <button
        class="w-full flex items-center gap-3 px-4 py-3 nav-item-inactive text-tertiary hover:text-on-surface transition-colors"
        aria-label="Support"
      >
        <LifeBuoy class="w-4 h-4 flex-shrink-0" />
        <span class="font-terminal tracking-terminal text-xs uppercase">SUPPORT</span>
      </button>
      <button
        class="w-full flex items-center gap-3 px-4 py-3 nav-item-inactive text-tertiary hover:text-error transition-colors"
        aria-label="Log out"
      >
        <LogOut class="w-4 h-4 flex-shrink-0" />
        <span class="font-terminal tracking-terminal text-xs uppercase">LOGOUT</span>
      </button>
    </div>

  </aside>
</template>
