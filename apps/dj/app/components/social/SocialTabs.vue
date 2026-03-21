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

      <!-- Left: tabs -->
      <div class="flex items-end gap-0">
        <!-- QUEUE tab -->
        <button
          class="px-4 py-3 font-terminal tracking-terminal text-xs uppercase transition-colors border-b-2 -mb-px"
          :class="activeTab === 'queue'
            ? 'text-primary border-primary'
            : 'text-tertiary border-transparent hover:text-on-surface'"
          @click="onTabChange('queue')"
        >
          QUEUE
        </button>

        <!-- CALENDAR tab -->
        <button
          class="px-4 py-3 font-terminal tracking-terminal text-xs uppercase transition-colors border-b-2 -mb-px"
          :class="activeTab === 'calendar'
            ? 'text-primary border-primary'
            : 'text-tertiary border-transparent hover:text-on-surface'"
          @click="onTabChange('calendar')"
        >
          CALENDAR
        </button>

        <!-- ANALYTICS stub (not a real tab) -->
        <div class="relative group">
          <span class="px-4 py-3 font-terminal tracking-terminal text-xs uppercase text-tertiary/40 cursor-not-allowed inline-block border-b-2 border-transparent -mb-px">
            ANALYTICS
          </span>
          <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-surface-container border border-outline-variant/30 font-terminal tracking-terminal text-[10px] text-tertiary uppercase whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
            Coming Soon
          </div>
        </div>

        <!-- AUTOMATIONS stub (not a real tab) -->
        <div class="relative group">
          <span class="px-4 py-3 font-terminal tracking-terminal text-xs uppercase text-tertiary/40 cursor-not-allowed inline-block border-b-2 border-transparent -mb-px">
            AUTOMATIONS
          </span>
          <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-surface-container border border-outline-variant/30 font-terminal tracking-terminal text-[10px] text-tertiary uppercase whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
            Coming Soon
          </div>
        </div>
      </div>

      <!-- Right: action buttons -->
      <div class="flex items-center gap-3 pb-3">
        <button
          class="ghost-border px-4 py-2 font-terminal tracking-terminal text-xs uppercase text-on-surface-variant hover:text-primary hover:border-primary/40 transition-colors flex items-center gap-2"
          @click="emit('quick-export')"
        >
          <span>⚡</span>
          <span>QUICK EXPORT FROM TRACKLIST</span>
        </button>
        <button
          class="gradient-cta px-4 py-2 font-terminal tracking-terminal text-xs uppercase text-on-primary flex items-center gap-2"
          @click="emit('add-to-queue')"
        >
          <span>+ ADD TO QUEUE</span>
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
