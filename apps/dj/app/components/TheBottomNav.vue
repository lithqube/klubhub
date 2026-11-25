<script setup lang="ts">
import {
  LayoutDashboard,
  Layers,
  Send,
  FileText,
  CalendarDays,
  Banknote,
  ClipboardList,
  Megaphone,
} from 'lucide-vue-next'
import type { KhNavItem } from '#kui/types/shell'

const primaryNav: KhNavItem[] = [
  { label: 'DASH',   icon: LayoutDashboard, to: '/'          },
  { label: 'TRACKS', icon: Layers,           to: '/tracklist' },
  { label: 'SOCIAL', icon: Send,             to: '/social'    },
  { label: 'EPK',    icon: FileText,         to: '/epk'       },
  { label: 'GIGS',   icon: CalendarDays,     to: '/gigs'      },
]

// The sections the bar has no room for.
const moreNav: KhNavItem[] = [
  { label: 'OUTREACH', icon: Megaphone,     to: '/outreach' },
  { label: 'RIDER',    icon: ClipboardList, to: '/rider' },
  { label: 'FINANCE',  icon: Banknote,      to: '/finance' },
]
</script>

<template>
  <KhBottomNav :items="primaryNav" :more="moreNav" />
</template>
