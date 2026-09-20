<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRaStore } from '~/stores/ra'
import Input from '~/components/ui/input/Input.vue'
import { Search, ExternalLink, Check, AlertCircle, Loader, X, Plus, Calendar, MapPin, Users } from 'lucide-vue-next'
import type { RAEVENT } from '~/types/ra'

const raStore = useRaStore()

const artistSlug = ref('')
const events = ref<RAEVENT[]>([])
const selectedEventIds = ref<Set<string>>(new Set())
const dryRun = ref(true)
const importing = ref(false)
const importResult = ref<{ gigsCreated: number; gigsSkipped: number; skippedReasons: string[] } | null>(null)

const hasSelection = computed(() => selectedEventIds.value.size > 0)

async function fetchEvents() {
  if (!artistSlug.value.trim()) return
  importing.value = true
  await raStore.fetchEvents(artistSlug.value)
  events.value = []
  importing.value = false
}

function toggleEvent(eventId: string) {
  if (selectedEventIds.value.has(eventId)) {
    selectedEventIds.value.delete(eventId)
  } else {
    selectedEventIds.value.add(eventId)
  }
  selectedEventIds.value = new Set(selectedEventIds.value)
}

function selectAll() {
  events.value.forEach(e => selectedEventIds.value.add(e.id))
  selectedEventIds.value = new Set(selectedEventIds.value)
}

function selectNone() {
  selectedEventIds.value.clear()
  selectedEventIds.value = new Set(selectedEventIds.value)
}

async function importEvents() {
  if (!hasSelection.value) return
  importing.value = true
  importResult.value = null
  try {
    const result = await raStore.importEvents({
      artist_slug: artistSlug.value,
      dry_run: dryRun.value,
    })
    importResult.value = {
      gigsCreated: result.gigsCreated,
      gigsSkipped: result.gigsSkipped,
      skippedReasons: result.skippedReasons,
    }
    if (!dryRun.value) {
      const gigStore = await import('~/stores/gig').then(m => m.useGigStore())
      await gigStore.loadFromApi()
    }
  } catch (e) {
    console.error('Import failed:', e)
  } finally {
    importing.value = false
  }
}

