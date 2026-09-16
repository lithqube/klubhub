<script setup lang="ts">
import { ref, watch } from 'vue'
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'
import type { PressQuote } from '~/types/epk'
import Input from '~/components/ui/input/Input.vue'
import Textarea from '~/components/ui/textarea/Textarea.vue'
import Button from '~/components/ui/button/Button.vue'

const store = useEpkStore()
const { scheduleSave } = useEpkAutosave()

// Local state initialized from store
const quotes = ref<PressQuote[]>((store.pressQuotes as PressQuote[]).map((q) => ({ ...q })))

// Sync store → local on external changes
watch(() => store.pressQuotes as PressQuote[], (v) => {
  const vStr = JSON.stringify(v)
  if (vStr !== JSON.stringify(quotes.value)) {
    quotes.value = v.map((q) => ({ ...q }))
  }
})

function addQuote() {
  const updated = [...quotes.value, { text: '', source: '' }]
  quotes.value = updated
  store.pressQuotes = updated
  scheduleSave({ pressQuotes: updated })
}

function removeQuote(index: number) {
  const updated = quotes.value.filter((_, i) => i !== index)
  quotes.value = updated
  store.pressQuotes = updated
  scheduleSave({ pressQuotes: updated })
}

function onTextChange(index: number, value: string | number) {
  const str = String(value)
  const updated = quotes.value.map((q, i) => i === index ? { ...q, text: str } : q)
  quotes.value = updated
  store.pressQuotes = updated
  scheduleSave({ pressQuotes: updated })
}

function onSourceChange(index: number, value: string | number) {
  const str = String(value)
  const updated = quotes.value.map((q, i) => i === index ? { ...q, source: str } : q)
  quotes.value = updated
  store.pressQuotes = updated
  scheduleSave({ pressQuotes: updated })
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">PRESS QUOTES</p>

    <!-- Quotes list -->
    <div class="space-y-4">
      <div
        v-for="(quote, index) in quotes"
        :key="index"
        class="space-y-2 ghost-border p-2"
        :data-testid="`quote-item-${index}`"
      >
        <Textarea
          :model-value="quote.text"
          :data-testid="`quote-text-${index}`"
          placeholder="Quote text..."
          :rows="3"
          class="font-mono text-xs"
          @update:model-value="onTextChange(index, $event)"
        />
        <div class="flex items-center gap-2">
          <Input
            :model-value="quote.source"
            :data-testid="`quote-source-${index}`"
            placeholder="Source (e.g. Mixmag)"
            class="flex-1"
            @update:model-value="onSourceChange(index, $event)"
          />
          <button
            :data-testid="`quote-delete-${index}`"
            class="text-tertiary hover:text-on-surface px-2 py-1 font-terminal text-xs"
            type="button"
            @click="removeQuote(index)"
          >
            ×
          </button>
        </div>
      </div>
    </div>

    <!-- Add quote button -->
    <Button
      variant="ghost"
      size="sm"
      type="button"
      data-testid="add-quote-btn"
      @click="addQuote"
    >
      + ADD QUOTE
    </Button>
  </div>
</template>
