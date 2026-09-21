<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useEpkStore } from '~/stores/epk'
import { useSettingsStore } from '~/stores/settings'
import { useGigStore } from '~/stores/gig'
import { useEpkAutosave } from '~/composables/useEpkAutosave'
import { Download, ExternalLink, Image, Loader, Plus, Trash2, X, Check, AlertCircle } from 'lucide-vue-next'

useHead({ title: 'EPK — KlubHub DJ' })

const store = useEpkStore()
const settings = useSettingsStore()
const gigStore = useGigStore()
const { scheduleSave, flush } = useEpkAutosave()

// Don't lose an edit made just before navigating away.
onBeforeUnmount(() => { void flush() })

const { bioShort, bioLong, techRider, gigHighlights, photoPaths, photoUrls, sectionVisibility, canAddPhoto } = storeToRefs(store)
const { djName, socialLinks, contactInfo } = storeToRefs(settings)

const loading = ref(true)
const loadError = ref('')

onMounted(async () => {
  try {
    await Promise.all([store.loadFromApi(), store.loadExports(), settings.loadFromApi()])
  } catch {
    loadError.value = 'Could not load your press kit. Check that the API is running, then reload.'
  } finally {
    loading.value = false
  }
})

// ── Sections (persisted; the same keys drive the PDF export) ──────────────
const SECTIONS = [
  { key: 'bio', label: 'BIO' },
  { key: 'photos', label: 'PRESS PHOTOS' },
  { key: 'gigHighlights', label: 'GIG HIGHLIGHTS' },
  { key: 'socialLinks', label: 'SOCIAL LINKS' },
  { key: 'techRider', label: 'TECH RIDER' },
] as const
type SectionKey = typeof SECTIONS[number]['key']

function isVisible(key: SectionKey): boolean {
  return sectionVisibility.value[key] !== false
}
function toggleSection(key: SectionKey) {
  const next = { ...sectionVisibility.value, [key]: !isVisible(key) }
  sectionVisibility.value = next
  scheduleSave({ sectionVisibility: next })
}

// ── EPK content fields (autosaved) ────────────────────────────────────────
const SHORT_BIO_MAX = 200

function onShortBio(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  bioShort.value = val
  scheduleSave({ bioShort: val })
}
function onLongBio(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  bioLong.value = val
  scheduleSave({ bioLong: val })
}
function onTechRider(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  techRider.value = val
  scheduleSave({ techRider: val })
}

// ── Settings-backed fields (name, contact, links), debounced together ─────
const SOCIAL_FIELDS = [
  { key: 'instagram', label: 'INSTAGRAM', placeholder: 'https://instagram.com/you' },
  { key: 'soundcloud', label: 'SOUNDCLOUD', placeholder: 'https://soundcloud.com/you' },
  { key: 'bandcamp', label: 'BANDCAMP', placeholder: 'https://you.bandcamp.com' },
  { key: 'residentAdvisor', label: 'RESIDENT ADVISOR', placeholder: 'https://ra.co/dj/you' },
  { key: 'website', label: 'WEBSITE', placeholder: 'https://you.com' },
] as const

let settingsTimer: ReturnType<typeof setTimeout> | null = null
let pendingSettings: Parameters<typeof settings.save>[0] = {}

function queueSettings(patch: Parameters<typeof settings.save>[0]) {
  pendingSettings = { ...pendingSettings, ...patch }
  settings.saveStatus = 'saving'
  if (settingsTimer) clearTimeout(settingsTimer)
  settingsTimer = setTimeout(async () => {
    const patchToSend = pendingSettings
    pendingSettings = {}
    try {
      await settings.save(patchToSend)
    } catch {
      // saveStatus is 'error'; the status chip tells the user
    }
  }, 900)
}

function onDjName(e: Event) {
  djName.value = (e.target as HTMLInputElement).value
  queueSettings({ dj_name: djName.value })
}
function onContact(e: Event) {
  contactInfo.value = (e.target as HTMLInputElement).value
  queueSettings({ contact_info: contactInfo.value })
}
function onSocial(key: string, e: Event) {
  socialLinks.value = { ...socialLinks.value, [key]: (e.target as HTMLInputElement).value.trim() }
  queueSettings({ social_links: socialLinks.value })
}

