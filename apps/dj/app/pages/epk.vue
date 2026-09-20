<script setup lang="ts">
import { useEpkStore } from '~/stores/epk'
import { useEpkAutosave } from '~/composables/useEpkAutosave'
import { Download, Edit, Music, Globe, MessageCircle, Image } from 'lucide-vue-next'

useHead({ title: 'EPK — KlubHub DJ' })

const store = useEpkStore()
const { scheduleSave } = useEpkAutosave()

onMounted(async () => {
  await Promise.all([store.loadFromApi(), store.loadExports()])
})

// Section visibility toggles
const sectionToggles = reactive({
  bio: true,
  photos: true,
  gigs: true,
  social: true,
  rider: true,
})

// Editable fields (local mirrors for immediate reactivity)
const djName = ref('DJ PHANTOM')
const tagline = ref('TECHNO · BERLIN · EST. 2018')
const bookingContact = ref('')
const location = ref('BERLIN, DE')

// Bio from store
const bioText = computed(() => store.bioLong || store.bioShort || 'Berlin-based DJ and producer operating at the intersection of raw techno and industrial noise. Resident at Tresor since 2021.')

// Mock gig highlights (will be replaced when gig store is available)
const gigHighlights = [
  { venue: 'BERGHAIN', details: "NEW YEAR'S EVE · BERLIN, DE", date: 'DEC 31, 2025' },
  { venue: 'DEKMANTEL FESTIVAL', details: 'MAIN STAGE · AMSTERDAM, NL', date: 'AUG 2, 2025' },
  { venue: 'FABRIC LONDON', details: 'ROOM 1 · LONDON, UK', date: 'JUL 19, 2025' },
  { venue: 'TRESOR', details: 'RESIDENT NIGHT · BERLIN, DE', date: 'MAR 8, 2026' },
]

// Photo slots
const photoSlots = computed(() => {
  const paths = store.photoPaths.slice(0, 6)
  while (paths.length < 6) paths.push('')
  return paths
})

async function handleExportPdf() {
  await store.generateExport()
}

