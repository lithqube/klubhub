<script setup lang="ts">
import { computed } from 'vue'
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'
import Textarea from '#kui/components/ui/textarea/Textarea.vue'

const store = useEpkStore()
const techRider = computed({
  get: () => store.techRider,
  set: (v) => { store.techRider = v },
})
const { scheduleSave } = useEpkAutosave()

function onTechRiderChange(value: string) {
  scheduleSave({ techRider: value })
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">TECH RIDER</p>

    <Textarea
      v-model="techRider"
      data-testid="tech-rider-textarea"
      :rows="10"
      placeholder="Technical rider requirements..."
      @update:model-value="onTechRiderChange"
    />
  </div>
</template>
