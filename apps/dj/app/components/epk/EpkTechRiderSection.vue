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

/** Common asks, for a one-tap start — the field stays free text underneath
 * for anything these don't cover (stage plot, power, hospitality). */
const RIDER_CHIPS = ['2× CDJ-3000', 'DJM-A9 or better', 'Monitor wedges ×2', 'XLR line out', 'Own controller allowed', 'Booth PA check on arrival']
function insertChip(text: string) {
  const next = techRider.value.trim() ? `${techRider.value.trim()}, ${text}` : text
  techRider.value = next
  onTechRiderChange(next)
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">TECH RIDER</p>

    <div class="flex flex-wrap gap-2" role="group" aria-label="Quick-insert requirement">
      <button v-for="c in RIDER_CHIPS" :key="c" type="button" class="social-chip" @click="insertChip(c)">+ {{ c }}</button>
    </div>

    <Textarea
      v-model="techRider"
      data-testid="tech-rider-textarea"
      :rows="10"
      placeholder="Technical rider requirements..."
      @update:model-value="onTechRiderChange"
    />
  </div>
</template>
