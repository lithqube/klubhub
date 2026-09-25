<script setup lang="ts">
import { useGigStore } from '../../stores/gig'
import { storeToRefs } from 'pinia'
import type { Gig } from '../../types/gig'

const gigStore = useGigStore()
const { gigs } = storeToRefs(gigStore)

const viewDate = ref(new Date())

const daysInMonth = computed(() => {
  const year = viewDate.value.getFullYear()
  const month = viewDate.value.getMonth()
  const firstDay = new Date(year, month, 1)
  const lastDay = new Date(year, month + 1, 0)
  const days: Date[] = []
  // Add padding for day of week
  const startPadding = firstDay.getDay() === 0 ? 6 : firstDay.getDay() - 1
  // Day 0 is the last day of the previous month, so the final padding
  // cell (i = startPadding - 1) must resolve to day 0, not day -1.
  for (let i = 0; i < startPadding; i++) {
    days.push(new Date(year, month, -startPadding + i + 1))
  }
  for (let d = 1; d <= lastDay.getDate(); d++) {
    days.push(new Date(year, month, d))
  }
  // Pad to complete last week
  while (days.length % 7 !== 0) {
    days.push(new Date(year, month + 1, days.length - startPadding - lastDay.getDate() + 1))
  }
  return days
})

// Build the YYYY-MM-DD key from local date parts. toISOString() converts
// local midnight to UTC, which is the previous day in any UTC+ timezone
// and would file gigs under the wrong cell.
function dayKey(day: Date): string {
  const m = String(day.getMonth() + 1).padStart(2, '0')
  const d = String(day.getDate()).padStart(2, '0')
  return `${day.getFullYear()}-${m}-${d}`
}

const gigsByDate = computed(() => {
  const map = new Map<string, typeof gigs.value>()
  for (const gig of gigs.value) {
    const dateKey = gig.date.split('T')[0]
    if (!map.has(dateKey)) map.set(dateKey, [])
    map.get(dateKey)!.push(gig)
  }
  return map
})

function prevMonth() {
  viewDate.value = new Date(
    viewDate.value.getFullYear(),
    viewDate.value.getMonth() - 1,
    1
  )
}

function nextMonth() {
  viewDate.value = new Date(
    viewDate.value.getFullYear(),
    viewDate.value.getMonth() + 1,
    1
  )
}

function gigStatusColor(status: string) {
  switch (status) {
    case 'confirmed':
      return 'var(--color-primary)'
    case 'inquiry':
    case 'advanced':
      return 'var(--color-secondary)'
    case 'played':
      return 'var(--color-status-archived)'
    case 'cancelled':
      return 'var(--color-error)'
    default:
      return 'var(--color-tertiary)'
  }
}

const emit = defineEmits<{
  'gig-click': [gig: Gig]
}>()
</script>

<template>
  <div class="glass" style="padding:14px;">
    <!-- Calendar header -->
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;">
      <button
        class="btn-hud btn-hud-ghost btn-hud-xs"
        @click="prevMonth"
      >
        ← PREV
      </button>
      <div
        style="font-family:var(--font-command);font-size:14px;font-weight:700;letter-spacing:-.02em;text-transform:uppercase;"
      >
        {{ viewDate.toLocaleDateString('en-US', { month: 'long', year: 'numeric' }) }}
      </div>
      <button
        class="btn-hud btn-hud-ghost btn-hud-xs"
        @click="nextMonth"
      >
        NEXT →
      </button>
    </div>

    <!-- Day labels -->
    <div class="cal-grid" style="margin-bottom:4px;">
      <div
        v-for="day in ['MON','TUE','WED','THU','FRI','SAT','SUN']"
        :key="day"
        class="section-lbl"
        style="text-align:center;"
      >
        {{ day }}
      </div>
    </div>

    <!-- Calendar grid -->
    <div class="cal-grid">
      <div
        v-for="(day, idx) in daysInMonth"
        :key="idx"
        class="cal-day"
      >
        <div
          class="cal-day-num"
          :style="{
            color: day.getMonth() === viewDate.getMonth()
              ? 'var(--color-on-surface)'
              : 'var(--color-tertiary)',
          }"
        >
          {{ day.getDate() }}
        </div>
        <div
          style="display:flex;flex-wrap:wrap;gap:2px;"
          :style="{ opacity: day.getMonth() === viewDate.getMonth() ? 1 : 0.4 }"
        >
          <template
            v-if="gigsByDate.has(dayKey(day))"
          >
            <template
              v-for="gig in (gigsByDate.get(dayKey(day)) || []).slice(0, 3)"
              :key="gig.id"
            >
              <div
                style="width:6px;height:6px;"
                :style="{ background: gigStatusColor(gig.status) }"
                :title="gig.event_name"
                @click="emit('gig-click', gig)"
              />
            </template>
            <div
              v-if="(gigsByDate.get(dayKey(day)) || []).length > 3"
              class="bar-lbl"
            >
              +{{ (gigsByDate.get(dayKey(day)) || []).length - 3 }}
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
