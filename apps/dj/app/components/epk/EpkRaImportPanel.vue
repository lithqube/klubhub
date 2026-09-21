<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRaStore } from '~/stores/ra'
import { useEpkStore } from '~/stores/epk'
import { useSettingsStore } from '~/stores/settings'
import Input from '~/components/ui/input/Input.vue'
import { Search, ExternalLink, Check, AlertCircle, Loader, X } from 'lucide-vue-next'

const raStore = useRaStore()
const epkStore = useEpkStore()
const settingsStore = useSettingsStore()

const artistSlug = ref('')
const clearTrigger = ref(0)

const importedArtist = ref<typeof raStore.artist>(null)
const importing = ref(false)
// Apply feedback: the button used to do its work (or fail) in total silence.
const applyState = ref<'idle' | 'applying' | 'applied' | 'error'>('idle')
const applyError = ref('')
const appliedSummary = ref('')

watch(() => raStore.artist, (artist) => {
  if (artist) {
    importedArtist.value = artist
    importing.value = false
  }
})

async function fetchArtist() {
  if (!artistSlug.value.trim()) return
  importing.value = true
  importedArtist.value = null
  applyState.value = 'idle'
  try {
    await raStore.fetchArtist(artistSlug.value.trim())
  } finally {
    // The watcher only fires when an artist arrives; on a failed fetch the
    // spinner used to spin forever.
    importing.value = false
  }
}

async function applyToEpk() {
  const artist = importedArtist.value
  if (!artist || applyState.value === 'applying') return

  applyState.value = 'applying'
  applyError.value = ''
  try {
    const applied: string[] = []

    // Bio lives in the EPK content. Setting the store refs alone changed
    // nothing on the server, so the import vanished on reload.
    if (artist.biography) {
      await epkStore.updateContent({
        bioLong: artist.biography,
        bioShort: artist.biography.substring(0, 200),
      })
      applied.push('bio')
    }

    // Name and links live in settings. Only overwrite links RA actually has,
    // so a manually entered Spotify/YouTube link is not blanked by an import.
    const fromRa: Record<string, string> = {
      instagram: artist.instagram,
      soundcloud: artist.soundcloud,
      bandcamp: artist.bandcamp,
      residentAdvisor: artist.url,
      twitter: artist.twitter,
      facebook: artist.facebook,
      discogs: artist.discogs,
      website: artist.website,
    }
    const links = { ...settingsStore.socialLinks }
    let linkCount = 0
    for (const [key, value] of Object.entries(fromRa)) {
      if (value) {
        links[key] = value
        linkCount++
      }
    }
    await settingsStore.save({ dj_name: artist.name, social_links: links })
    applied.push('name')
    if (linkCount > 0) applied.push(`${linkCount} link${linkCount === 1 ? '' : 's'}`)

    appliedSummary.value = applied.join(' · ')
    applyState.value = 'applied'
  } catch (e) {
    const body = (e as { data?: { message?: string; error?: string } } | null)?.data
    applyError.value = body?.message || body?.error || 'Could not apply the import. Try again.'
    applyState.value = 'error'
  }
}

function clearForm() {
  artistSlug.value = ''
  raStore.clear()
  importedArtist.value = null
  applyState.value = 'idle'
  clearTrigger.value++
}
</script>

