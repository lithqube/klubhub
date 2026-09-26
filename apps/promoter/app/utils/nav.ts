import {
  LayoutDashboard,
  CalendarRange,
  MapPin,
  ListChecks,
  ScanLine,
  Users,
  Megaphone,
  Handshake,
  Settings,
  Radio,
} from 'lucide-vue-next'
import type { KhNavItem } from '#kui/types/shell'

/**
 * Promoter primary navigation, grouped by job (docs/design/promoter-p1-ux.md §3):
 * PLAN a night, RUN it, GROW the audience, DEAL with artists. Sections not
 * built yet carry their roadmap phase and open an honest placeholder.
 */
export const PROMOTER_NAV: KhNavItem[] = [
  { label: 'DASHBOARD', icon: LayoutDashboard, to: '/', group: 'PLAN' },
  { label: 'EVENTS',    icon: CalendarRange,   to: '/events', group: 'PLAN' },
  { label: 'VENUES',    icon: MapPin,          to: '/venues', group: 'PLAN' },
  { label: 'GUESTS',    icon: ListChecks,      to: '/guests', group: 'RUN', tag: 'P2' },
  { label: 'DOOR',      icon: ScanLine,        to: '/door', group: 'RUN', tag: 'P2' },
  { label: 'AUDIENCE',  icon: Users,           to: '/audience', group: 'GROW', tag: 'P3' },
  { label: 'CAMPAIGNS', icon: Megaphone,       to: '/campaigns', group: 'GROW', tag: 'P3' },
  { label: 'BOOKINGS',  icon: Handshake,       to: '/bookings', group: 'DEAL', tag: 'P4' },
  { label: 'SETTINGS',  icon: Settings,        to: '/settings' },
]

/** Mobile bottom bar: only working destinations (unbuilt sections live in MORE). */
export const PROMOTER_BOTTOM_NAV: KhNavItem[] = [
  { label: 'DASH',   icon: LayoutDashboard, to: '/' },
  { label: 'EVENTS', icon: CalendarRange,   to: '/events' },
  { label: 'NOW',    icon: Radio,           to: '/now' },
  { label: 'VENUES', icon: MapPin,          to: '/venues' },
]

/** Everything not in the bottom bar, for the MORE sheet. */
export const PROMOTER_MORE_NAV: KhNavItem[] = PROMOTER_NAV.filter(
  i => !PROMOTER_BOTTOM_NAV.some(b => b.to === i.to),
)