function onNameInput(e: Event) {
  djName.value = (e.target as HTMLInputElement).value
}
function onTaglineInput(e: Event) {
  tagline.value = (e.target as HTMLInputElement).value
}
function onContactInput(e: Event) {
  bookingContact.value = (e.target as HTMLInputElement).value
}
function onLocationInput(e: Event) {
  location.value = (e.target as HTMLInputElement).value
}
function onBioInput(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  store.bioShort = val
  scheduleSave({ bioShort: val })
}
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">EPK</div>
        <div class="page-sub">ELECTRONIC PRESS KIT · LIVE PREVIEW</div>
      </div>
      <div style="display:flex;gap:8px;">
        <button class="btn-hud btn-hud-ghost hidden md:inline-flex" style="padding:0 14px;" aria-label="Edit EPK">
          <Edit style="width:12px;height:12px;" aria-hidden="true" />
          EDIT
        </button>
        <button class="btn-hud btn-hud-violet" style="padding:0 14px;" aria-label="Export PDF" @click="handleExportPdf">
          <Download style="width:12px;height:12px;" aria-hidden="true" />
          EXPORT PDF
        </button>
      </div>
    </div>

    <!-- Split layout: edit controls left + live preview right -->
    <div style="flex:1;display:flex;overflow:hidden;">

      <!-- ─── Left: Edit Controls (240px, desktop only) ─── -->
      <div
        class="hidden md:flex"
        style="width:240px;min-width:240px;border-right:1px solid rgba(200,184,255,.08);overflow-y:auto;padding:14px;display:flex;flex-direction:column;gap:14px;background:rgba(22,22,24,.4);"
      >
        <!-- Section visibility label -->
        <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-secondary);margin-bottom:2px;">
          SECTIONS
        </div>

        <!-- Toggle controls -->
        <div style="display:flex;flex-direction:column;gap:8px;">
          <div
            v-for="(label, key) in { bio: 'BIO', photos: 'PRESS PHOTOS', gigs: 'GIG HIGHLIGHTS', social: 'SOCIAL LINKS', rider: 'TECH RIDER' }"
            :key="key"
            style="display:flex;align-items:center;gap:10px;cursor:pointer;"
            @click="sectionToggles[key as keyof typeof sectionToggles] = !sectionToggles[key as keyof typeof sectionToggles]"
          >
            <div
              class="hud-toggle"
              :class="{ on: sectionToggles[key as keyof typeof sectionToggles] }"
            >
              <div class="hud-toggle-thumb" />
            </div>
            <span
              style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;"
              :style="sectionToggles[key as keyof typeof sectionToggles] ? 'color:var(--color-on-surface)' : 'color:var(--color-tertiary)'"
            >{{ label }}</span>
          </div>
        </div>

        <div class="section-divider" />

        <!-- Bio editor -->
        <div>
          <div class="input-label">DJ NAME</div>
          <input class="hud-input" :value="djName" style="margin-bottom:8px;" @input="onNameInput" >
          <div class="input-label">TAGLINE</div>
          <input class="hud-input" :value="tagline" style="margin-bottom:8px;" @input="onTaglineInput" >
          <div class="input-label">SHORT BIO</div>
          <textarea
            class="hud-textarea"
            :value="store.bioShort || ''"
            style="min-height:70px;"
            @input="onBioInput"
          />
        </div>

        <div class="section-divider" />

        <!-- Contact -->
        <div>
          <div class="input-label">BOOKING CONTACT</div>
          <input class="hud-input" :value="bookingContact" placeholder="EMAIL OR AGENCY" style="margin-bottom:8px;" @input="onContactInput" >
          <div class="input-label">LOCATION</div>
          <input class="hud-input" :value="location" @input="onLocationInput" >
        </div>

        <div class="section-divider" />

        <!-- RA Import -->
        <EpkRaImportPanel />

        <div class="section-divider" />

        <button class="btn-hud btn-hud-violet" style="width:100%;padding:0 14px;" @click="handleExportPdf">
          <Download style="width:12px;height:12px;" aria-hidden="true" />
          EXPORT PDF
        </button>
        <button class="btn-hud btn-hud-ghost" style="width:100%;padding:0 14px;">
          SHARE LINK
        </button>
      </div>

      <!-- ─── Right: EPK Live Preview ─── -->
      <div
        style="flex:1;overflow-y:auto;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.12);"
      >

        <!-- Hero section -->
        <div class="epk-preview-hero">
          <!-- Background accent shape -->
          <div style="position:absolute;inset:0;display:flex;align-items:center;justify-content:flex-end;padding-right:24px;opacity:.15;pointer-events:none;">
            <div style="width:140px;height:180px;background:linear-gradient(160deg,var(--color-secondary-container),transparent);clip-path:polygon(0 0,100% 0,100% 90%,80% 100%,0 100%);"/>
          </div>
          <div style="position:relative;">
            <div class="epk-dj-name">{{ djName.includes('\n') ? djName : djName.replace(' ', '\n') }}</div>
            <div class="epk-tagline">{{ tagline }}</div>
            <div style="display:flex;gap:10px;margin-top:14px;flex-wrap:wrap;">
              <span class="data-frag" style="color:var(--color-secondary);background:rgba(200,184,255,.08);">TECHNO</span>
              <span class="data-frag" style="color:var(--color-secondary);background:rgba(200,184,255,.08);">INDUSTRIAL</span>
              <span class="data-frag" style="color:var(--color-secondary);background:rgba(200,184,255,.08);">BERLIN</span>
              <span class="data-frag" style="color:var(--color-secondary);background:rgba(200,184,255,.08);">TRESOR RESIDENT</span>
            </div>
          </div>
        </div>

        <!-- Bio section -->
        <div
          v-if="sectionToggles.bio"
          style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);display:flex;flex-direction:column;gap:10px;"
        >
          <div class="glass-violet hud-card hud-card-v" style="padding:16px 18px;">
            <div class="epk-preview-label">BIO</div>
            <div class="epk-preview-text">{{ bioText }}</div>
          </div>
        </div>

        <!-- Press Photos -->
        <div
          v-if="sectionToggles.photos"
          style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);display:flex;flex-direction:column;gap:10px;"
        >
          <div class="epk-preview-label">PRESS PHOTOS</div>
          <div class="photo-grid">
            <div
              v-for="(path, idx) in photoSlots"
              :key="idx"
              class="photo-slot"
            >
              <template v-if="path">
                <img :src="path" :alt="`Press photo ${idx + 1}`" style="width:100%;height:100%;object-fit:cover;" >
              </template>
              <template v-else>
                <Image style="width:20px;height:20px;stroke:var(--color-tertiary);stroke-width:1.5;" aria-hidden="true" />
                <span style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);">UPLOAD</span>
              </template>
            </div>
          </div>
        </div>

        <!-- Gig Highlights -->
        <div
          v-if="sectionToggles.gigs"
          style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);display:flex;flex-direction:column;gap:10px;"
        >
          <div style="display:flex;justify-content:space-between;align-items:center;">
            <div class="epk-preview-label">GIG HIGHLIGHTS</div>
            <button class="btn-hud btn-hud-ghost btn-hud-xs" style="border-color:rgba(200,184,255,.2);color:var(--color-secondary);padding:0 8px;">
              IMPORT FROM GIGS
            </button>
          </div>
          <div class="glass-violet" style="overflow:hidden;">
            <div
              v-for="gig in gigHighlights"
              :key="gig.venue"
              class="gig-row accent-bar-draft"
            >
              <div style="flex:1;">
                <div class="gig-venue">{{ gig.venue }}</div>
                <div class="spost-meta" style="margin-top:2px;">{{ gig.details }}</div>
              </div>
              <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);">{{ gig.date }}</span>
            </div>
          </div>
        </div>

        <!-- Social Links -->
        <div
          v-if="sectionToggles.social"
          style="padding:16px 20px;border-bottom:1px solid rgba(200,184,255,.06);display:flex;flex-direction:column;gap:10px;"
        >
          <div class="epk-preview-label">SOCIAL &amp; STREAMING</div>
          <div style="display:flex;flex-wrap:wrap;gap:6px;">
            <div class="social-chip">
              <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="5"/><circle cx="12" cy="12" r="4"/><circle cx="17.5" cy="6.5" r="1"/></svg>
              @DJPHANTOM
            </div>
            <div class="social-chip">
              <Music style="width:11px;height:11px;" aria-hidden="true" />
              SOUNDCLOUD
            </div>
            <div class="social-chip">
              <Globe style="width:11px;height:11px;" aria-hidden="true" />
              DJPHANTOM.COM
            </div>
            <div class="social-chip">
              <MessageCircle style="width:11px;height:11px;" aria-hidden="true" />
              RA / DJPHANTOM
            </div>
          </div>
        </div>

        <!-- Tech Rider -->
        <div
          v-if="sectionToggles.rider"
          style="padding:16px 20px 24px;display:flex;flex-direction:column;gap:10px;"
        >
          <div class="epk-preview-label">TECH RIDER</div>
          <div class="glass-violet hud-card hud-card-v" style="padding:14px 16px;">
            <div style="display:flex;flex-direction:column;gap:8px;">
              <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;">
                <div>
                  <div class="epk-preview-label" style="margin-bottom:3px;">DECKS</div>
                  <div class="epk-preview-text" style="font-size:11px;">2× Pioneer CDJ-3000 or CDJ-2000NXS2</div>
                </div>
                <div>
                  <div class="epk-preview-label" style="margin-bottom:3px;">MIXER</div>
                  <div class="epk-preview-text" style="font-size:11px;">Pioneer DJM-900NXS2</div>
                </div>
                <div>
                  <div class="epk-preview-label" style="margin-bottom:3px;">MONITOR</div>
                  <div class="epk-preview-text" style="font-size:11px;">Dedicated monitor speaker at DJ booth</div>
                </div>
                <div>
                  <div class="epk-preview-label" style="margin-bottom:3px;">MEDIA</div>
                  <div class="epk-preview-text" style="font-size:11px;">USB sticks (no laptop required)</div>
                </div>
              </div>
              <div class="prog-track" style="margin-top:4px;" />
              <div>
                <div class="epk-preview-label" style="margin-bottom:3px;">NOTES</div>
                <div class="epk-preview-text" style="font-size:11px;">{{ store.techRider || 'Pioneer ecosystem strictly required. No Denon/Traktor hardware. Please confirm technical setup 48 hours in advance.' }}</div>
              </div>
            </div>
          </div>
          <div style="margin-top:4px;">
            <button class="btn-hud btn-hud-ghost btn-hud-sm" style="padding:0 14px;">
              <Download style="width:11px;height:11px;" aria-hidden="true" />
              DOWNLOAD RIDER PDF
            </button>
          </div>
        </div>

      </div>
    </div>

  </div>
</template>
