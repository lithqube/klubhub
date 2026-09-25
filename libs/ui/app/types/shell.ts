import type { Component } from 'vue'

/** One entry in a product's primary navigation (sidebar, bottom bar). */
export interface KhNavItem {
  label: string
  icon: Component
  to: string
}

/** Active-route rule shared by every nav: '/' is exact, the rest match by prefix. */
export function isNavActive(currentPath: string, to: string): boolean {
  if (to === '/') return currentPath === '/'
  return currentPath.startsWith(to)
}