<template>
  <div class="glass-panel" style="padding:14px 16px;">
    <!-- Header -->
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:14px;">
      <div class="flex items-center gap-2" style="gap:8px;">
        <div style="width:20px;height:20px;display:flex;align-items:center;justify-content:center;border:1px solid rgba(200,184,255,.2);border-radius:4px;background:rgba(200,184,255,.04);">
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" style="color:var(--color-secondary);">
            <circle cx="12" cy="12" r="10"/>
            <circle cx="12" cy="12" r="3"/>
            <path d="M12 2v4M12 18v4M2 12h4M18 12h4"/>
          </svg>
        </div>
        <span class="section-lbl" style="color:var(--color-secondary);font-size:9px;letter-spacing:.08em;">
          IMPORT FROM RA
        </span>
      </div>
      <button
        v-if="importedArtist"
        class="btn-hud btn-hud-ghost btn-hud-xs"
        style="color:var(--color-tertiary);"
        @click="clearForm"
      >
        <X style="width:8px;height:8px;margin-right:4px;" aria-hidden="true" />
        CLEAR
      </button>
    </div>

    <!-- Slug input -->
    <div style="margin-bottom:14px;">
      <div class="input-label" style="margin-bottom:6px;">RA ARTIST SLUG</div>
      <div style="display:flex;gap:8px;">
        <Input
          v-model="artistSlug"
          :key="clearTrigger"
          placeholder="e.g. helenahauff"
          class="hud-input flex-1"
          style="height:34px;font-size:12px;"
        />
        <button
          class="btn-hud btn-hud-violet btn-hud-sm"
          :disabled="!artistSlug.trim() || importing"
          style="height:34px;min-height:34px;"
          @click="fetchArtist"
        >
          <Loader
            v-if="importing"
            class="animate-spin"
            style="width:11px;height:11px;"
            aria-hidden="true"
          />
          <Search v-else style="width:11px;height:11px;" aria-hidden="true" />
          <span style="font-size:10px;letter-spacing:.06em;">FETCH</span>
        </button>
      </div>
      <p style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);margin-top:6px;letter-spacing:.05em;text-transform:uppercase;">
        Enter artist slug from <span style="color:var(--color-secondary);">ra.co/dj/</span><span style="color:var(--color-secondary);text-transform:lowercase;">&lt;slug&gt;</span>
      </p>
    </div>

    <!-- Artist info card -->
    <div
      v-if="importedArtist"
      class="hud-card hud-card-v"
      style="padding:13px 15px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.10);margin-bottom:14px;"
    >
      <!-- Artist header -->
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:11px;">
        <div style="width:30px;height:30px;border-radius:50%;background:linear-gradient(135deg,var(--color-secondary-container),var(--color-primary-container));display:flex;align-items:center;justify-content:center;flex-shrink:0;box-shadow:0 0 24px rgba(200,184,255,.15);border:1px solid rgba(200,184,255,.1);">
          <span style="font-family:var(--font-command);font-size:14px;color:var(--color-on-primary);font-weight:700;line-height:1;">
            {{ importedArtist.name.charAt(0) }}
          </span>
        </div>
        <div style="flex:1;min-width:0;">
          <div style="font-family:var(--font-command);font-size:14px;font-weight:600;color:var(--color-on-surface);letter-spacing:-.02em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">
            {{ importedArtist.name }}
          </div>
          <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);margin-top:2px;">
            RESIDENT ADVISOR · {{ importedArtist.followers.toLocaleString() }} FOLLOWERS
          </div>
        </div>
        <a
          :href="importedArtist.url"
          target="_blank"
          rel="noopener noreferrer"
          class="social-chip"
          style="flex-shrink:0;padding:5px 9px;"
        >
          <ExternalLink style="width:10px;height:10px;" aria-hidden="true" />
          VIEW RA
        </a>
      </div>

      <!-- Bio -->
      <div
        v-if="importedArtist.biography"
        style="padding:11px 13px;background:rgba(150,248,255,.03);border:1px solid rgba(150,248,255,.07);margin-bottom:11px;border-radius:2px;"
      >
        <div class="epk-preview-label" style="margin-bottom:7px;">
          BIOGRAPHY
        </div>
        <p style="font-family:var(--font-data);font-size:12px;color:var(--color-on-surface-variant);line-height:1.6;margin:0;">
          {{ importedArtist.biography }}
        </p>
      </div>

      <!-- Social links -->
      <div style="display:flex;flex-wrap:wrap;gap:7px;">
        <span
          v-if="importedArtist.instagram"
          class="social-chip"
        >
          <span style="color:var(--color-tertiary);font-weight:600;">IG</span>
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-secondary);">{{ importedArtist.instagram }}</span>
        </span>
        <span
          v-if="importedArtist.soundcloud"
          class="social-chip"
        >
          <span style="color:var(--color-tertiary);font-weight:600;">SC</span>
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-secondary);">{{ importedArtist.soundcloud }}</span>
        </span>
        <span
          v-if="importedArtist.bandcamp"
          class="social-chip"
        >
          <span style="color:var(--color-tertiary);font-weight:600;">BC</span>
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-secondary);">{{ importedArtist.bandcamp }}</span>
        </span>
        <span
          v-if="importedArtist.website"
          class="social-chip"
        >
          <span style="color:var(--color-tertiary);font-weight:600;">WEB</span>
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-secondary);">{{ importedArtist.website }}</span>
        </span>
        <span
          v-if="importedArtist.twitter"
          class="social-chip"
        >
          <span style="color:var(--color-tertiary);font-weight:600;">X</span>
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-secondary);">{{ importedArtist.twitter }}</span>
        </span>
        <span
          v-if="importedArtist.facebook"
          class="social-chip"
        >
          <span style="color:var(--color-tertiary);font-weight:600;">FB</span>
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-secondary);">{{ importedArtist.facebook }}</span>
        </span>
        <span
          v-if="importedArtist.discogs"
          class="social-chip"
        >
          <span style="color:var(--color-tertiary);font-weight:600;">DS</span>
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-secondary);">{{ importedArtist.discogs }}</span>
        </span>
      </div>
    </div>

    <!-- Apply button -->
    <div v-if="importedArtist" style="margin-bottom:14px;">
      <button
        class="btn-hud btn-hud-violet luminous-threshold"
        style="width:100%;padding:0 20px;height:36px;display:inline-flex;align-items:center;gap:8px;"
        :disabled="applyState === 'applying'"
        @click="applyToEpk"
      >
        <Loader v-if="applyState === 'applying'" class="animate-spin" style="width:13px;height:13px;" aria-hidden="true" />
        <Check v-else style="width:13px;height:13px;" aria-hidden="true" />
        <span style="font-size:10px;letter-spacing:.08em;text-transform:uppercase;font-weight:600;">
          {{ applyState === 'applying' ? 'APPLYING...' : applyState === 'applied' ? 'APPLY AGAIN' : 'APPLY TO EPK' }}
        </span>
      </button>
      <p
        v-if="applyState === 'applied'"
        role="status"
        data-testid="ra-apply-success"
        style="font-family:var(--font-terminal);font-size:9px;color:var(--color-primary);margin-top:7px;text-align:center;letter-spacing:.04em;text-transform:uppercase;"
      >
        Saved to your EPK: {{ appliedSummary }}
      </p>
      <p
        v-else-if="applyState === 'error'"
        role="alert"
        data-testid="ra-apply-error"
        style="font-family:var(--font-data);font-size:11px;color:var(--color-error);margin-top:7px;text-align:center;line-height:1.4;"
      >
        {{ applyError }}
      </p>
      <p v-else style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);margin-top:7px;text-align:center;letter-spacing:.04em;text-transform:uppercase;">
        Replaces your DJ name and bio · adds RA's social links
      </p>
    </div>

    <!-- Loading -->
    <div
      v-if="importing && !importedArtist"
      style="padding:18px;display:flex;align-items:center;justify-content:center;gap:10px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.10);border-radius:2px;"
    >
      <Loader class="animate-spin" style="width:16px;height:16px;color:var(--color-secondary);filter:drop-shadow(0 0 8px rgba(200,184,255,.4));" aria-hidden="true" />
      <span class="section-lbl" style="color:var(--color-secondary);font-size:9px;letter-spacing:.08em;">
        FETCHING FROM RA...
      </span>
    </div>

    <!-- Error -->
    <div
      v-if="raStore.error && !importing"
      style="display:flex;align-items:center;gap:8px;padding:10px 12px;background:rgba(255,113,108,.06);border:1px solid rgba(255,113,108,.15);border-radius:2px;"
    >
      <AlertCircle style="width:14px;height:14px;color:var(--color-error);flex-shrink:0;filter:drop-shadow(0 0 6px rgba(255,113,108,.3));" aria-hidden="true" />
      <span style="font-family:var(--font-data);font-size:12px;color:var(--color-error);line-height:1.4;">{{ raStore.error }}</span>
    </div>

    <!-- Footer info -->
    <div style="margin-top:12px;padding-top:11px;border-top:1px solid rgba(200,184,255,.05);">
      <p style="font-family:var(--font-data);font-size:11px;color:var(--color-tertiary);line-height:1.55;margin:0;">
        Resident Advisor is the primary platform for electronic music.
        Importing from RA brings verified bio, social links, and profile URL
        directly into your press kit.
      </p>
    </div>
  </div>
</template>
