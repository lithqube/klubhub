<script setup lang="ts">
import { ref, watch } from 'vue'
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'
import Input from '~/components/ui/input/Input.vue'
import Button from '~/components/ui/button/Button.vue'

const store = useEpkStore()
const { scheduleSave } = useEpkAutosave()

// Local state initialized from store
const highlights = ref<string[]>([...store.gigHighlights as string[]])

// Sync store → local on external changes (e.g. loadFromApi)
watch(() => store.gigHighlights as string[], (v) => {
  if (JSON.stringify(v) !== JSON.stringify(highlights.value)) {
    highlights.value = [...v]
  }
})

function addHighlight() {
  const updated = [...highlights.value, '']
  highlights.value = updated
  store.gigHighlights = updated
  scheduleSave({ gigHighlights: updated })
}

function removeHighlight(index: number) {
  const updated = highlights.value.filter((_, i) => i !== index)
  highlights.value = updated
  store.gigHighlights = updated
  scheduleSave({ gigHighlights: updated })
}

function onHighlightChange(index: number, value: string | number) {
  const str = String(value)
  const updated = [...highlights.value]
  updated[index] = str
  highlights.value = updated
  store.gigHighlights = updated
  scheduleSave({ gigHighlights: updated })
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">GIG HIGHLIGHTS</p>

    <!-- Highlights list -->
    <div class="space-y-2">
      <div
        v-for="(highlight, index) in highlights"
        :key="index"
        class="flex items-center gap-2"
        :data-testid="`highlight-item-${index}`"
      >
        <Input
          :model-value="highlight"
          :data-testid="`highlight-input-${index}`"
          placeholder="Highlight..."
          class="flex-1"
          @update:model-value="onHighlightChange(index, $event)"
        />
        <button
          :data-testid="`highlight-delete-${index}`"
          class="text-tertiary hover:text-on-surface px-2 py-1 font-terminal text-xs"
          type="button"
          @click="removeHighlight(index)"
        >
          ×
        </button>
      </div>
    </div>

    <!-- Action buttons -->
    <div class="flex gap-2 flex-wrap">
      <Button
        variant="ghost"
        size="sm"
        type="button"
        data-testid="add-highlight-btn"
        @click="addHighlight"
      >
        + ADD HIGHLIGHT
      </Button>

      <Button
        variant="outline"
        size="sm"
        type="button"
        :disabled="true"
        title="Coming Soon — available after Gig Tracker is set up"
        data-testid="import-from-gigs-btn"
      >
        IMPORT FROM GIGS
      </Button>
    </div>
  </div>
</template>
