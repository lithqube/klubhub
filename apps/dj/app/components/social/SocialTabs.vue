<script setup lang="ts">
import { ref } from 'vue'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '~/components/ui/tabs'
import { storeToRefs } from 'pinia'
import { useSocialStore } from '~/stores/social'
import SocialPostCompose from '~/components/social/SocialPostCompose.vue'
import SocialQueueGrid from '~/components/social/SocialQueueGrid.vue'
import type { ScheduledPost } from '~/types/social'

const props = defineProps<{
  posts: ScheduledPost[]
  composePanelOpen: boolean
  prefilledImageId: string | null
}>()

const emit = defineEmits<{
  'tab-change': [value: 'queue' | 'calendar']
  'quick-export': []
  'add-to-queue': []
}>()

const activeTab = ref<'queue' | 'calendar'>('queue')

function onTabChange(val: string) {
  activeTab.value = val as 'queue' | 'calendar'
  emit('tab-change', activeTab.value)
}
</script>

<template>
  <div class="flex flex-col flex-1 overflow-hidden">

    <!-- Tab bar row -->
    <div class="flex items-center justify-between border-b border-outline-variant/20 pb-0 flex-shrink-0">

      <!-- Left: tabs — Fitts's Law: min-h-[44px] on all tabs -->
      <div class="flex items-end gap-0" role="tablist" aria-label="Social scheduler views">

        <!-- QUEUE tab -->
        <button
          class="px-4 py-3 min-h-[44px] font-terminal tracking-terminal text-xs uppercase transition-colors border-b-2 -mb-px"
          :class="activeTab === 'queue'
            ? 'text-primary border-primary'
            : 'text-tertiary border-transparent hover:text-on-surface'"
          @click="onTabChange('queue')"
          role="tab"
          :aria-selected="activeTab === 'queue'"
        >
          QUEUE
        </button>

        <!-- CALENDAR tab -->
        <button
          class="px-4 py-3 min-h-[44px] font-terminal tracking-terminal text-xs uppercase transition-colors border-b-2 -mb-px"
          :class="activeTab === 'calendar'
            ? 'text-primary border-primary'
            : 'text-tertiary border-transparent hover:text-on-surface'"
          @click="onTabChange('calendar')"
          role="tab"
          :aria-selected="activeTab === 'calendar'"
        >
          CALENDAR
        </button>

        <!-- Stub tabs: hidden on mobile (Hick's Law — progressive disclosure of future features) -->
        <!-- ANALYTICS stub -->
        <div class="relative group hidden sm:block">
          <span class="px-4 py-3 min-h-[44px] font-terminal tracking-terminal text-xs uppercase text-tertiary/40 cursor-not-allowed inline-flex items-center border-b-2 border-transparent -mb-px">
            ANALYTICS
          </span>
          <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-surface-container border border-outline-variant/30 font-terminal tracking-terminal text-[10px] text-tertiary uppercase whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
            Coming Soon
          </div>
        </div>

        <!-- AUTOMATIONS stub -->
        <div class="relative group hidden sm:block">
          <span class="px-4 py-3 min-h-[44px] font-terminal tracking-terminal text-xs uppercase text-tertiary/40 cursor-not-allowed inline-flex items-center border-b-2 border-transparent -mb-px">
            AUTOMATIONS
          </span>
          <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-surface-container border border-outline-variant/30 font-terminal tracking-terminal text-[10px] text-tertiary uppercase whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
            Coming Soon
          </div>
        </div>

      </div>

      <!-- Right: action buttons
           Progressive Disclosure: long label hidden on mobile (Hick's Law — reduce choices shown)
           Fitts's Law: min-h-[44px] on all tap targets
      -->
      <div class="flex items-center gap-2 md:gap-3 pb-3">
        <button
          class="ghost-border px-3 md:px-4 min-h-[44px] font-terminal tracking-terminal text-xs uppercase text-on-surface-variant hover:text-primary hover:border-primary/40 transition-colors flex items-center gap-1.5 md:gap-2"
          @click="emit('quick-export')"
          aria-label="Quick export from tracklist"
        >
          <span aria-hidden="true">⚡</span>
          <!-- Label: icon-only on mobile (Progressive Disclosure), full label on md+ -->
          <span class="hidden md:inline">QUICK EXPORT</span>
        </button>
        <button
          class="gradient-cta px-3 md:px-4 min-h-[44px] font-terminal tracking-terminal text-xs uppercase text-on-primary flex items-center gap-1.5 md:gap-2"
          @click="emit('add-to-queue')"
        >
          <span>+</span>
          <span class="hidden sm:inline">ADD TO QUEUE</span>
          <span class="sm:hidden">QUEUE</span>
        </button>
      </div>

    </div>

    <!-- Scrollable content area -->
    <div class="flex-1 overflow-y-auto py-6">

      <!-- Compose panel (shown above queue when open) -->
      <SocialPostCompose
        v-if="props.composePanelOpen"
        :prefilled-image-id="props.prefilledImageId"
        class="mb-6"
      />

      <!-- Queue view -->
      <div v-if="activeTab === 'queue'">
        <SocialQueueGrid :posts="props.posts" />
      </div>

      <!-- Calendar view (stub) -->
      <div v-else-if="activeTab === 'calendar'" class="flex items-center justify-center py-20">
        <div class="text-center">
          <p class="font-terminal tracking-terminal text-xs text-tertiary uppercase">
            CALENDAR VIEW
          </p>
          <p class="font-data text-xs text-on-surface-variant mt-2">
            Monthly calendar coming soon
          </p>
        </div>
      </div>

    </div>

  </div>
</template>
