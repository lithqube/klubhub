<script setup lang="ts">
import {
  LogOut,
  Moon,
  Monitor,
  Sun,
  PanelLeftClose,
  PanelLeftOpen,
} from 'lucide-vue-next'
import type { ThemeMode } from '#kui/composables/useTheme'
import { isNavActive, type KhNavItem } from '#kui/types/shell'

/**
 * KhSideNav — the desktop sidebar shared by every KlubHub product.
 * Products pass their brand and primary nav; the theme switcher and the
 * collapse rail are the same everywhere.
 */
const props = withDefaults(defineProps<{
  brand: string
  items: KhNavItem[]
  monogram?: string
  status?: string
  /** Show the logout row (products without auth leave it off). */
  showLogout?: boolean
}>(), {
  monogram: 'KH',
  status: 'SYSTEM: ONLINE',
  showLogout: false,
})

const emit = defineEmits<{ logout: [] }>()

const route = useRoute()
const { mode, setTheme } = useTheme()
const { collapsed, toggle } = useSidebar()

const themeOptions: { id: ThemeMode; label: string; icon: typeof Moon }[] = [
  { id: 'dark',   label: 'KINETIC', icon: Moon },
  { id: 'system', label: 'SYSTEM',  icon: Monitor },
  { id: 'light',  label: 'DAYTIME', icon: Sun },
]

function isActive(to: string) {
  return isNavActive(route.path, to)
}
</script>

