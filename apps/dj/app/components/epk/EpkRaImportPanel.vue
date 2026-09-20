<script setup lang="ts">
import { ref } from 'vue'
import { useRaStore } from '~/stores/ra'
import { useEpkStore } from '~/stores/epk'
import Input from '~/components/ui/input/Input.vue'
import { Search, ExternalLink, Check, AlertCircle, Loader } from 'lucide-vue-next'

const raStore = useRaStore()
const epkStore = useEpkStore()

const artistSlug = ref('')
const importing = ref(false)
const importedArtist = ref<typeof raStore.artist>(null)

async function fetchArtist() {
  if (!artistSlug.value.trim()) return
  importing.value = true
  await raStore.fetchArtist(artistSlug.value)
  importedArtist.value = raStore.artist
  importing.value = false
}

async function applyToEpk() {
  if (!importedArtist.value) return

  if (importedArtist.value.biography) {
    epkStore.bioLong = importedArtist.value.biography
    epkStore.bioShort = importedArtist.value.biography.substring(0, 200)
  }

  try {
    await $fetch('/api/v1/settings', {
      method: 'PUT',
      body: {
        social_links: {
          instagram: importedArtist.value.instagram || '',
          soundcloud: importedArtist.value.soundcloud || '',
          spotify: '',
          mixcloud: '',
          residentAdvisor: importedArtist.value.url || '',
          bandcamp: importedArtist.value.bandcamp || '',
          youtube: '',
          twitter: importedArtist.value.twitter || '',
          facebook: importedArtist.value.facebook || '',
          discogs: importedArtist.value.discogs || '',
          website: importedArtist.value.website || '',
        },
      },
    })
  } catch (e) {
    console.error('Failed to save social links:', e)
  }
}

function clearForm() {
  artistSlug.value = ''
  raStore.clear()
  importedArtist.value = null
}
</script>

