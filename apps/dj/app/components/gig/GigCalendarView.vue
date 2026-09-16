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
      return 'var(--color-tertiary)'
    default:
      return 'var(--color-tertiary)'
  }
}

const emit = defineEmits<{
  'gig-click': [gig: Gig]
}>()
</script>

<template>
  <div class="glass" style="padding:16px;">
    <!-- Calendar header -->
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px;">
      <button
        class="btn-hud"
        style="padding:4px 8px;font-size:10px;"
        @click="prevMonth"
      >
        ← PREV
      </button>
      <div
        style="font-family:var(--font-command);font-size:14px;font-weight:700;letter-spacing:-.02em;"
      >
        {{ viewDate.toLocaleDateString('en-US', { month: 'long', year: 'numeric' }) }}
      </div>
      <button
        class="btn-hud"
        style="padding:4px 8px;font-size:10px;"
        @click="nextMonth"
      >
        NEXT →
      </button>
    </div>

    <!-- Day labels -->
    <div style="display:grid;grid-template-columns:repeat(7,1fr);gap:2px;margin-bottom:4px;">
      <div
        v-for="day in ['MON','TUE','WED','THU','FRI','SAT','SUN']"
        :key="day"
        class="section-lbl"
        style="text-align:center;font-size:8px;"
      >
        {{ day }}
      </div>
    </div>

    <!-- Calendar grid -->
    <div style="display:grid;grid-template-columns:repeat(7,1fr);gap:2px;">
      <div
        v-for="(day, idx) in daysInMonth"
        :key="idx"
        style="min-height:48px;padding:4px;border-radius:2px;"
        :style="{
          background: day.getMonth() === viewDate.getMonth()
            ? 'rgba(255,255,255,0.03)'
            : 'transparent',
        }"
      >
        <div
          style="font-size:10px;font-family:var(--font-command);margin-bottom:4px;"
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
              v-for="gig in (gigsByDate.get(day.toISOString().split('T')[0]) || []).slice(0, 3)"
              :key="gig.id"
            >
              <div
                style="width:8px;height:8px;border-radius:50%;"
                :style="{ background: gigStatusColor(gig.status) }"
                :title="gig.event_name"
                @click="emit('gig-click', gig)"
              />
            </template>
            <div
              v-if="(gigsByDate.get(day.toISOString().split('T')[0]) || []).length > 3"
              style="font-size:7px;color:var(--color-tertiary);"
            >
              +{{ (gigsByDate.get(day.toISOString().split('T')[0]) || []).length - 3 }}
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
