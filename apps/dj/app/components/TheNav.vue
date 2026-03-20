<script setup lang="ts">
import { Layers, Bell, Settings } from 'lucide-vue-next'

const route = useRoute()

const navItems = [
  { label: 'TRACKLIST', icon: Layers, to: '/tracklist' },
  // Future modules added here as phases complete:
  // { label: 'SOCIAL', icon: Send, to: '/social' },
  // { label: 'GIGS', icon: Calendar, to: '/gigs' },
]
</script>

<template>
  <!-- Fixed sidebar: 280px width, full height -->
  <aside class="w-[280px] min-h-screen bg-surface-container-low flex flex-col flex-shrink-0">
    <!-- Logo area -->
    <div class="px-6 py-5 border-b border-outline-variant/20">
      <p class="font-command font-bold text-on-surface text-lg uppercase tracking-wide">
        KlubHub DJ
      </p>
      <div class="flex items-center gap-2 mt-1">
        <!-- Green status dot -->
        <span class="w-2 h-2 rounded-full bg-green-400" aria-hidden="true" />
        <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
          STATUS: SYNCED
        </span>
      </div>
    </div>

    <!-- Nav items -->
    <nav class="flex-1 px-4 py-4 space-y-1">
      <NuxtLink
        v-for="item in navItems"
        :key="item.to"
        :to="item.to"
        class="flex items-center gap-3 px-4 py-3 transition-all duration-200"
        :class="route.path.startsWith(item.to)
          ? 'nav-item-active'
          : 'nav-item-inactive text-tertiary hover:text-on-surface'"
      >
        <component :is="item.icon" class="w-4 h-4 flex-shrink-0" />
        <span class="font-terminal tracking-terminal text-xs uppercase">{{ item.label }}</span>
      </NuxtLink>
    </nav>

    <!-- Bottom: settings + bell -->
    <div class="px-4 py-4 border-t border-outline-variant/20 flex items-center gap-2">
      <button class="flex items-center gap-2 px-3 py-2 text-tertiary hover:text-on-surface transition-colors">
        <Bell class="w-4 h-4" />
      </button>
      <button class="flex items-center gap-2 px-3 py-2 text-tertiary hover:text-on-surface transition-colors">
        <Settings class="w-4 h-4" />
      </button>
    </div>
  </aside>
</template>
