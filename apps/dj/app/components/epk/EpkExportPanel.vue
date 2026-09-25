<script setup lang="ts">
import { ref, watch } from 'vue'
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'
import type { SectionVisibility } from '~/types/epk'
import Switch from '#kui/components/ui/switch/Switch.vue'
import Button from '#kui/components/ui/button/Button.vue'
import Progress from '#kui/components/ui/progress/Progress.vue'

const SECTIONS = [
  { key: 'bio',           label: 'BIO' },
  { key: 'photos',        label: 'PHOTOS' },
  { key: 'gigHighlights', label: 'GIG HIGHLIGHTS' },
  { key: 'pressQuotes',   label: 'PRESS QUOTES' },
  { key: 'techRider',     label: 'TECH RIDER' },
  { key: 'stagePlot',     label: 'STAGE PLOT' },
  { key: 'socialLinks',   label: 'SOCIAL LINKS' },
  { key: 'contactInfo',   label: 'CONTACT INFO' },
] as const

const store = useEpkStore()
const { scheduleSave } = useEpkAutosave()

// Local state for section visibility — initialized from store
const sectionVisibility = ref<SectionVisibility>({ ...(store.sectionVisibility as SectionVisibility) })

// Sync store → local on external changes
watch(() => store.sectionVisibility as SectionVisibility, (v) => {
  sectionVisibility.value = { ...v }
})

// Initialize all section keys to true if not set
SECTIONS.forEach(({ key }) => {
  if (sectionVisibility.value[key] === undefined) {
    sectionVisibility.value[key] = true
  }
})

const isGenerating = ref(false)

async function handleExport() {
  isGenerating.value = true
  try {
    await store.generateExport()
    await store.loadExports()
  } finally {
    isGenerating.value = false
  }
}

async function handleDeleteExport(id: string) {
  await store.deleteExport(id)
}

function onSectionToggle(key: string, value: boolean) {
  sectionVisibility.value = { ...sectionVisibility.value, [key]: value }
  store.sectionVisibility = sectionVisibility.value
  scheduleSave({ sectionVisibility: sectionVisibility.value })
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">EXPORT PDF</p>

    <!-- Save status indicator -->
    <div
      v-if="(store.saveStatus as string) === 'saved'"
      data-testid="save-status-indicator"
      class="text-xs font-terminal tracking-terminal text-primary"
    >
      SAVED
    </div>

    <!-- Section visibility toggles -->
    <div class="space-y-2">
      <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">INCLUDE SECTIONS</p>
      <div
        v-for="section in SECTIONS"
        :key="section.key"
        class="flex items-center justify-between py-1"
        :data-testid="`section-toggle-row-${section.key}`"
      >
        <span class="text-xs font-terminal text-on-surface-variant uppercase tracking-terminal">
          {{ section.label }}
        </span>
        <Switch
          :model-value="sectionVisibility[section.key] !== false"
          :data-testid="`section-toggle-${section.key}`"
          @update:model-value="onSectionToggle(section.key, $event)"
        />
      </div>
    </div>

    <!-- Progress bar shown during generation -->
    <Progress
      v-if="isGenerating"
      data-testid="export-progress"
      :model-value="0"
      class="animate-pulse"
    />

    <!-- Export PDF button -->
    <Button
      class="gradient-cta w-full"
      :disabled="isGenerating"
      data-testid="export-pdf-btn"
      type="button"
      @click="handleExport"
    >
      {{ isGenerating ? 'GENERATING...' : 'EXPORT PDF' }}
    </Button>

    <!-- Export history list -->
    <div class="space-y-2">
      <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">EXPORT HISTORY</p>
      <div
        v-if="(store.exports as any[]).length === 0"
        class="text-xs text-tertiary font-terminal uppercase"
      >
        No exports yet
      </div>
      <ul
        v-else
        data-testid="export-history-list"
        class="space-y-2"
      >
        <li
          v-for="item in (store.exports as any[])"
          :key="item.id"
          data-testid="export-history-item"
          class="flex items-center justify-between gap-2 text-xs font-terminal"
        >
          <span class="font-terminal text-tertiary">{{ formatDate(item.createdAt) }}</span>
          <div class="flex items-center gap-2">
            <a
              :href="item.downloadUrl"
              target="_blank"
              rel="noopener"
              class="text-primary hover:underline uppercase tracking-terminal"
              :data-testid="`export-download-${item.id}`"
            >
              DOWNLOAD
            </a>
            <button
              class="text-tertiary hover:text-error font-terminal"
              type="button"
              :data-testid="`export-delete-${item.id}`"
              @click="handleDeleteExport(item.id)"
            >
              ×
            </button>
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>
