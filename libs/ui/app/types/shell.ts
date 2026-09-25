import type { Component } from 'vue'

/** One entry in a product's primary navigation (sidebar, bottom bar). */
export interface KhNavItem {
  label: string
  icon: Component
  to: string
  /** Sidebar group heading (e.g. PLAN, RUN); items of one group are adjacent. */
  group?: string
  /** Small tag shown after the label, e.g. a roadmap phase ("P2"). */
  tag?: string
  /** Counter shown after the label (e.g. items needing attention). */
  count?: number
  /** 'error' renders the counter in the error tone. */
  countTone?: 'default' | 'error'
}

/** Active-route rule shared by every nav: '/' is exact, the rest match by prefix. */
export function isNavActive(currentPath: string, to: string): boolean {
  if (to === '/') return currentPath === '/'
  return currentPath.startsWith(to)
}
