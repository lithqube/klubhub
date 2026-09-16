<script setup lang="ts">
import { ref, computed } from 'vue'
import type { ScheduledPost } from '~/types/social'
import { dotColorForStatus } from './socialCalendarUtils'

const props = defineProps<{
  posts: ScheduledPost[]
  /** Optional: override the displayed month. Defaults to start of current month. */
  month?: Date
}>()

const emit = defineEmits<{
  'day-selected': [date: string | null]
}>()

// Current month state — use prop if provided (for testing), otherwise current month
const currentMonth = ref<Date>(
  props.month
    ? new Date(props.month.getFullYear(), props.month.getMonth(), 1)
    : new Date(new Date().getFullYear(), new Date().getMonth(), 1)
)

const selectedDay = ref<string | null>(null)

// Compute postsByDate: Map<YYYY-MM-DD, ScheduledPost[]>
const postsByDate = computed(() => {
  const map = new Map<string, ScheduledPost[]>()
  for (const post of props.posts) {
    const d = new Date(post.scheduledAtUtc)
    // Use local date components for the key
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    if (!map.has(key)) map.set(key, [])
    map.get(key)!.push(post)
  }
  return map
})

// Calendar grid computation
const calendarDays = computed(() => {
  const year = currentMonth.value.getFullYear()
  const month = currentMonth.value.getMonth()
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  // 0=Sun, 1=Mon ... convert to Mon-start: Mon=0, Tue=1, ... Sun=6
  const rawFirstDay = new Date(year, month, 1).getDay()
  const firstDayOfWeek = rawFirstDay === 0 ? 6 : rawFirstDay - 1

  const days: Array<{ date: string; dayNum: number; isCurrentMonth: boolean } | null> = []

  // Leading empty cells
  for (let i = 0; i < firstDayOfWeek; i++) {
    days.push(null)
  }

  // Actual days
  for (let d = 1; d <= daysInMonth; d++) {
    const dateStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    days.push({ date: dateStr, dayNum: d, isCurrentMonth: true })
  }

  return days
})

const monthLabel = computed(() => {
  const year = currentMonth.value.getFullYear()
  const month = currentMonth.value.toLocaleString('en-US', { month: 'long' }).toUpperCase()
  return `${month} ${year}`
})

function prevMonth() {
  const d = currentMonth.value
  currentMonth.value = new Date(d.getFullYear(), d.getMonth() - 1, 1)
}

function nextMonth() {
  const d = currentMonth.value
  currentMonth.value = new Date(d.getFullYear(), d.getMonth() + 1, 1)
}

function onDayClick(dateStr: string) {
  const dayPosts = postsByDate.value.get(dateStr)
  if (!dayPosts || dayPosts.length === 0) return

  if (selectedDay.value === dateStr) {
    selectedDay.value = null
    emit('day-selected', null)
  } else {
    selectedDay.value = dateStr
    emit('day-selected', dateStr)
  }
}

// Expose for testing
defineExpose({ postsByDate })
</script>

<template>
  <div class="glass-panel p-4">
    <!-- Header: prev / month label / next -->
    <div class="flex items-center justify-between mb-4">
      <button
        class="ghost-border px-3 py-1.5 font-terminal tracking-terminal text-xs uppercase text-tertiary hover:text-on-surface transition-colors"
        @click="prevMonth"
      >
        ← PREV
      </button>
      <span class="font-command font-bold text-sm uppercase text-on-surface tracking-command">
        {{ monthLabel }}
      </span>
      <button
        class="ghost-border px-3 py-1.5 font-terminal tracking-terminal text-xs uppercase text-tertiary hover:text-on-surface transition-colors"
        @click="nextMonth"
      >
        NEXT →
      </button>
    </div>

    <!-- Day-of-week headers (Mon-start) -->
    <div class="grid grid-cols-7 gap-1 mb-2">
      <div
        v-for="day in ['MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT', 'SUN']"
        :key="day"
        class="font-terminal tracking-terminal text-xs text-tertiary uppercase text-center py-1"
      >
        {{ day }}
      </div>
    </div>

    <!-- Calendar grid -->
    <div class="grid grid-cols-7 gap-1">
      <template v-for="(cell, idx) in calendarDays" :key="idx">
        <!-- Empty cell for padding -->
        <div v-if="cell === null" class="h-14" />

        <!-- Day cell -->
        <div
          v-else
          :data-date="cell.date"
          class="h-14 p-1.5 flex flex-col justify-between cursor-pointer border border-transparent transition-colors"
          :class="[
            selectedDay === cell.date
              ? 'bg-primary/20 border-primary'
              : 'hover:bg-surface-container',
            postsByDate.get(cell.date)?.length ? 'cursor-pointer' : 'cursor-default',
          ]"
          @click="onDayClick(cell.date)"
        >
          <!-- Day number -->
          <span class="font-command tracking-command text-xs text-on-surface">
            {{ cell.dayNum }}
          </span>

          <!-- Dot indicators -->
          <div v-if="postsByDate.get(cell.date)?.length" class="flex items-center gap-0.5 flex-wrap">
            <template v-if="(postsByDate.get(cell.date)?.length ?? 0) <= 3">
              <span
                v-for="post in postsByDate.get(cell.date)"
                :key="post.id"
                class="w-1.5 h-1.5 flex-shrink-0"
                :class="dotColorForStatus(post.status)"
              />
            </template>
            <template v-else>
              <span
                v-for="post in postsByDate.get(cell.date)!.slice(0, 3)"
                :key="post.id"
                class="w-1.5 h-1.5 flex-shrink-0"
                :class="dotColorForStatus(post.status)"
              />
              <span class="font-terminal text-[10px] text-tertiary leading-none">
                +{{ (postsByDate.get(cell.date)?.length ?? 0) - 3 }}
              </span>
            </template>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
