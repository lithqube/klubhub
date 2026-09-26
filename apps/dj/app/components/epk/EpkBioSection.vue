<script setup lang="ts">
import { ref, watch } from 'vue'
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'
import Input from '#kui/components/ui/input/Input.vue'
import Textarea from '#kui/components/ui/textarea/Textarea.vue'

const store = useEpkStore()
const { scheduleSave } = useEpkAutosave()

// Local state initialized from store — plain refs that avoid storeToRefs incompatibility
const bioShort = ref(store.bioShort as string)
const bioLong = ref(store.bioLong as string)

// Sync store → local on external changes (e.g. loadFromApi)
watch(() => store.bioShort as string, (v) => { if (v !== bioShort.value) bioShort.value = v })
watch(() => store.bioLong as string, (v) => { if (v !== bioLong.value) bioLong.value = v })

function onBioShortChange(value: string | number) {
  const str = String(value)
  bioShort.value = str
  store.bioShort = str
  scheduleSave({ bioShort: str })
}

function onBioLongChange(value: string | number) {
  const str = String(value)
  bioLong.value = str
  store.bioLong = str
  scheduleSave({ bioLong: str })
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">BIO</p>

    <!-- Short bio -->
    <div class="space-y-1">
      <label class="text-xs tracking-terminal text-tertiary uppercase font-terminal">
        SHORT BIO
      </label>
      <Input
        :model-value="bioShort"
        data-testid="bio-short-input"
        :maxlength="500"
        placeholder="Short bio..."
        @update:model-value="onBioShortChange"
      />
      <span
        data-testid="bio-short-counter"
        class="text-xs text-tertiary font-terminal"
      >
        {{ bioShort.length }} / 280
      </span>
    </div>

    <!-- Long bio -->
    <div class="space-y-1">
      <label class="text-xs tracking-terminal text-tertiary uppercase font-terminal">
        LONG BIO (Markdown)
      </label>
      <Textarea
        :model-value="bioLong"
        data-testid="bio-long-textarea"
        class="font-mono text-xs"
        :rows="8"
        placeholder="Long bio in Markdown..."
        @update:model-value="onBioLongChange"
      />
    </div>
  </div>
</template>