// ── Combined save status ──────────────────────────────────────────────────
const saveState = computed(() => {
  const states = [store.saveStatus, settings.saveStatus]
  if (states.includes('error')) return 'error'
  if (states.includes('saving')) return 'saving'
  if (states.includes('saved')) return 'saved'
  return 'idle'
})

// ── Photos ────────────────────────────────────────────────────────────────
const fileInput = ref<HTMLInputElement>()
const uploading = ref(false)
const photoError = ref('')
const confirmDeletePath = ref<string | null>(null)

async function onPhotosSelected(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  if (files.length === 0) return
  uploading.value = true
  photoError.value = ''
  try {
    for (const file of files) await store.uploadPhoto(file)
  } catch (err) {
    const body = (err as { data?: { error?: string; message?: string } } | null)?.data
    photoError.value = body?.error || body?.message || 'Upload failed. Use a JPEG, PNG or WebP image.'
  } finally {
    uploading.value = false
    input.value = ''
  }
}

async function deletePhoto(path: string) {
  // Two-step: first click arms, second click deletes.
  if (confirmDeletePath.value !== path) {
    confirmDeletePath.value = path
    return
  }
  confirmDeletePath.value = null
  try {
    await store.deletePhoto(path)
  } catch {
    photoError.value = 'Could not remove that photo. Try again.'
  }
}

// ── Gig highlights ────────────────────────────────────────────────────────
const newHighlight = ref('')
const importingGigs = ref(false)
const gigImportNote = ref('')

function setHighlights(next: string[]) {
  gigHighlights.value = next
  scheduleSave({ gigHighlights: next })
}
function addHighlight() {
  const val = newHighlight.value.trim()
  if (!val) return
  setHighlights([...gigHighlights.value, val])
  newHighlight.value = ''
}
function removeHighlight(idx: number) {
  setHighlights(gigHighlights.value.filter((_, i) => i !== idx))
}

