<script setup lang="ts">
import {
  LayoutDashboard,
  Layers,
  Send,
  FileText,
  CalendarDays,
  Banknote,
} from 'lucide-vue-next'
import type { KhNavItem } from '#kui/types/shell'

// The app has no user authentication yet, so there is nothing to log out of;
// KhSideNav keeps the logout row hidden until a product opts in.
const primaryNav: KhNavItem[] = [
  { label: 'DASHBOARD', icon: LayoutDashboard, to: '/' },
  { label: 'TRACKLIST', icon: Layers,           to: '/tracklist' },
  { label: 'SOCIAL',    icon: Send,             to: '/social' },
  { label: 'EPK',       icon: FileText,         to: '/epk' },
  { label: 'GIGS',      icon: CalendarDays,     to: '/gigs' },
  { label: 'FINANCE',   icon: Banknote,         to: '/finance' },
]
</script>

<template>
  <KhSideNav brand="KlubHub DJ" :items="primaryNav" />
</template>
