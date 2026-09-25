import {
  LayoutDashboard,
  CalendarRange,
  ListChecks,
  ScanLine,
  Users,
  Megaphone,
  Handshake,
  Settings,
} from 'lucide-vue-next'
import type { KhNavItem } from '#kui/types/shell'

/**
 * Promoter primary navigation. Order follows the organiser's workflow:
 * plan the event, fill the lists, run the door, grow the audience,
 * promote, book artists (P4), configure.
 */
export const PROMOTER_NAV: KhNavItem[] = [
  { label: 'DASHBOARD', icon: LayoutDashboard, to: '/' },
  { label: 'EVENTS',    icon: CalendarRange,   to: '/events' },
  { label: 'GUESTS',    icon: ListChecks,      to: '/guests' },
  { label: 'DOOR',      icon: ScanLine,        to: '/door' },
  { label: 'AUDIENCE',  icon: Users,           to: '/audience' },
  { label: 'CAMPAIGNS', icon: Megaphone,       to: '/campaigns' },
  { label: 'BOOKINGS',  icon: Handshake,       to: '/bookings' },
  { label: 'SETTINGS',  icon: Settings,        to: '/settings' },
]

/** Mobile bottom bar: the five destinations used on the night. */
export const PROMOTER_BOTTOM_NAV: KhNavItem[] = [
  { label: 'DASH',   icon: LayoutDashboard, to: '/' },
  { label: 'EVENTS', icon: CalendarRange,   to: '/events' },
  { label: 'GUESTS', icon: ListChecks,      to: '/guests' },
  { label: 'DOOR',   icon: ScanLine,        to: '/door' },
  { label: 'PROMO',  icon: Megaphone,       to: '/campaigns' },
]
