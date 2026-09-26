<script setup lang="ts">
import {
  LayoutDashboard,
  Layers,
  Send,
  FileText,
  CalendarDays,
} from 'lucide-vue-next'
import type { KhNavItem } from '#kui/types/shell'

const primaryNav: KhNavItem[] = [
  { label: 'DASH',   icon: LayoutDashboard, to: '/'          },
  { label: 'TRACKS', icon: Layers,           to: '/tracklist' },
  { label: 'SOCIAL', icon: Send,             to: '/social'    },
  { label: 'EPK',    icon: FileText,         to: '/epk'       },
  { label: 'GIGS',   icon: CalendarDays,     to: '/gigs'      },
]
</script>

<template>
  <KhBottomNav :items="primaryNav" />
</template>