<template>
  <div class="glass-panel" style="padding:14px 16px;">
    <!-- Header row -->
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px;">
      <div style="display:flex;align-items:center;gap:6px;">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:var(--color-secondary);">
          <circle cx="12" cy="12" r="10"/>
          <circle cx="12" cy="12" r="3"/>
          <path d="M12 2v4M12 18v4M2 12h4M18 12h4"/>
        </svg>
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-secondary);">
          IMPORT FROM RA
        </span>
      </div>
      <button
        v-if="importedArtist"
        class="btn-hud btn-hud-ghost"
        style="padding:0 8px;height:18px;font-size:8px;"
        @click="clearForm"
      >
        CLEAR
      </button>
    </div>

    <!-- Slug input row -->
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
          @click="fetchArtist"
        >
          <Loader
            v-if="importing"
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

    <!-- Artist info card -->
    <div
      v-if="importedArtist"
      style="padding:12px 14px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.06);margin-bottom:10px;"
    >
      <!-- Artist header -->
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:10px;">
        <div style="width:24px;height:24px;border-radius:50%;background:linear-gradient(135deg,var(--color-secondary-container),var(--color-primary-container));display:flex;align-items:center;justify-content:center;flex-shrink:0;">
          <span style="font-family:var(--font-terminal);font-size:11px;color:var(--color-on-primary);font-weight:700;">
            {{ importedArtist.name.charAt(0) }}
          </span>
        </div>
        <div style="flex:1;min-width:0;">
          <div style="font-family:var(--font-command);font-size:13px;font-weight:600;color:var(--color-on-surface);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">
            {{ importedArtist.name }}
          </div>
          <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);margin-top:1px;">
            RESIDENT ADVISOR · {{ importedArtist.followers.toLocaleString() }} FOLLOWERS
          </div>
        </div>
        <a
          :href="importedArtist.url"
          target="_blank"
          rel="noopener noreferrer"
          style="display:inline-flex;align-items:center;gap:4px;padding:5px 8px;background:rgba(200,184,255,.06);border:1px solid rgba(200,184,255,.12);font-family:var(--font-terminal);font-size:7px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-secondary);text-decoration:none;flex-shrink:0;"
        >
          <ExternalLink style="width:9px;height:9px;" aria-hidden="true" />
          VIEW RA
        </a>
      </div>

      <!-- Bio -->
      <div
        v-if="importedArtist.biography"
        style="padding:10px 12px;background:rgba(150,248,255,.03);border:1px solid rgba(150,248,255,.06);margin-bottom:8px;"
      >
        <div style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);margin-bottom:5px;">
          BIOGRAPHY
        </div>
        <p style="font-family:var(--font-data);font-size:11px;line-height:1.55;color:var(--color-on-surface-variant);margin:0;">
          {{ importedArtist.biography }}
        </p>
      </div>

      <!-- Social links grid -->
      <div style="display:grid;grid-template-columns:repeat(2,1fr);gap:5px;">
        <div
          v-if="importedArtist.instagram"
          style="display:flex;align-items:center;gap:5px;padding:5px 7px;background:rgba(200,184,255,.04);border:1px solid rgba(200,184,255,.06);font-size:10px;"
        >
          <span style="color:var(--color-tertiary);font-weight:600;letter-spacing:.04em;">IG</span>
          <span style="color:var(--color-secondary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ importedArtist.instagram }}</span>
        </div>
        <div
          v-if="importedArtist.soundcloud"
          style="display:flex;align-items:center;gap:5px;padding:5px 7px;background:rgba(200,184,255,.04);border:1px solid rgba(200,184,255,.06);font-size:10px;"
        >
          <span style="color:var(--color-tertiary);font-weight:600;letter-spacing:.04em;">SC</span>
          <span style="color:var(--color-secondary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ importedArtist.soundcloud }}</span>
        </div>
        <div
          v-if="importedArtist.bandcamp"
          style="display:flex;align-items:center;gap:5px;padding:5px 7px;background:rgba(200,184,255,.04);border:1px solid rgba(200,184,255,.06);font-size:10px;"
        >
          <span style="color:var(--color-tertiary);font-weight:600;letter-spacing:.04em;">BC</span>
          <span style="color:var(--color-secondary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ importedArtist.bandcamp }}</span>
        </div>
        <div
          v-if="importedArtist.website"
          style="display:flex;align-items:center;gap:5px;padding:5px 7px;background:rgba(200,184,255,.04);border:1px solid rgba(200,184,255,.06);font-size:10px;"
        >
          <span style="color:var(--color-tertiary);font-weight:600;letter-spacing:.04em;">WEB</span>
          <span style="color:var(--color-secondary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ importedArtist.website }}</span>
        </div>
      </div>
    </div>

    <!-- Apply button -->
    <div v-if="importedArtist" style="margin-bottom:10px;">
      <button
        class="btn-hud btn-hud-violet luminous-threshold"
        style="width:100%;height:30px;display:inline-flex;align-items:center;justify-content:center;gap:6px;"
        @click="applyToEpk"
      >
        <Check style="width:12px;height:12px;" aria-hidden="true" />
        <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;">
          APPLY TO EPK
        </span>
      </button>
      <div style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);margin-top:5px;text-align:center;letter-spacing:.04em;">
        Bio and social links will be added to your press kit
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="importing && !importedArtist"
      style="padding:16px;display:flex;align-items:center;justify-content:center;gap:8px;background:var(--color-surface-container-lowest);border:1px solid rgba(200,184,255,.06);"
    >
      <Loader class="animate-spin" style="width:14px;height:14px;color:var(--color-secondary);" aria-hidden="true" />
      <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-secondary);">
        FETCHING FROM RA...
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

    <!-- Footer info -->
    <div style="margin-top:8px;padding-top:8px;border-top:1px solid rgba(200,184,255,.04);">
      <p style="margin:0;font-family:var(--font-data);font-size:10px;color:var(--color-tertiary);line-height:1.5;">
        Resident Advisor is the primary platform for electronic music.
        Importing from RA brings verified bio, social links, and profile URL
        directly into your press kit.
      </p>
    </div>
  </div>
</template>
