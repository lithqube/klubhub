<script setup lang="ts">
import {
  LayoutDashboard,
  Layers,
  Send,
  FileText,
  CalendarDays,
  Banknote,
  LogOut,
  Moon,
  Monitor,
  Sun,
} from 'lucide-vue-next'
import type { ThemeMode } from '~/composables/useTheme'

const route = useRoute()
const { mode, setTheme } = useTheme()

const primaryNav = [
  { label: 'DASHBOARD', icon: LayoutDashboard, to: '/' },
  { label: 'TRACKLIST', icon: Layers,           to: '/tracklist' },
  { label: 'SOCIAL',    icon: Send,             to: '/social' },
  { label: 'EPK',       icon: FileText,         to: '/epk' },
  { label: 'GIGS',      icon: CalendarDays,     to: '/gigs' },
  { label: 'FINANCE',   icon: Banknote,         to: '/finance' },
]

const themeOptions: { id: ThemeMode; label: string; icon: typeof Moon }[] = [
  { id: 'dark',   label: 'KINETIC', icon: Moon },
  { id: 'system', label: 'SYSTEM',  icon: Monitor },
  { id: 'light',  label: 'DAYTIME', icon: Sun },
]

function isActive(to: string) {
  if (to === '/') return route.path === '/'
  return route.path.startsWith(to)
}
</script>

<template>
  <!-- Sidebar: 220px, full height, desktop only -->
  <aside
    class="hidden lg:flex flex-col flex-shrink-0 border-r border-primary/[0.08]"
    style="width:220px;min-width:220px;height:100vh;background:var(--color-surface-container-low);position:relative;z-index:10;"
  >

    <!-- Logo area with bracket-box corner decoration -->
    <div
      class="bracket-box border-b border-primary/[0.08]"
      style="padding:16px 18px;"
    >
      <div style="font-family:var(--font-command);font-size:14px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;text-transform:uppercase;">
        KlubHub DJ
      </div>
      <div style="display:flex;align-items:center;gap:6px;margin-top:5px;">
        <div class="pulse-dot" aria-hidden="true" />
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);">
          SYSTEM: ONLINE
        </span>
      </div>
    </div>

    <!-- Primary nav -->
    <nav
      style="flex:1;padding:8px 10px;display:flex;flex-direction:column;gap:1px;overflow-y:auto;"
      aria-label="Primary navigation"
    >
      <NuxtLink
        v-for="item in primaryNav"
        :key="item.to"
        :to="item.to"
        :aria-current="isActive(item.to) ? 'page' : undefined"
        style="display:flex;align-items:center;gap:10px;padding:10px 12px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;cursor:pointer;transition:all .15s;position:relative;text-decoration:none;"
        :class="isActive(item.to) ? 'nav-item-active' : 'nav-item-inactive'"
        :style="isActive(item.to)
          ? 'color:var(--color-primary);box-shadow:var(--shadow-glow-primary);'
          : 'color:var(--color-tertiary);'"
      >
        <component
          :is="item.icon"
          style="width:13px;height:13px;stroke:currentColor;fill:none;stroke-width:2;flex-shrink:0;"
          aria-hidden="true"
        />
        {{ item.label }}
      </NuxtLink>
    </nav>

    <!-- Footer: theme switcher + logout -->
    <div class="border-t border-primary/[0.08]">

      <!-- 3-way theme segmented control -->
      <div class="border-b border-primary/[0.08]" style="padding:10px 12px;">
        <div style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);margin-bottom:7px;">
          DISPLAY MODE
        </div>
        <div class="theme-seg">
          <button
            v-for="opt in themeOptions"
            :key="opt.id"
            class="theme-opt"
            :class="{ active: mode === opt.id }"
            :aria-pressed="mode === opt.id"
            :aria-label="`Switch to ${opt.label} mode`"
            @click="setTheme(opt.id)"
          >
            <component
              :is="opt.icon"
              style="width:12px;height:12px;stroke:currentColor;fill:none;stroke-width:2;flex-shrink:0;"
              aria-hidden="true"
            />
            {{ opt.label }}
          </button>
        </div>
      </div>

      <!-- Logout -->
      <button
        class="border-b border-dashed border-primary/[0.12]"
        style="display:flex;align-items:center;gap:10px;padding:10px 12px;width:100%;font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);background:transparent;border-left:none;border-right:none;border-top:none;cursor:pointer;transition:all .15s;"
        aria-label="Log out"
        @mouseenter="($event.currentTarget as HTMLElement).style.color = 'var(--color-error)'"
        @mouseleave="($event.currentTarget as HTMLElement).style.color = 'var(--color-tertiary)'"
      >
        <LogOut style="width:13px;height:13px;stroke:currentColor;fill:none;stroke-width:2;flex-shrink:0;" aria-hidden="true" />
        LOGOUT
      </button>
    </div>

  </aside>
</template>
