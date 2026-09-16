<script setup lang="ts">
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'

interface Props {
  dates: Date[]
  selectedDate: Date
  events?: Date[]
}
const props = withDefaults(defineProps<Props>(), { events: () => [] })
const emit = defineEmits<{ 'update:selectedDate': [date: Date] }>()

const dayNames = ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT']

function isSameDay(a: Date, b: Date) {
  return a.toDateString() === b.toDateString()
}
function hasEvent(date: Date) {
  return props.events.some(e => isSameDay(e, date))
}
</script>

<template>
  <div class="bg-surface-container-low px-4 py-3">
    <!-- Header: label left + chevrons right -->
    <div class="flex justify-between items-center mb-3">
      <span class="font-terminal tracking-terminal text-tertiary text-[8px] uppercase">
        TIMELINE_VIEW
      </span>
      <div class="flex gap-1">
        <button class="p-1 text-tertiary hover:text-primary transition-colors">
          <ChevronLeft class="w-4 h-4" />
        </button>
        <button class="p-1 text-tertiary hover:text-primary transition-colors">
          <ChevronRight class="w-4 h-4" />
        </button>
      </div>
    </div>

    <!-- Scrollable date strip -->
    <div class="flex gap-2 overflow-x-auto pb-1">
      <button
        v-for="date in dates"
        :key="date.toISOString()"
        class="flex flex-col items-center min-w-[48px] px-2 py-2 transition-all duration-200 flex-shrink-0"
        :class="isSameDay(date, selectedDate)
          ? 'luminous-threshold bg-surface-container'
          : 'bg-surface-container-low text-tertiary hover:text-on-surface'"
        @click="emit('update:selectedDate', date)"
      >
        <!-- Day number -->
        <span
          class="font-command text-sm font-bold"
          :class="isSameDay(date, selectedDate) ? 'text-primary' : ''"
        >
          {{ date.getDate() }}
        </span>
        <!-- Day name -->
        <span class="font-terminal tracking-terminal text-[8px] uppercase mt-0.5">
          {{ dayNames[date.getDay()] }}
        </span>
        <!-- Event indicator dot (below date) -->
        <span
          v-if="hasEvent(date)"
          class="w-1.5 h-1.5 rounded-full bg-primary mt-1"
          aria-label="has event"
        />
        <span v-else class="w-1.5 h-1.5 mt-1" />
      </button>
    </div>
  </div>
</template>