async function importFromGigs() {
  importingGigs.value = true
  gigImportNote.value = ''
  try {
    await gigStore.fetchGigs()
    const lines = [...gigStore.gigs]
      .filter((g) => g.status !== 'cancelled')
      .sort((a, b) => (a.date < b.date ? 1 : -1))
      .slice(0, 8)
      .map((g) => {
        const place = [g.city, g.country].filter(Boolean).join(', ')
        const when = new Date(g.date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' })
        return [g.event_name || g.venue, place, when].filter(Boolean).join(' · ')
      })
    const fresh = lines.filter((l) => !gigHighlights.value.includes(l))
    if (fresh.length === 0) {
      gigImportNote.value = lines.length === 0 ? 'No gigs in your tracker yet.' : 'All your gigs are already listed.'
      return
    }
    setHighlights([...gigHighlights.value, ...fresh])
    gigImportNote.value = `Added ${fresh.length} gig${fresh.length === 1 ? '' : 's'}.`
  } finally {
    importingGigs.value = false
  }
}

// ── Preview helpers ───────────────────────────────────────────────────────
const previewBio = computed(() => bioLong.value || bioShort.value)

// Hero readouts: label/value pairs, only for data that exists.
const heroMeta = computed(() => {
  const items: { label: string; value: string }[] = []
  if (contactInfo.value) items.push({ label: 'BOOKING', value: contactInfo.value })
  const ra = socialLinks.value.residentAdvisor
  if (ra) items.push({ label: 'RA', value: ra.replace(/^https?:\/\/(www\.)?/, '').replace(/\/$/, '') })
  return items
})
const activeLinks = computed(() =>
  Object.entries(socialLinks.value)
    .filter(([, url]) => !!url)
    .map(([key, url]) => ({
      key,
      url,
      label: url.replace(/^https?:\/\/(www\.)?/, '').replace(/\/$/, ''),
    })),
)

// ── Export ────────────────────────────────────────────────────────────────
const exporting = ref(false)
const exportUrl = ref('')
const exportError = ref('')

async function handleExportPdf() {
  if (exporting.value) return
  exporting.value = true
  exportUrl.value = ''
  exportError.value = ''
  try {
    const result = await store.generateExport()
    exportUrl.value = result.downloadUrl
    await store.loadExports()
  } catch {
    exportError.value = 'Export failed. Try again in a moment.'
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">EPK</div>
        <div class="page-sub">ELECTRONIC PRESS KIT · AUTOSAVED · LIVE PREVIEW</div>
      </div>
      <div style="display:flex;gap:10px;align-items:center;">
        <!-- Save status: autosave must never be silent -->
        <span
          v-if="saveState !== 'idle'"
          role="status"
          class="section-lbl"
          data-testid="epk-save-status"
          style="display:inline-flex;align-items:center;gap:5px;"
          :style="saveState === 'error' ? 'color:var(--color-error)' : 'color:var(--color-tertiary)'"
        >
          <Loader v-if="saveState === 'saving'" class="animate-spin" style="width:10px;height:10px;" aria-hidden="true" />
          <Check v-else-if="saveState === 'saved'" style="width:10px;height:10px;" aria-hidden="true" />
          <AlertCircle v-else style="width:10px;height:10px;" aria-hidden="true" />
          {{ saveState === 'saving' ? 'SAVING…' : saveState === 'saved' ? 'ALL CHANGES SAVED' : 'NOT SAVED — RETRY BY EDITING' }}
        </span>
        <button
          class="btn-hud btn-hud-violet"
          style="padding:0 14px;"
          :disabled="exporting"
          @click="handleExportPdf"
        >
          <Loader v-if="exporting" class="animate-spin" style="width:12px;height:12px;" aria-hidden="true" />
          <Download v-else style="width:12px;height:12px;" aria-hidden="true" />
          {{ exporting ? 'BUILDING PDF…' : 'EXPORT PDF' }}
        </button>
      </div>
    </div>

    <!-- Export result -->
    <div
      v-if="exportUrl || exportError"
      :role="exportError ? 'alert' : 'status'"
      data-testid="epk-export-result"
      style="display:flex;align-items:center;gap:10px;padding:9px 20px;border-bottom:1px solid rgba(200,184,255,.08);"
      :style="exportError ? 'background:rgba(255,113,108,.06)' : 'background:rgba(150,248,255,.04)'"
    >
      <span class="section-lbl" :style="exportError ? 'color:var(--color-error)' : 'color:var(--color-primary)'">
        {{ exportError || 'YOUR PDF IS READY' }}
      </span>
      <a
        v-if="exportUrl"
        :href="exportUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="btn-hud btn-hud-ghost btn-hud-xs"
        style="padding:0 10px;text-decoration:none;"
      >
        <ExternalLink style="width:10px;height:10px;" aria-hidden="true" />
        OPEN PDF
      </a>
      <button
        class="btn-hud btn-hud-ghost btn-hud-xs"
        style="margin-left:auto;padding:0 6px;"
        aria-label="Dismiss"
        @click="exportUrl = ''; exportError = ''"
      >
        <X style="width:10px;height:10px;" aria-hidden="true" />
      </button>
    </div>

    <!-- Load failure -->
    <div v-if="loadError" role="alert" style="padding:24px 20px;color:var(--color-error);font-family:var(--font-data);font-size:13px;">
      {{ loadError }}
    </div>

    <!--
      Split layout. Desktop: two independently scrolling panes.
      Mobile: one scrolling column, editor first — the editor used to be
      hidden below md, leaving phones with a read-only page.
    -->
    <div v-else class="flex-1 flex flex-col md:flex-row overflow-y-auto md:overflow-hidden">

      <!-- ─── Editor ─── -->
      <div
        class="w-full md:w-[320px] md:min-w-[320px] md:overflow-y-auto"
        style="border-right:1px solid rgba(200,184,255,.08);padding:14px;display:flex;flex-direction:column;gap:14px;background:rgba(22,22,24,.4);"
      >
        <!-- Quick start -->
        <EpkRaImportPanel />

        <div class="section-divider" />

        <!-- Profile -->
        <div>
          <label class="input-label" for="epk-dj-name">DJ NAME</label>
          <input
            id="epk-dj-name"
            class="hud-input"
            :value="djName"
            placeholder="Your artist name"
            autocomplete="off"
            style="margin-bottom:10px;"
            @input="onDjName"
          >

          <div style="display:flex;justify-content:space-between;align-items:baseline;">
            <label class="input-label" for="epk-bio-short">SHORT BIO</label>
            <span
              class="section-lbl"
              :style="bioShort.length > SHORT_BIO_MAX ? 'color:var(--color-error)' : 'color:var(--color-tertiary)'"
            >{{ bioShort.length }} / {{ SHORT_BIO_MAX }}</span>
          </div>
          <textarea
            id="epk-bio-short"
            class="hud-textarea"
            :value="bioShort"
            placeholder="One or two lines a promoter can paste into an event page."
            style="min-height:64px;margin-bottom:10px;"
            @input="onShortBio"
          />

          <label class="input-label" for="epk-bio-long">FULL BIO</label>
          <textarea
            id="epk-bio-long"
            class="hud-textarea"
            :value="bioLong"
            placeholder="Your story: sound, residencies, releases."
            style="min-height:110px;"
            @input="onLongBio"
          />
        </div>

        <div class="section-divider" />

        <!-- Contact + links -->
        <div>
          <label class="input-label" for="epk-contact">BOOKING CONTACT</label>
          <input
            id="epk-contact"
            class="hud-input"
            :value="contactInfo"
            placeholder="Email or agency"
            style="margin-bottom:12px;"
            @input="onContact"
          >

          <div class="input-label" style="color:var(--color-secondary);">SOCIAL LINKS</div>
          <div v-for="field in SOCIAL_FIELDS" :key="field.key" style="margin-top:6px;">
            <label class="input-label" :for="`epk-social-${field.key}`">{{ field.label }}</label>
            <input
              :id="`epk-social-${field.key}`"
              class="hud-input"
              type="url"
              inputmode="url"
              :value="socialLinks[field.key] || ''"
              :placeholder="field.placeholder"
              @input="onSocial(field.key, $event)"
            >
          </div>
        </div>

        <div class="section-divider" />

        <!-- Photos -->
        <div>
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px;">
            <div class="input-label" style="margin:0;">PRESS PHOTOS · {{ photoPaths.length }}/20</div>
            <button
              class="btn-hud btn-hud-ghost btn-hud-xs"
              style="padding:0 8px;"
              :disabled="uploading || !canAddPhoto"
              @click="fileInput?.click()"
            >
              <Loader v-if="uploading" class="animate-spin" style="width:10px;height:10px;" aria-hidden="true" />
              <Plus v-else style="width:10px;height:10px;" aria-hidden="true" />
              {{ uploading ? 'UPLOADING…' : 'ADD PHOTOS' }}
            </button>
            <input
              ref="fileInput"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              multiple
              class="sr-only"
              aria-label="Add press photos"
              @change="onPhotosSelected"
            >
          </div>
          <p v-if="photoError" role="alert" style="font-family:var(--font-data);font-size:11px;color:var(--color-error);margin:0 0 6px;">
            {{ photoError }}
          </p>
          <div v-if="photoPaths.length > 0" style="display:grid;grid-template-columns:repeat(3,1fr);gap:6px;">
            <div v-for="path in photoPaths" :key="path" style="position:relative;aspect-ratio:1;overflow:hidden;border:1px solid rgba(200,184,255,.12);">
              <img :src="photoUrls[path]" alt="Press photo" style="width:100%;height:100%;object-fit:cover;">
              <button
                class="btn-hud btn-hud-xs"
                :class="confirmDeletePath === path ? 'btn-hud-error' : 'btn-hud-ghost'"
                style="position:absolute;right:3px;bottom:3px;padding:0 6px;min-height:24px;"
                :aria-label="confirmDeletePath === path ? 'Confirm remove photo' : 'Remove photo'"
                @click="deletePhoto(path)"
                @blur="confirmDeletePath = null"
              >
                <template v-if="confirmDeletePath === path">REMOVE?</template>
                <Trash2 v-else style="width:10px;height:10px;" aria-hidden="true" />
              </button>
            </div>
          </div>
          <p v-else style="font-family:var(--font-data);font-size:11px;color:var(--color-tertiary);margin:0;line-height:1.5;">
            Promoters want 2–3 high-res shots. JPEG, PNG or WebP.
          </p>
        </div>

        <div class="section-divider" />

        <!-- Gig highlights -->
        <div>
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px;">
            <div class="input-label" style="margin:0;">GIG HIGHLIGHTS</div>
            <button
              class="btn-hud btn-hud-ghost btn-hud-xs"
              style="padding:0 8px;"
              :disabled="importingGigs"
              @click="importFromGigs"
            >
              <Loader v-if="importingGigs" class="animate-spin" style="width:10px;height:10px;" aria-hidden="true" />
              IMPORT FROM GIGS
            </button>
          </div>
          <p v-if="gigImportNote" role="status" class="section-lbl" style="color:var(--color-secondary);margin:0 0 6px;">
            {{ gigImportNote }}
          </p>
          <div
            v-for="(line, idx) in gigHighlights"
            :key="`${idx}-${line}`"
            style="display:flex;align-items:center;gap:6px;padding:5px 0;border-bottom:1px solid rgba(200,184,255,.06);"
          >
            <span style="flex:1;font-family:var(--font-data);font-size:11px;color:var(--color-on-surface);line-height:1.4;">{{ line }}</span>
            <button
              class="btn-hud btn-hud-ghost btn-hud-xs"
              style="padding:0 6px;min-height:24px;"
              :aria-label="`Remove highlight: ${line}`"
              @click="removeHighlight(idx)"
            >
              <X style="width:10px;height:10px;" aria-hidden="true" />
            </button>
          </div>
          <form style="display:flex;gap:6px;margin-top:8px;" @submit.prevent="addHighlight">
            <label class="sr-only" for="epk-new-highlight">Add a gig highlight</label>
            <input
              id="epk-new-highlight"
              v-model="newHighlight"
              class="hud-input"
              placeholder="Venue · City · Date"
              style="flex:1;"
            >
            <button type="submit" class="btn-hud btn-hud-ghost btn-hud-sm" style="padding:0 10px;" :disabled="!newHighlight.trim()">
              ADD
            </button>
          </form>
        </div>

        <div class="section-divider" />

        <!-- Tech rider -->
        <div>
          <label class="input-label" for="epk-rider">TECH RIDER</label>
          <textarea
            id="epk-rider"
            class="hud-textarea"
            :value="techRider"
            placeholder="Decks, mixer, monitoring, anything the venue must provide."
            style="min-height:90px;"
            @input="onTechRider"
          />
        </div>

        <div class="section-divider" />

        <!-- Sections included in preview + PDF -->
        <div>
          <div class="input-label" style="color:var(--color-secondary);">INCLUDE IN PRESS KIT</div>
          <div style="display:flex;flex-direction:column;gap:4px;margin-top:6px;">
            <button
              v-for="section in SECTIONS"
              :key="section.key"
              type="button"
              role="switch"
              :aria-checked="isVisible(section.key)"
              style="display:flex;align-items:center;gap:10px;cursor:pointer;background:none;border:0;padding:6px 0;min-height:32px;text-align:left;"
              @click="toggleSection(section.key)"
            >
              <span class="hud-toggle" :class="{ on: isVisible(section.key) }" aria-hidden="true">
                <span class="hud-toggle-thumb" />
              </span>
              <span
                style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;"
                :style="isVisible(section.key) ? 'color:var(--color-on-surface)' : 'color:var(--color-tertiary)'"
              >{{ section.label }}</span>
            </button>
          </div>
        </div>
      </div>

      <!-- ─── Live preview (real data only) ─── -->
      <div
        class="md:flex-1 md:overflow-y-auto"
        style="background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.12);"
        aria-label="Press kit preview"
      >
        <div v-if="loading" style="padding:40px 20px;display:flex;align-items:center;gap:10px;color:var(--color-tertiary);">
          <Loader class="animate-spin" style="width:14px;height:14px;" aria-hidden="true" />
          <span class="section-lbl">LOADING YOUR PRESS KIT…</span>
        </div>

        <template v-else>
          <!-- Hero -->
          <div class="epk-preview-hero">
            <div class="epk-eyebrow">ELECTRONIC PRESS KIT</div>
            <div class="epk-dj-name" :style="djName ? '' : 'color:var(--color-tertiary)'">{{ djName || 'YOUR DJ NAME' }}</div>
            <div v-if="heroMeta.length > 0" class="epk-meta">
              <span v-for="item in heroMeta" :key="item.label" class="epk-meta-item">
                <span class="epk-meta-label">{{ item.label }}</span>
                <span class="epk-meta-value">{{ item.value }}</span>
              </span>
            </div>
          </div>

          <!-- Bio -->
          <div v-if="isVisible('bio')" style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);">
            <div class="glass-violet hud-card hud-card-v" style="padding:16px 18px;">
              <div class="epk-preview-label">BIO</div>
              <div v-if="previewBio" class="epk-preview-text" style="white-space:pre-line;">{{ previewBio }}</div>
              <div v-else class="epk-preview-text" style="color:var(--color-tertiary);">
                No bio yet. Write one on the left, or import it from Resident Advisor.
              </div>
            </div>
          </div>

          <!-- Photos -->
          <div v-if="isVisible('photos')" style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);display:flex;flex-direction:column;gap:10px;">
            <div class="epk-preview-label">PRESS PHOTOS</div>
            <div v-if="photoPaths.length > 0" class="photo-grid">
              <div v-for="path in photoPaths.slice(0, 6)" :key="path" class="photo-slot">
                <img :src="photoUrls[path]" alt="Press photo" style="width:100%;height:100%;object-fit:cover;">
              </div>
            </div>
            <button
              v-else
              type="button"
              class="photo-slot"
              style="width:100%;aspect-ratio:auto;padding:22px 12px;cursor:pointer;gap:6px;"
              @click="fileInput?.click()"
            >
              <Image style="width:20px;height:20px;stroke:var(--color-tertiary);stroke-width:1.5;" aria-hidden="true" />
              <span class="section-lbl" style="color:var(--color-tertiary);">NO PHOTOS YET — CLICK TO ADD</span>
            </button>
          </div>

          <!-- Gig highlights -->
          <div v-if="isVisible('gigHighlights')" style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);display:flex;flex-direction:column;gap:10px;">
            <div class="epk-preview-label">GIG HIGHLIGHTS</div>
            <div v-if="gigHighlights.length > 0" class="glass-violet" style="overflow:hidden;">
              <div v-for="(line, idx) in gigHighlights" :key="`${idx}-${line}`" class="gig-row accent-bar-draft">
                <div class="gig-venue" style="flex:1;">{{ line }}</div>
              </div>
            </div>
            <div v-else class="epk-preview-text" style="color:var(--color-tertiary);">
              No highlights yet. Use “Import from gigs” to pull them from your gig tracker.
            </div>
          </div>

          <!-- Social links -->
          <div v-if="isVisible('socialLinks')" style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);display:flex;flex-direction:column;gap:10px;">
            <div class="epk-preview-label">SOCIAL &amp; STREAMING</div>
            <div v-if="activeLinks.length > 0" style="display:flex;flex-wrap:wrap;gap:6px;">
              <a
                v-for="link in activeLinks"
                :key="link.key"
                :href="link.url"
                target="_blank"
                rel="noopener noreferrer"
                class="social-chip"
                style="text-decoration:none;"
              >
                <ExternalLink style="width:11px;height:11px;" aria-hidden="true" />
                {{ link.label }}
              </a>
            </div>
            <div v-else class="epk-preview-text" style="color:var(--color-tertiary);">
              No links yet. Add them on the left.
            </div>
          </div>

          <!-- Tech rider -->
          <div v-if="isVisible('techRider')" style="padding:16px 20px 24px;display:flex;flex-direction:column;gap:10px;">
            <div class="epk-preview-label">TECH RIDER</div>
            <div class="glass-violet hud-card hud-card-v" style="padding:14px 16px;">
              <div v-if="techRider" class="epk-preview-text" style="font-size:12px;white-space:pre-line;">{{ techRider }}</div>
              <div v-else class="epk-preview-text" style="color:var(--color-tertiary);">
                No tech rider yet. List the decks, mixer and monitoring you need.
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

  </div>
</template>
