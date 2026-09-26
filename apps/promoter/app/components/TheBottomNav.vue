<script setup lang="ts">
import { MoreHorizontal } from 'lucide-vue-next'
import { PROMOTER_BOTTOM_NAV, PROMOTER_MORE_NAV } from '~/utils/nav'

const open = ref(false)
const route = useRoute()
watch(() => route.fullPath, () => { open.value = false })
</script>

<template>
  <KhBottomNav :items="PROMOTER_BOTTOM_NAV">
    <template #after>
      <button
        type="button"
        style="position:relative;flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:4px;padding:10px 4px;min-height:56px;background:none;border:0;color:var(--color-tertiary);cursor:pointer;"
        :aria-expanded="open"
        aria-controls="more-sheet"
        @click="open = true"
      >
        <MoreHorizontal style="width:18px;height:18px;" aria-hidden="true" />
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;line-height:1;">MORE</span>
      </button>
    </template>
  </KhBottomNav>
  <Sheet v-model:open="open" side="bottom">
    <nav id="more-sheet" aria-label="More sections" style="padding:12px 4px 24px;">
      <div class="section-lbl" style="padding:0 12px 8px;">MORE</div>
      <NuxtLink
        v-for="item in PROMOTER_MORE_NAV"
        :key="item.to"
        :to="item.to"
        style="display:flex;align-items:center;gap:12px;min-height:56px;padding:0 16px;font-family:var(--font-terminal);font-size:11px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-on-surface);text-decoration:none;"
      >
        <component :is="item.icon" style="width:18px;height:18px;color:var(--color-tertiary);" aria-hidden="true" />
        <span>{{ item.label }}</span>
        <span v-if="item.tag" class="data-frag" style="margin-left:auto;font-size:8px;">{{ item.tag }}</span>
      </NuxtLink>
    </nav>
  </Sheet>
</template>
