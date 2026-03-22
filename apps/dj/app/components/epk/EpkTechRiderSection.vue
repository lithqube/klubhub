<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'

const store = useEpkStore()
const { techRider } = storeToRefs(store)
const { scheduleSave } = useEpkAutosave()

function onTechRiderChange(value: string) {
  scheduleSave({ techRider: value })
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">TECH RIDER</p>

    <Textarea
      v-model="techRider"
      data-testid="tech-rider-textarea"
      :rows="10"
      placeholder="Technical rider requirements..."
      @update:model-value="onTechRiderChange"
    />
  </div>
</template>
