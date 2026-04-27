/**
 * useTheme — Kinetic HUD theme switcher
 *
 * Supports 3 modes:
 *   'dark'   → Kinetic HUD (radioactive cyan on obsidian)
 *   'system' → follows OS prefers-color-scheme
 *   'light'  → Daytime HUD (teal/violet on cool white)
 *
 * DOM data-theme is always 'dark' or 'light' — 'system' resolves at runtime.
 */

export type ThemeMode = 'dark' | 'system' | 'light'
type ResolvedTheme = 'dark' | 'light'

const STORAGE_KEY = 'klubhub-theme'

function resolveMode(mode: ThemeMode): ResolvedTheme {
  if (mode === 'system' && import.meta.client) {
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
  }
  return mode === 'light' ? 'light' : 'dark'
}

export const useTheme = () => {
  // mode: what the user explicitly chose (dark | system | light)
  const mode = useState<ThemeMode>('themeMode', () => 'dark')
  // theme: the resolved value applied to the DOM (dark | light)
  const theme = useState<ResolvedTheme>('theme', () => 'dark')

  const isDark = computed(() => theme.value === 'dark')
  const isLight = computed(() => theme.value === 'light')

  function applyTheme(t: ResolvedTheme) {
    theme.value = t
    if (import.meta.client) {
      document.documentElement.setAttribute('data-theme', t)
    }
  }

  function setTheme(m: ThemeMode) {
    mode.value = m
    if (import.meta.client) {
      localStorage.setItem(STORAGE_KEY, m)
    }
    applyTheme(resolveMode(m))
  }

  /** Legacy toggle: dark ↔ light (used by mobile header) */
  function toggleTheme() {
    setTheme(theme.value === 'dark' ? 'light' : 'dark')
  }

  function initTheme() {
    if (!import.meta.client) return
    const stored = localStorage.getItem(STORAGE_KEY) as ThemeMode | null
    const resolved = stored ?? 'dark'
    mode.value = resolved as ThemeMode

    // Listen for OS preference changes when in system mode
    const mq = window.matchMedia('(prefers-color-scheme: light)')
    mq.addEventListener('change', () => {
      if (mode.value === 'system') applyTheme(mq.matches ? 'light' : 'dark')
    })

    setTheme(mode.value)
  }

  return {
    theme: readonly(theme),
    mode: readonly(mode),
    isDark,
    isLight,
    setTheme,
    toggleTheme,
    initTheme,
  }
}
