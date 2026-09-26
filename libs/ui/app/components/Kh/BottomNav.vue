<script setup lang="ts">
import { isNavActive, type KhNavItem } from '#kui/types/shell'

/** KhBottomNav — mobile bottom navigation shared by every KlubHub product. */
const props = defineProps<{ items: KhNavItem[] }>()

const route = useRoute()

function isActive(to: string) {
  return isNavActive(route.path, to)
}
</script>

<template>
  <nav
    class="lg:hidden bottom-nav-bar"
    style="position:fixed;bottom:0;left:0;right:0;z-index:20;padding-bottom:env(safe-area-inset-bottom, 0px);"
    aria-label="Primary navigation"
  >
    <div style="display:flex;align-items:stretch;">
      <NuxtLink
        v-for="item in props.items"
        :key="item.to"
        :to="item.to"
        style="position:relative;flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:4px;padding:10px 4px;min-height:56px;text-decoration:none;transition:color .15s;"
        :style="isActive(item.to) ? 'color:var(--color-primary);' : 'color:var(--color-tertiary);'"
        :aria-label="item.label"
        :aria-current="isActive(item.to) ? 'page' : undefined"
      >
        <!-- Top accent line on active -->
        <div
          style="position:absolute;top:0;left:50%;transform:translateX(-50%);width:32px;height:2px;transition:all .2s;"
          :style="isActive(item.to) ? 'background:var(--color-primary);box-shadow:0 0 8px rgba(150,248,255,.5);' : 'background:transparent;'"
          aria-hidden="true"
        />
        <component
          :is="item.icon"
          style="width:18px;height:18px;flex-shrink:0;"
          aria-hidden="true"
        />
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;line-height:1;">
          {{ item.label }}
        </span>
      </NuxtLink>
      <!-- Extra actions (e.g. a MORE sheet trigger) supplied by the product. -->
      <slot name="after" />
    </div>
  </nav>
</template>
