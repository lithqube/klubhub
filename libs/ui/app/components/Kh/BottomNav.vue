<script setup lang="ts">
import { MoreHorizontal } from 'lucide-vue-next'
import Sheet from '#kui/components/ui/sheet/Sheet.vue'
import { isNavActive, type KhNavItem } from '#kui/types/shell'

/**
 * KhBottomNav — mobile bottom navigation shared by every KlubHub product.
 * Destinations that do not fit the bar go in `more`: a MORE button opens them
 * in a sheet, so every section stays reachable on a phone.
 */
const props = withDefaults(defineProps<{ items: KhNavItem[], more?: KhNavItem[] }>(), { more: () => [] })

const route = useRoute()
const open = ref(false)
watch(() => route.fullPath, () => { open.value = false })

function isActive(to: string) {
  return isNavActive(route.path, to)
}
const moreActive = computed(() => props.more.some(item => isActive(item.to)))
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
      <button
        v-if="props.more.length"
        type="button"
        style="position:relative;flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:4px;padding:10px 4px;min-height:56px;background:none;border:0;cursor:pointer;"
        :style="moreActive ? 'color:var(--color-primary);' : 'color:var(--color-tertiary);'"
        :aria-expanded="open"
        aria-controls="more-sheet"
        @click="open = true"
      >
        <MoreHorizontal style="width:18px;height:18px;" aria-hidden="true" />
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;line-height:1;">MORE</span>
      </button>
    </div>
  </nav>
  <Sheet v-if="props.more.length" v-model:open="open" side="bottom">
    <nav id="more-sheet" aria-label="More sections" style="padding:12px 4px 24px;">
      <div class="section-lbl" style="padding:0 12px 8px;">MORE</div>
      <NuxtLink
        v-for="item in props.more"
        :key="item.to"
        :to="item.to"
        style="display:flex;align-items:center;gap:12px;min-height:56px;padding:0 16px;font-family:var(--font-terminal);font-size:11px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-on-surface);text-decoration:none;"
        :aria-current="isActive(item.to) ? 'page' : undefined"
      >
        <component :is="item.icon" style="width:18px;height:18px;color:var(--color-tertiary);" aria-hidden="true" />
        <span>{{ item.label }}</span>
        <span v-if="item.tag" class="data-frag" style="margin-left:auto;font-size:8px;">{{ item.tag }}</span>
      </NuxtLink>
    </nav>
  </Sheet>
</template>