function formatDate(dateStr: string) {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

function isUpcoming(dateStr: string) {
  const date = new Date(dateStr + 'T00:00:00')
  return date >= new Date()
}

function clearForm() {
  artistSlug.value = ''
  events.value = []
  selectedEventIds.value = new Set()
  importResult.value = null
  raStore.clear()
}
</script>

<template>
  <div class="glass-panel" style="padding:16px;">
    <!-- Header -->
    <div class="flex items-center justify-between" style="margin-bottom:14px;">
      <div class="flex items-center gap-2">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:var(--color-secondary);">
          <rect x="3" y="4" width="18" height="18" rx="2"/>
          <path d="M3 10h18M8 4v16"/>
        </svg>
        <span class="section-lbl" style="color:var(--color-secondary);letter-spacing:.08em;">
          IMPORT FROM RA
        </span>
      </div>
      <button
        v-if="events.length > 0"
        class="btn-hud btn-hud-ghost btn-hud-xs"
        style="color:var(--color-tertiary);"
        @click="clearForm"
      >
        <X style="width:9px;height:9px;margin-right:4px;" aria-hidden="true" />
        CLEAR
      </button>
    </div>

    <!-- Slug input -->
    <div style="margin-bottom:14px;">
      <div class="input-label" style="margin-bottom:6px;">RA ARTIST SLUG</div>
      <div class="flex gap-2" style="gap:8px;">
        <Input
          v-model="artistSlug"
          placeholder="e.g. dj-phantom"
          class="hud-input flex-1"
          style="height:34px;font-size:12px;"
        />
        <button
          class="btn-hud btn-hud-violet btn-hud-sm"
          :disabled="!artistSlug.trim() || importing"
          style="height:34px;min-height:34px;"
          @click="fetchEvents"
        >
          <Loader
            v-if="importing && !events.length"
            class="animate-spin"
            style="width:11px;height:11px;"
            aria-hidden="true"
          />
          <Search v-else style="width:11px;height:11px;" aria-hidden="true" />
          FETCH
        </button>
      </div>
      <p style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);margin-top:6px;letter-spacing:.05em;text-transform:uppercase;">
        Enter slug from <span style="color:var(--color-secondary);">ra.co/dj/</span><span style="color:var(--color-secondary);text-transform:lowercase;">&lt;artist&gt;</span>
      </p>
    </div>

    <!-- Events list -->
    <div v-if="events.length > 0" style="margin-bottom:14px;">
      <!-- Selection bar -->
      <div style="display:flex;align-items:center;justify-content:space-between;padding:8px 10px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.08);margin-bottom:10px;border-radius:2px;">
        <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.05em;text-transform:uppercase;color:var(--color-tertiary);">
          <span style="color:var(--color-primary);font-weight:600;">{{ selectedEventIds.size }}</span> OF {{ events.length }} SELECTED
        </span>
        <div class="flex gap-2" style="gap:10px;">
          <button
            class="btn-hud btn-hud-ghost btn-hud-xs"
            @click="selectAll"
          >
            SELECT ALL
          </button>
          <button
            class="btn-hud btn-hud-ghost btn-hud-xs"
            @click="selectNone"
          >
            SELECT NONE
          </button>
        </div>
      </div>

      <!-- Event items -->
      <div style="display:flex;flex-direction:column;gap:6px;">
        <div
          v-for="event in events"
          :key="event.id"
          class="hud-card"
          style="padding:11px 13px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.08);cursor:pointer;transition:all .15s;display:flex;flex-direction:column;gap:7px;border-radius:2px;"
          :style="selectedEventIds.has(event.id) ? 'border-color:rgba(150,248,255,.18);background:rgba(150,248,255,.03);box-shadow:0 0 24px rgba(150,248,255,.06);' : ''"
          @click="toggleEvent(event.id)"
          :data-testid="`ra-event-${event.id}`"
        >
          <!-- Row 1: checkbox + title + status badge -->
          <div style="display:flex;align-items:flex-start;gap:8px;">
            <div
              style="width:16px;height:16px;border-radius:2px;border:1.5px solid flex-shrink:0;margin-top:2px;cursor:pointer;display:flex;align-items:center;justify-content:center;transition:all .15s;background:transparent;"
              :style="selectedEventIds.has(event.id) ? 'border-color:var(--color-primary);background:var(--color-primary);box-shadow:0 0 12px rgba(150,248,255,.3);' : 'border-color:var(--color-outline);'"
              :data-testid="`ra-event-checkbox-${event.id}`"
            >
              <Check
                v-if="selectedEventIds.has(event.id)"
                style="width:9px;height:9px;color:var(--color-surface);"
                aria-hidden="true"
              />
            </div>
            <div style="flex:1;min-width:0;">
              <div style="display:flex;align-items:center;gap:6px;">
                <span
                  style="font-family:var(--font-command);font-size:13px;font-weight:600;color:var(--color-on-surface);letter-spacing:-.02em;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;"
                >
                  {{ event.title }}
                </span>
                <span
                  v-if="!isUpcoming(event.date)"
                  class="badge-hud"
                  style="background:rgba(255,113,108,.08);border:1px dashed rgba(255,113,108,.25);color:var(--color-error);padding:2px 7px;font-size:8px;"
                >
                  PAST EVENT
                </span>
              </div>
            </div>
          </div>

          <!-- Row 2: meta info with icon chips -->
          <div style="display:flex;flex-wrap:wrap;gap:10px;" class="spost-meta">
            <span style="display:inline-flex;align-items:center;gap:5px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.04em;text-transform:uppercase;color:var(--color-tertiary);">
              <Calendar style="width:10px;height:10px;flex-shrink:0;" aria-hidden="true" />
              {{ formatDate(event.date) }}
            </span>
            <span style="display:inline-flex;align-items:center;gap:5px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.04em;text-transform:uppercase;color:var(--color-tertiary);">
              <MapPin style="width:10px;height:10px;flex-shrink:0;" aria-hidden="true" />
              {{ event.venueName }}
            </span>
            <span
              v-if="event.attending > 0"
              style="display:inline-flex;align-items:center;gap:5px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.04em;text-transform:uppercase;color:var(--color-tertiary);"
            >
              <Users style="width:10px;height:10px;flex-shrink:0;" aria-hidden="true" />
              {{ event.attending }} attending
            </span>
          </div>

          <!-- Row 3: promo text -->
          <p
            v-if="event.promo"
            class="spost-caption"
            style="margin:0;font-size:12px;line-height:1.55;color:var(--color-on-surface-variant);display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;"
          >
            {{ event.promo }}
          </p>

          <!-- Row 4: ticket link -->
          <a
            v-if="event.contentUrl"
            :href="event.contentUrl"
            target="_blank"
            rel="noopener noreferrer"
            style="align-self:flex-end;font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);text-decoration:none;display:inline-flex;align-items:center;gap:5px;padding:4px 8px;background:rgba(200,184,255,.04);border:1px solid rgba(200,184,255,.1);transition:all .15s;border-radius:1px;"
            @mouseenter="($event.target as HTMLElement).style.color = 'var(--color-secondary)';($event.target as HTMLElement).style.borderColor = 'rgba(200,184,255,.3)';($event.target as HTMLElement).style.background = 'rgba(200,184,255,.06)'"
            @mouseleave="($event.target as HTMLElement).style.color = 'var(--color-tertiary)';($event.target as HTMLElement).style.borderColor = 'rgba(200,184,255,.1)';($event.target as HTMLElement).style.background = 'rgba(200,184,255,.04)'"
          >
            <ExternalLink style="width:9px;height:9px;" aria-hidden="true" />
            VIEW TICKETS
          </a>
        </div>
      </div>
    </div>

    <!-- Options row -->
    <div v-if="events.length > 0" style="display:flex;align-items:center;justify-content:space-between;padding:9px 0;border-top:1px solid rgba(200,184,255,.06);margin-bottom:14px;">
      <label style="display:flex;align-items:center;gap:7px;cursor:pointer;">
        <input
          type="checkbox"
          v-model="dryRun"
          style="width:15px;height:15px;accent-color:var(--color-primary);cursor:pointer;"
        />
        <span class="section-lbl" style="color:var(--color-tertiary);">DRY RUN</span>
      </label>
      <span style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);letter-spacing:.04em;text-transform:uppercase;opacity:.7;">
        PREVIEW WITHOUT CREATING GIGS
      </span>
    </div>

    <!-- Import button -->
    <div v-if="events.length > 0" style="margin-bottom:14px;">
      <button
        class="btn-hud btn-hud-violet luminous-threshold"
        :disabled="!hasSelection || importing"
        style="width:100%;padding:0 20px;height:36px;"
        @click="importEvents"
      >
        <Loader
          v-if="importing"
          class="animate-spin"
          style="width:13px;height:13px;"
          aria-hidden="true"
        />
        <Plus v-else style="width:13px;height:13px;" aria-hidden="true" />
        <span style="font-size:10px;letter-spacing:.08em;">{{ dryRun ? 'PREVIEW IMPORT' : 'IMPORT EVENTS' }}</span>
      </button>
    </div>

    <!-- Import result -->
    <div v-if="importResult" style="margin-bottom:14px;">
      <div style="padding:11px 13px;background:var(--color-surface-container-lowest);border:1px solid rgba(150,248,255,.08);border-radius:2px;">
        <div style="display:flex;align-items:center;gap:14px;margin-bottom:7px;">
          <span style="display:inline-flex;align-items:center;gap:6px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.05em;text-transform:uppercase;color:var(--color-secondary);">
            <Check style="width:10px;height:10px;" aria-hidden="true" />
            <span style="color:var(--color-primary);font-weight:600;">{{ importResult.gigsCreated }}</span> GIGS CREATED
          </span>
          <span
            v-if="importResult.gigsSkipped > 0"
            style="display:inline-flex;align-items:center;gap:6px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.05em;text-transform:uppercase;color:var(--color-error);"
          >
            <AlertCircle style="width:10px;height:10px;" aria-hidden="true" />
            <span style="font-weight:600;">{{ importResult.gigsSkipped }}</span> SKIPPED
          </span>
        </div>
        <div v-if="importResult.skippedReasons.length > 0" style="display:flex;flex-direction:column;gap:3px;padding-left:2px;">
          <p
            v-for="(reason, idx) in importResult.skippedReasons.slice(0, 3)"
            :key="idx"
            style="font-family:var(--font-data);font-size:10px;color:var(--color-on-surface-variant);margin:0;display:flex;align-items:flex-start;gap:5px;"
          >
            <span style="color:var(--color-outline);font-size:8px;margin-top:2px;flex-shrink:0;">•</span>
            {{ reason }}
          </p>
          <p
            v-if="importResult.skippedReasons.length > 3"
            style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);margin:6px 0 0 7px;letter-spacing:.04em;text-transform:uppercase;"
          >
            ...and {{ importResult.skippedReasons.length - 3 }} more
          </p>
        </div>
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="importing && events.length === 0"
      style="padding:18px;display:flex;align-items:center;justify-content:center;gap:10px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.1);border-radius:2px;"
    >
      <Loader class="animate-spin" style="width:16px;height:16px;color:var(--color-secondary);" aria-hidden="true" />
      <span class="section-lbl" style="color:var(--color-secondary);letter-spacing:.08em;font-size:9px;">
        FETCHING EVENTS FROM RA...
      </span>
    </div>

    <!-- Error -->
    <div
      v-if="raStore.error && !importing"
      style="display:flex;align-items:center;gap:7px;padding:9px 11px;background:rgba(255,113,108,.05);border:1px solid rgba(255,113,108,.12);border-radius:2px;"
    >
      <AlertCircle style="width:13px;height:13px;color:var(--color-error);flex-shrink:0;" aria-hidden="true" />
      <span style="font-family:var(--font-data);font-size:11px;color:var(--color-error);">{{ raStore.error }}</span>
    </div>

    <!-- Footer -->
    <div style="margin-top:12px;padding-top:10px;border-top:1px solid rgba(200,184,255,.05);">
      <p style="font-family:var(--font-data);font-size:11px;color:var(--color-tertiary);line-height:1.55;margin:0;">
        Import upcoming events from Resident Advisor as gig records.
        Past events (7+ days old) are automatically skipped.
        Venues and contacts are matched or created as needed.
      </p>
    </div>
  </div>
</template>