<template>
  <!--
    Sidebar: 220px, or a 56px icon rail when collapsed; full height and pinned
    (sticky) so it stays put while a long page scrolls; desktop only.
    The bottom padding keeps the footer (theme switcher, toggle) clear of the
    fixed status bar. In the rail the labels stay in the DOM as sr-only text, so
    every control keeps its accessible name; sighted users get a title tooltip.
  -->
  <aside
    id="app-sidebar"
    class="hidden lg:flex flex-col flex-shrink-0 border-r border-primary/[0.08]"
    :style="{ width: collapsed ? '56px' : '220px', minWidth: collapsed ? '56px' : '220px' }"
    style="height:100vh;padding-bottom:var(--status-bar-h);background:var(--color-surface-container-low);position:sticky;top:0;align-self:flex-start;z-index:10;overflow:hidden;transition:width .2s,min-width .2s;"
  >

    <!-- Header: brand + status. The rail keeps the corner brackets and shows a
         monogram over the status dot. -->
    <div
      class="bracket-box border-b border-primary/[0.08]"
      :style="collapsed
        ? 'padding:14px 0;display:flex;flex-direction:column;align-items:center;gap:8px;'
        : 'padding:16px 18px;'"
    >
      <template v-if="!collapsed">
        <div style="font-family:var(--font-command);font-size:14px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;text-transform:uppercase;white-space:nowrap;">
          {{ props.brand }}
        </div>
        <div style="display:flex;align-items:center;gap:6px;margin-top:5px;">
          <div class="pulse-dot" aria-hidden="true" />
          <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);white-space:nowrap;">
            {{ props.status }}
          </span>
        </div>
      </template>
      <template v-else>
        <div
          :title="props.brand"
          style="font-family:var(--font-command);font-size:13px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;"
        >
          {{ props.monogram }}
        </div>
        <div class="pulse-dot" role="img" :aria-label="props.status" :title="props.status" />
      </template>
    </div>

    <!-- Primary nav -->
    <nav
      style="flex:1;display:flex;flex-direction:column;gap:1px;overflow-y:auto;"
      :style="collapsed ? 'padding:8px;' : 'padding:8px 10px;'"
      aria-label="Primary navigation"
    >
      <NuxtLink
        v-for="item in props.items"
        :key="item.to"
        :to="item.to"
        :aria-current="isActive(item.to) ? 'page' : undefined"
        :title="collapsed ? item.label : undefined"
        style="display:flex;align-items:center;gap:10px;padding:10px 12px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;cursor:pointer;transition:all .15s;position:relative;text-decoration:none;"
        :class="isActive(item.to) ? 'nav-item-active' : 'nav-item-inactive'"
        :style="[
          isActive(item.to)
            ? 'color:var(--color-primary);box-shadow:var(--shadow-glow-primary);'
            : 'color:var(--color-tertiary);',
          collapsed ? 'justify-content:center;gap:0;padding:10px 0;' : '',
        ]"
      >
        <component
          :is="item.icon"
          style="width:13px;height:13px;stroke:currentColor;fill:none;stroke-width:2;flex-shrink:0;"
          aria-hidden="true"
        />
        <span :class="{ 'sr-only': collapsed }">{{ item.label }}</span>
      </NuxtLink>
    </nav>

    <!-- Footer: theme switcher + logout -->
    <div class="border-t border-primary/[0.08]">

      <!-- 3-way theme segmented control (a column of icons in the rail) -->
      <div class="border-b border-primary/[0.08]" :style="collapsed ? 'padding:8px;' : 'padding:10px 12px;'">
        <div
          :class="{ 'sr-only': collapsed }"
          style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);margin-bottom:7px;"
        >
          DISPLAY MODE
        </div>
        <div class="theme-seg" :style="collapsed ? 'flex-direction:column;' : ''">
          <button
            v-for="opt in themeOptions"
            :key="opt.id"
            class="theme-opt"
            :class="{ active: mode === opt.id }"
            :aria-pressed="mode === opt.id"
            :aria-label="`Switch to ${opt.label} mode`"
            :title="collapsed ? opt.label : undefined"
            :style="collapsed ? 'min-height:32px;padding:6px 0;' : ''"
            @click="setTheme(opt.id)"
          >
            <component
              :is="opt.icon"
              style="width:12px;height:12px;stroke:currentColor;fill:none;stroke-width:2;flex-shrink:0;"
              aria-hidden="true"
            />
            <span :class="{ 'sr-only': collapsed }">{{ opt.label }}</span>
          </button>
        </div>
      </div>

      <!-- Logout: only for products with authentication -->
      <button
        v-if="props.showLogout"
        class="border-b border-dashed border-primary/[0.12]"
        style="display:flex;align-items:center;gap:10px;padding:10px 12px;width:100%;font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);background:transparent;border-left:none;border-right:none;border-top:none;cursor:pointer;transition:all .15s;"
        :style="collapsed ? 'justify-content:center;gap:0;padding:10px 0;' : ''"
        aria-label="Log out"
        :title="collapsed ? 'LOGOUT' : undefined"
        @mouseenter="($event.currentTarget as HTMLElement).style.color = 'var(--color-error)'"
        @mouseleave="($event.currentTarget as HTMLElement).style.color = 'var(--color-tertiary)'"
        @click="emit('logout')"
      >
        <LogOut style="width:13px;height:13px;stroke:currentColor;fill:none;stroke-width:2;flex-shrink:0;" aria-hidden="true" />
        <span :class="{ 'sr-only': collapsed }">LOGOUT</span>
      </button>

      <!-- Collapse toggle: the last row of the sidebar -->
      <button
        type="button"
        class="side-nav-action"
        :style="collapsed ? 'justify-content:center;gap:0;padding:10px 0;' : ''"
        :aria-label="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        :aria-expanded="!collapsed"
        aria-controls="app-sidebar"
        :title="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        @click="toggle"
      >
        <component
          :is="collapsed ? PanelLeftOpen : PanelLeftClose"
          style="width:13px;height:13px;stroke:currentColor;fill:none;stroke-width:2;flex-shrink:0;"
          aria-hidden="true"
        />
        <span :class="{ 'sr-only': collapsed }">{{ collapsed ? 'EXPAND' : 'COLLAPSE' }}</span>
      </button>
    </div>

  </aside>
</template>
