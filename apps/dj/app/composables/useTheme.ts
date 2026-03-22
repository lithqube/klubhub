/**
 * useTheme — Kinetic HUD theme switcher
 *
 * Manages dark ("The Kinetic HUD") / light ("Daytime HUD") theme state.
 *
 * - Reads user preference from localStorage on first load
 * - Falls back to OS prefers-color-scheme
 * - Applies `data-theme` attribute on <html> (drives all CSS variable overrides)
 * - Reactive via Nuxt's useState (shared across components, SSR-safe)
 */

type Theme = 'dark' | 'light'

const STORAGE_KEY = 'klubhub-theme'

export const useTheme = () => {
  // Shared state — initialized to dark (the default HUD mode)
  const theme = useState<Theme>('theme', () => 'dark')

  const isDark = computed(() => theme.value === 'dark')
  const isLight = computed(() => theme.value === 'light')

  /** Apply a theme: updates state, DOM attribute, and localStorage */
  function setTheme(t: Theme) {
    theme.value = t
    if (import.meta.client) {
      document.documentElement.setAttribute('data-theme', t)
      localStorage.setItem(STORAGE_KEY, t)
    }
  }

  /** Toggle between dark and light */
  function toggleTheme() {
    setTheme(theme.value === 'dark' ? 'light' : 'dark')
  }

  /**
   * Initialize theme from localStorage or OS preference.
   * Call once in app.vue onMounted — the inline head script handles
   * FOUC prevention, this call just syncs Nuxt reactive state.
   */
  function initTheme() {
    if (!import.meta.client) return
    const stored = localStorage.getItem(STORAGE_KEY) as Theme | null
    const osPrefers: Theme = window.matchMedia('(prefers-color-scheme: light)').matches
      ? 'light'
      : 'dark'
    setTheme(stored ?? osPrefers)
  }

  return {
    theme: readonly(theme),
    isDark,
    isLight,
    setTheme,
    toggleTheme,
    initTheme,
  }
}
