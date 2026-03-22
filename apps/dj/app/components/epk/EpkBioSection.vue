<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'

const store = useEpkStore()
const { bioShort, bioLong } = storeToRefs(store)
const { scheduleSave } = useEpkAutosave()

function onBioShortChange(value: string) {
  scheduleSave({ bioShort: value })
}

function onBioLongChange(value: string) {
  scheduleSave({ bioLong: value })
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">BIO</p>

    <!-- Short bio -->
    <div class="space-y-1">
      <label class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">
        SHORT BIO
      </label>
      <Input
        v-model="bioShort"
        data-testid="bio-short-input"
        :maxlength="500"
        placeholder="Short bio..."
        @update:model-value="onBioShortChange"
      />
      <span
        data-testid="bio-short-counter"
        class="text-xs text-muted-foreground font-terminal"
      >
        {{ bioShort.length }} / 280
      </span>
    </div>

    <!-- Long bio -->
    <div class="space-y-1">
      <label class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">
        LONG BIO (Markdown)
      </label>
      <Textarea
        v-model="bioLong"
        data-testid="bio-long-textarea"
        class="font-mono text-xs"
        :rows="8"
        placeholder="Long bio in Markdown..."
        @update:model-value="onBioLongChange"
      />
    </div>
  </div>
</template>
