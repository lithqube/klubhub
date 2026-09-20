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
  // For demo: events will be populated from backend in real implementation
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
  <div class="glass-panel" style="padding:14px 16px;">
    <!-- Header -->
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px;">
      <div style="display:flex;align-items:center;gap:6px;">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:var(--color-secondary);">
          <rect x="3" y="4" width="18" height="18" rx="2"/>
          <path d="M3 10h18M8 4v16"/>
        </svg>
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-secondary);">
          IMPORT FROM RA
        </span>
      </div>
      <button
        v-if="events.length > 0"
        class="btn-hud btn-hud-ghost"
        style="padding:0 8px;height:18px;font-size:8px;"
        @click="clearForm"
      >
        <X style="width:9px;height:9px;margin-right:3px;" aria-hidden="true" />
        CLEAR
      </button>
    </div>

    <!-- Slug input -->
    <div style="margin-bottom:10px;">
      <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);margin-bottom:5px;">
        RA ARTIST SLUG
      </div>
      <div style="display:flex;gap:6px;">
        <Input
          v-model="artistSlug"
          :placeholder="'e.g. dj-phantom'"
          class="flex-1 hud-input"
          style="padding:0 10px;height:28px;font-size:11px;"
        />
        <button
          class="btn-hud btn-hud-violet hud-action-btn"
          :disabled="!artistSlug.trim() || importing"
          style="height:28px;padding:0 10px;display:inline-flex;align-items:center;gap:4px;"
          @click="fetchEvents"
        >
          <Loader
            v-if="importing && !events.length"
            class="animate-spin"
            style="width:11px;height:11px;"
            aria-hidden="true"
          />
          <Search v-else style="width:11px;height:11px;" aria-hidden="true" />
          <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;">
            FETCH
          </span>
        </button>
      </div>
      <div style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);margin-top:4px;letter-spacing:.04em;">
        ra.co/dj/<span style="color:var(--color-secondary);opacity:.7;">&lt;artist&gt;</span>
      </div>
    </div>

    <!-- Events list -->
    <div v-if="events.length > 0" style="margin-bottom:10px;">
      <!-- Selection bar -->
      <div style="display:flex;align-items:center;justify-content:space-between;padding:6px 8px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.04);margin-bottom:6px;">
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.04em;color:var(--color-tertiary);">
          <span style="color:var(--color-secondary);font-weight:600;">{{ selectedEventIds.size }}</span> of {{ events.length }} selected
        </span>
        <div style="display:flex;gap:8px;">
          <button
            class="hud-link"
            style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);background:none;border:none;cursor:pointer;padding:0;"
            @click="selectAll"
          >
            SELECT ALL
          </button>
          <button
            class="hud-link"
            style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);background:none;border:none;cursor:pointer;padding:0;"
            @click="selectNone"
          >
            SELECT NONE
          </button>
        </div>
      </div>

      <!-- Event items -->
      <div style="display:flex;flex-direction:column;gap:5px;">
        <div
          v-for="event in events"
          :key="event.id"
          style="display:flex;align-items:flex-start;gap:8px;padding:9px 10px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.06);cursor:pointer;transition:border-color .15s, background .15s;"
          :style="selectedEventIds.has(event.id) ? 'border-color:rgba(150,248,255,.2);background:rgba(150,248,255,.03);' : 'border-color:rgba(200,184,255,.06);'",
          @click="toggleEvent(event.id)"
          :data-testid="`ra-event-${event.id}`"
        >
          <!-- Checkbox -->
          <div
            style="width:14px;height:14px;border-radius:2px;border:1.5px solid flex-shrink:0;margin-top:1px;cursor:pointer;display:flex;align-items:center;justify-content:center;transition:all .15s;"
            :style="selectedEventIds.has(event.id) ? 'border-color:var(--color-primary);background:var(--color-primary);' : 'border-color:var(--color-outline);background:transparent;'"
            :data-testid="`ra-event-checkbox-${event.id}`"
          >
            <Check
              v-if="selectedEventIds.has(event.id)"
              style="width:8px;height:8px;color:var(--color-surface);"
              aria-hidden="true"
            />
          </div>

          <!-- Event content -->
          <div style="flex:1;min-width:0;">
            <!-- Event header -->
            <div style="display:flex;align-items:center;gap:6px;margin-bottom:5px;">
              <span
                style="font-family:var(--font-command);font-size:12px;font-weight:500;color:var(--color-on-surface);flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;"
              >
                {{ event.title }}
              </span>
              <span
                v-if="!isUpcoming(event.date)"
                style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-error);padding:2px 4px;background:rgba(255,113,108,.08);border:1px solid rgba(255,113,108,.12);flex-shrink:0;"
              >
                PAST
              </span>
            </div>

            <!-- Event meta -->
            <div style="display:flex;flex-wrap:wrap;gap:8px;margin-bottom:4px;">
              <span style="display:inline-flex;align-items:center;gap:3px;font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);letter-spacing:.03em;">
                <Calendar style="width:8px;height:8px;flex-shrink:0;" aria-hidden="true" />
                {{ formatDate(event.date) }}
              </span>
              <span style="display:inline-flex;align-items:center;gap:3px;font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);letter-spacing:.03em;">
                <MapPin style="width:8px;height:8px;flex-shrink:0;" aria-hidden="true" />
                {{ event.venueName }}
              </span>
              <span
                v-if="event.attending > 0"
                style="display:inline-flex;align-items:center;gap:3px;font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);letter-spacing:.03em;"
              >
                <Users style="width:8px;height:8px;flex-shrink:0;" aria-hidden="true" />
                {{ event.attending }} attending
              </span>
            </div>

            <!-- Promo -->
            <p
              v-if="event.promo"
              style="font-family:var(--font-data);font-size:10px;color:var(--color-on-surface-variant);margin:0;line-height:1.4;overflow:hidden;text-overflow:ellipsis;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;"
            >
              {{ event.promo }}
            </p>
          </div>

          <!-- Ticket link -->
          <a
            v-if="event.contentUrl"
            :href="event.contentUrl"
            target="_blank"
            rel="noopener noreferrer"
            style="display:inline-flex;align-items:center;justify-content:center;width:20px;height:20px;background:rgba(200,184,255,.04);border:1px solid rgba(200,184,255,.08);color:var(--color-tertiary);text-decoration:none;flex-shrink:0;transition:all .15s;"
            @mouseenter="style={...$event.target.style, color:'var(--color-secondary)', backgroundColor:'rgba(200,184,255,.08)'}"
            @mouseleave="style={...$event.target.style, color:'var(--color-tertiary)', backgroundColor:'rgba(200,184,255,.04)'}"
          >
            <ExternalLink style="width:9px;height:9px;" aria-hidden="true" />
          </a>
        </div>
      </div>
    </div>

    <!-- Options row -->
    <div v-if="events.length > 0" style="display:flex;align-items:center;justify-content:space-between;padding:8px 0;border-top:1px solid rgba(200,184,255,.04);margin-bottom:10px;">
      <label style="display:flex;align-items:center;gap:5px;cursor:pointer;">
        <input
          type="checkbox"
          v-model="dryRun"
          style="width:12px;height:12px;accent-color:var(--color-primary);cursor:pointer;"
        />
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);">
          DRY RUN
        </span>
      </label>
      <span style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);">
        (preview without creating gigs)
      </span>
    </div>

    <!-- Import button -->
    <div v-if="events.length > 0" style="margin-bottom:10px;">
      <button
        class="btn-hud btn-hud-violet luminous-threshold"
        :disabled="!hasSelection || importing"
        style="width:100%;height:30px;display:inline-flex;align-items:center;justify-content:center;gap:6px;"
        @click="importEvents"
      >
        <Loader
          v-if="importing"
          class="animate-spin"
          style="width:12px;height:12px;"
          aria-hidden="true"
        />
        <Plus
          v-else
          style="width:12px;height:12px;"
          aria-hidden="true"
        />
        <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;">
          {{ dryRun ? 'PREVIEW IMPORT' : 'IMPORT EVENTS' }}
        </span>
      </button>
    </div>

    <!-- Import result -->
    <div v-if="importResult" style="margin-bottom:10px;">
      <div style="padding:10px 12px;background:var(--color-surface-container-lowest);border:1px solid rgba(150,248,255,.06);">
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:6px;">
          <span style="display:inline-flex;align-items:center;gap:4px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.04em;color:var(--color-secondary);">
            <Check style="width:9px;height:9px;" aria-hidden="true" />
            {{ importResult.gigsCreated }} gigs created
          </span>
          <span
            v-if="importResult.gigsSkipped > 0"
            style="display:inline-flex;align-items:center;gap:4px;font-family:var(--font-terminal);font-size:9px;letter-spacing:.04em;color:var(--color-error);"
          >
            <AlertCircle style="width:9px;height:9px;" aria-hidden="true" />
            {{ importResult.gigsSkipped }} skipped
          </span>
        </div>
        <div v-if="importResult.skippedReasons.length > 0" style="display:flex;flex-direction:column;gap:2px;">
          <p
            v-for="(reason, idx) in importResult.skippedReasons.slice(0, 3)"
            :key="idx"
            style="font-family:var(--font-data);font-size:9px;color:var(--color-tertiary);margin:0;"
          >
            • {{ reason }}
          </p>
          <p
            v-if="importResult.skippedReasons.length > 3"
            style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);margin:0;"
          >
            ...and {{ importResult.skippedReasons.length - 3 }} more
          </p>
        </div>
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="importing && events.length === 0"
      style="padding:16px;display:flex;align-items:center;justify-content:center;gap:8px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.06);"
    >
      <Loader class="animate-spin" style="width:14px;height:14px;color:var(--color-secondary);" aria-hidden="true" />
      <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-secondary);">
        FETCHING EVENTS FROM RA...
      </span>
    </div>

    <!-- Error -->
    <div
      v-if="raStore.error && !importing"
      style="display:flex;align-items:center;gap:6px;padding:8px 10px;background:rgba(255,113,108,.05);border:1px solid rgba(255,113,108,.1);"
    >
      <AlertCircle style="width:12px;height:12px;color:var(--color-error);flex-shrink:0;" aria-hidden="true" />
      <span style="font-family:var(--font-data);font-size:10px;color:var(--color-error);">{{ raStore.error }}</span>
    </div>

    <!-- Footer -->
    <div style="margin-top:8px;padding-top:8px;border-top:1px solid rgba(200,184,255,.04);">
      <p style="font-family:var(--font-data);font-size:10px;color:var(--color-tertiary);line-height:1.5;margin:0;">
        Import upcoming events from Resident Advisor as gig records.
        Past events (7+ days old) are automatically skipped.
        Venues and contacts are matched or created as needed.
      </p>
    </div>
  </div>
</template>
