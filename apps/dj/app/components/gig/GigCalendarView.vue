<script setup lang="ts">
import { useGigStore } from '../../stores/gig'
import { storeToRefs } from 'pinia'

const gigStore = useGigStore()
const { gigs } = storeToRefs(gigStore)

const currentDate = ref(new Date())
const viewDate = ref(new Date())

const daysInMonth = computed(() => {
  const year = viewDate.value.getFullYear()
  const month = viewDate.value.getMonth()
  const firstDay = new Date(year, month, 1)
  const lastDay = new Date(year, month + 1, 0)
  const days: Date[] = []
  // Add padding for day of week
  const startPadding = firstDay.getDay() === 0 ? 6 : firstDay.getDay() - 1
  for (let i = 0; i < startPadding; i++) {
    days.push(new Date(year, month, -startPadding + i))
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
  'gig-click': [gig: any]
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
        :style="{ opacity: day.getMonth() === viewDate.getMonth() ? 1 : 0.4 }"
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
        <div style="display:flex;flex-wrap:wrap;gap:2px;">
          <template
            v-if="gigsByDate.has(day.toISOString().split('T')[0])"
          >
            <template
              v-for="(gig, i) in (gigsByDate.get(day.toISOString().split('T')[0]) || []).slice(0, 3)"
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
              v-if="(gigsByDate.get(day.toISOString().split('T')[0]) || []).length > 3"
              class="bar-lbl"
            >
              +{{ (gigsByDate.get(day.toISOString().split('T')[0]) || []).length - 3 }}
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
