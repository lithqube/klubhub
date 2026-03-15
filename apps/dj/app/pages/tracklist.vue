<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useDropZone } from '@vueuse/core'
import TrackcardPreview from '@/app/components/TrackcardPreview.vue'
import { 
  uploadTracklist, 
  listTracklists, 
  getTracklist, 
  updateTrack, 
  uploadTrackArtwork, 
  deleteTracklist, 
  pollArtworkStatus 
} from '@/app/composables/useTracklist'

// Step management: 'upload' or 'edit'
const step = ref<'upload' | 'edit'>('upload')

// Upload state
const uploadFile = ref<File | null>(null)
const uploadFilename = ref<string>('')
const uploadFileSize = ref<string>('')
const uploadError = ref<string | null>(null)
const uploadLoading = ref<boolean>(false)

// Edit state
const tracklist = ref<any>(null)
const tracks = ref<any[]>([])
const warnings = ref<any[]>([])
const editLoading = ref<boolean>(false)
const editError = ref<string | null>(null)
const abortController = ref<AbortController | null>(null)

// Customization state (synced with settings)
const preset = ref<'default' | 'dark' | 'light' | 'neon' | 'minimal'>('default')
const bgMode = ref<'solid' | 'upload' | 'mosaic'>('solid')
const bgValue = ref<string | null>(null)
const visibleFields = ref<string[]>(['title', 'artist', 'bpm', 'key', 'durationSecs'])
const maxTracks = ref<number>(50)
const trackRangeStart = ref<number | null>(null)
const trackRangeEnd = ref<number | null>(null)
const logoPath = ref<string | null>(null)
const logoPosition = ref<'top-left' | 'top-right' | 'bottom-left' | 'bottom-right'>('top-left')
const customPlaceholderPath = ref<string | null>(null)

// Export state
const exportLoading = ref<boolean>(false)
const exportError = ref<string | null>(null)

// Past tracklists
const pastTracklists = ref<any[]>([])
const pastTracklistsLoading = ref<boolean>(false)
const pastTracklistsError = ref<string | null>(null)

// Initialize page - load settings and past tracklists
const initializePage = async () => {
  try {
    // Load settings
    const settings = await $fetch('/api/v1/settings')
    preset.value = settings.tracklist_preferences?.preset ?? 'default'
    bgMode.value = settings.tracklist_preferences?.bgMode ?? 'solid'
    bgValue.value = settings.tracklist_preferences?.bgValue ?? null
    visibleFields.value = settings.tracklist_preferences?.visibleFields ?? ['title', 'artist', 'bpm', 'key', 'durationSecs']
    maxTracks.value = settings.tracklist_preferences?.maxTracks ?? 50
    trackRangeStart.value = settings.tracklist_preferences?.trackRangeStart ?? null
    trackRangeEnd.value = settings.tracklist_preferences?.trackRangeEnd ?? null
    logoPath.value = settings.logoPath ?? null
    logoPosition.value = settings.tracklist_preferences?.logoPosition ?? 'top-left'
    customPlaceholderPath.value = settings.custom_placeholder_path ?? null
    
    // Load past tracklists
    await loadPastTracklists()
  } catch (err: any) {
    editError.value = err.message ?? 'Failed to load settings'
  }
}

const loadPastTracklists = async () => {
  pastTracklistsLoading.value = true
  pastTracklistsError.value = null
  try {
    const data = await $fetch('/api/v1/tracklists')
    pastTracklists.value = Array.isArray(data) ? data : []
  } catch (err: any) {
    pastTracklistsError.value = err.message ?? 'Failed to load past tracklists'
  } finally {
    pastTracklistsLoading.value = false
  }
}

// File upload handling
const onDrop = async (e: DragEvent) => {
  e.preventDefault()
  const file = e.dataTransfer?.files?.[0]
  if (file) await handleFile(file)
}

const onFileSelect = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) await handleFile(file)
}

const handleFile = async (file: File) => {
  // Validate file type
  if (!file.name.toLowerCase().endsWith('.txt')) {
    uploadError.value = 'Only .txt files are allowed'
    return
  }
  
  uploadFile.value = file
  uploadFilename.value = file.name
  uploadFileSize.value = `${(file.size / 1024).toFixed(1)} KB`
  uploadError.value = null
}

// Parse file button handler
const parseFile = async () => {
  if (!uploadFile.value) return
  
  uploadLoading.value = true
  uploadError.value = null
  
  try {
    const result = await uploadTracklist(uploadFile.value)
    tracklist.value = result.tracklist
    tracks.value = result.tracks
    warnings.value = result.warnings
    
    // Transition to edit step
    step.value = 'edit'
    
    // Start artwork polling
    startArtworkPolling()
  } catch (err: any) {
    uploadError.value = err.message ?? 'Failed to parse file'
  } finally {
    uploadLoading.value = false
  }
}

// Artwork polling
const startArtworkPolling = () => {
  // Abort any existing polling
  if (abortController.value) {
    abortController.value.abort()
  }
  
  // Create new abort controller
  abortController.value = new AbortController()
  
  // Start polling
  pollArtworkStatus(
    tracklist.value.id,
    2000, // 2 second interval
    (updatedTracks: any[]) => {
      // Only update non-manual tracks to preserve manual overrides
      tracks.value = tracks.value.map(originalTrack => {
        const updatedTrack = updatedTracks.find(t => t.id === originalTrack.id)
        // If track has manual artwork, keep it as-is
        if (originalTrack.artworkStatus === 'manual') {
          return originalTrack
        }
        // Otherwise use updated track (or original if not found)
        return updatedTrack ?? originalTrack
      })
    },
    abortController.value.signal
  )
}

// Stop artwork polling
const stopArtworkPolling = () => {
  if (abortController.value) {
    abortController.value.abort()
    abortController.value = null
  }
}

// Track editing
const saveTrackEdit = async (trackId: string, field: keyof any, value: any) => {
  editLoading.value = true
  editError.value = null
  
  try {
    const result = await updateTrack(tracklist.value.id, trackId, { [field]: value })
    // Update local track
    const index = tracks.value.findIndex(t => t.id === trackId)
    if (index !== -1) {
      tracks.value[index] = { ...tracks.value[index], ...result }
    }
  } catch (err: any) {
    editError.value = err.message ?? 'Failed to save track'
  } finally {
    editLoading.value = false
  }
}

// Track artwork upload
const uploadTrackArt = async (trackId: string, file: File) => {
  editLoading.value = true
  editError.value = null
  
  try {
    const result = await uploadTrackArtwork(tracklist.value.id, trackId, file)
    // Update local track
    const index = tracks.value.findIndex(t => t.id === trackId)
    if (index !== -1) {
      tracks.value[index] = { 
        ...tracks.value[index], 
        artworkUrl: result.artworkUrl,
        artworkStatus: result.artworkStatus,
        artworkSource: result.artworkSource
      }
    }
  } catch (err: any) {
    editError.value = err.message ?? 'Failed to upload artwork'
  } finally {
    editLoading.value = false
  }
}

// Customization handlers
const updatePreset = async (newPreset: typeof preset.value) => {
  preset.value = newPreset
  await saveSettings()
}

const updateBgMode = async (newBgMode: typeof bgMode.value) => {
  bgMode.value = newBgMode
  // Reset bgValue when switching modes
  if (newBgMode === 'solid') {
    bgValue.value = null
  } else if (newBgMode === 'upload') {
    bgValue.value = null
  }
  await saveSettings()
}

const updateBgValue = async (value: string | null) => {
  bgValue.value = value
  await saveSettings()
}

const updateVisibleFields = async (fields: string[]) => {
  visibleFields.value = fields
  await saveSettings()
}

const updateMaxTracks = async (value: number) => {
  maxTracks.value = value
  await saveSettings()
}

const updateTrackRange = async (start: number | null, end: number | null) => {
  trackRangeStart.value = start
  trackRangeEnd.value = end
  await saveSettings()
}

const updateLogoPath = async (path: string | null) => {
  logoPath.value = path
  await saveSettings()
}

const updateLogoPosition = async (position: typeof logoPosition.value) => {
  logoPosition.value = position
  await saveSettings()
}

const updateCustomPlaceholderPath = async (path: string | null) => {
  customPlaceholderPath.value = path
  await saveSettings()
}

const saveSettings = async () => {
  try {
    await $fetch('/api/v1/settings', {
      method: 'PUT',
      body: {
        tracklist_preferences: {
          preset: preset.value,
          bg_mode: bgMode.value,
          visible_fields: visibleFields.value,
          max_tracks: maxTracks.value,
          track_range_start: trackRangeStart.value,
          track_range_end: trackRangeEnd.value,
          logo_position: logoPosition.value
        },
        custom_placeholder_path: customPlaceholderPath.value,
        logo_path: logoPath.value,
        bg_value: bgMode.value === 'solid' ? bgValue.value : null
      }
    })
    
    // Reload past tracklists in case settings affected them
    await loadPastTracklists()
  } catch (err: any) {
    editError.value = err.message ?? 'Failed to save settings'
  }
}

// Export handlers
const exportImage = async (format: 'story' | 'square') => {
  if (!tracklist.value) return
  
  exportLoading.value = true
  exportError.value = null
  
  try {
    // Note: In a real implementation, this would trigger a file download
    // For now, we'll just call the API and handle the response
    const result = await $fetch(`/api/v1/tracklists/${tracklist.value.id}/generate-image?format=${format}`, {
      method: 'POST'
    })
    
    // In a real app, we'd trigger a download here
    console.log('Export result:', result)
  } catch (err: any) {
    exportError.value = err.message ?? 'Failed to export image'
  } finally {
    exportLoading.value = false
  }
}

// Delete tracklist
const deleteTracklistHandler = async (id: string) => {
  if (!confirm(`Delete "${tracklist.value?.title}"? This will permanently remove ${tracks.value.length} tracks.`)) {
    return
  }
  
  try {
    await deleteTracklist(id)
    // Reset to upload step
    step.value = 'upload'
    tracklist.value = null
    tracks.value = []
    warnings.value = []
    
    // Reload past tracklists
    await loadPastTracklists()
  } catch (err: any) {
    editError.value = err.message ?? 'Failed to delete tracklist'
  }
}

// Load a past tracklist
const loadPastTracklist = async (id: string) => {
  try {
    step.value = 'edit'
    const result = await getTracklist(id)
    tracklist.value = result.tracklist
    tracks.value = result.tracks
    warnings.value = []
    
    // Start artwork polling
    startArtworkPolling()
  } catch (err: any) {
    editError.value = err.message ?? 'Failed to load tracklist'
  }
}

// Cleanup on unmount
onBeforeUnmount(() => {
  stopArtworkPolling()
})

// Initialize page on mount
onMounted(() => {
  initializePage())
}
</script>

<template>
  <div class="min-h-screen bg-gray-950 text-white p-6">
    <!-- STEP 1: Upload Zone -->
    <div v-if="step === 'upload'" class="max-w-2xl mx-auto text-center">
      <h1 class="text-4xl font-bold mb-6">KlubHub DJ Tracklist Generator</h1>
      <p class="mb-8 text-lg opacity-75">
        Drag and drop your Rekordbox TXT export here, or click to select a file
      </p>
      
      <!-- Drop Zone -->
      <div 
        v-dropzone="{ 
          onDrop: onDrop,
          activeClass: 'border-dashed border-primary',
          invalidClass: 'border-dashed border-error'
        }"
        class="border-2 border-dashed border-gray-600 rounded-lg p-12 hover:border-gray-400 transition-colors cursor-pointer"
        :class="{ 'border-primary': !uploadError, 'border-error': !!uploadError }"
      >
        <div v-if="!uploadFile">
          <svg class="mx-auto mb-6 h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M7 16v4a2 2 0 002 2h8a2 2 0 002-2v-4M11 8h6m0 0l4-4m-4 4l4 4m-5-4V4a2 2 0 012-2h1.5a2 2 0 011.414.586l2 2A2 2 0 0115 8.586v7a2 2 0 01-2 2h-2M9 16h6"/>
          </svg>
          <p class="mt-4 text-lg">Drop TXT file here</p>
          <p class="text-sm text-gray-500">or</p>
          <button @click="document.getElementById('file-input').click()" class="mt-2 px-4 py-2 bg-gray-800 hover:bg-gray-700 rounded">
            Select File
          </button>
          <input 
            id="file-input" 
            type="file" 
            accept=".txt" 
            style="display: none" 
            @change="onFileSelect"
          />
        </div>
        
        <div v-else class="mt-6 text-center">
          <p class="font-medium">{{ uploadFilename }}</p>
          <p class="text-sm text-gray-400">{{ uploadFileSize }}</p>
          <button 
            :disabled="uploadLoading" 
            class="mt-4 px-6 py-3 bg-primary hover:bg-primary-dark rounded-lg disabled:opacity-50"
          >
            {{ uploadLoading ? 'Parsing...' : 'Parse File' }}
          </button>
        </div>
        
        <div v-if="uploadError" class="mt-4 p-3 bg-red-900 rounded text-red-300">
          {{ uploadError }}
        </div>
      </div>
    </div>
    
    <!-- STEP 2: Edit + Preview -->
    <div v-else class="flex gap-6 p-6 min-h-screen bg-gray-950 text-white">
      <!-- LEFT COLUMN: Editable Track Table -->
      <div class="flex-1 bg-gray-800 rounded-lg p-4 overflow-hidden">
        <div class="mb-4">
          <h2 class="text-xl font-bold flex items-center gap-2">
            <span class="text-primary">📝</span> Edit Tracklist
          </h2>
          <p class="text-sm text-gray-400 mt-1">
            {{ tracklist?.value?.title || 'No tracklist loaded' }}
          </p>
        </div>
        
        <!-- Upload Error Banner -->
        <div v-if="editError" class="mb-4 p-3 bg-red-900 rounded text-red-300">
          {{ editError }}
        </div>
        
        <!-- Track Table -->
        <div class="overflow-auto">
          <table class="min-w-full border-collapse">
            <thead>
              <tr class="border-b border-gray-700">
                <th class="text-left py-2 pl-4 text-sm font-medium text-gray-400">#</th>
                <th class="text-left py-2 pl-4 text-sm font-medium text-gray-400">Cover Art</th>
                <th class="text-left py-2 pl-4 text-sm font-medium text-gray-400">Title</th>
                <th class="text-left py-2 pl-4 text-sm font-medium text-gray-400">Artist</th>
                <th v-if="visibleFields.value.includes('bpm')" class="text-left py-2 pl-4 text-sm font-medium text-gray-400">BPM</th>
                <th v-if="visibleFields.value.includes('musical_key')" class="text-left py-2 pl-4 text-sm font-medium text-gray-400">Key</th>
                <th v-if="visibleFields.value.includes('durationSecs')" class="text-left py-2 pl-4 text-sm font-medium text-gray-400">Duration</th>
                <th class="text-left py-2 pl-4 text-sm font-medium text-gray-400">Art</th>
                <th v-if="warnings.value.length > 0" class="text-left py-2 pl-4 text-sm font-medium text-gray-400">Warning</th>
              </tr>
            </thead>
            <tbody>
              <tr 
                v-for="(track, index) in tracks" 
                :key="track.id"
                class="border-t border-gray-700 hover:bg-gray-700/50"
              >
                <!-- Position -->
                <td class="py-4 pl-4 text-center font-mono text-sm">
                  {{ index + 1 }}
                </td>
                
                <!-- Cover Art -->
                <td class="py-4 pl-4">
                  <div class="relative w-10 h-10">
                    <div 
                      :style="[
                        { width: '100%', height: '100%', borderRadius: '4px' },
                        track.artworkUrl 
                          ? { 
                              backgroundImage: `url(${track.artworkUrl})`, 
                              backgroundSize: 'cover',
                              backgroundPosition: 'center'
                            }
                          : { 
                              backgroundColor: '#333333',
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center'
                            }
                      ]"
                    >
                      <template v-if="track.artworkStatus === 'manual'">
                        <div class="absolute inset-0 flex items-center justify-center pointer-events-none">
                          <span class="bg-black/50 text-xs text-white px-1 rounded">LOCK</span>
                        </div>
                      </template>
                    </div>
                  </div>
                </td>
                
                <!-- Title -->
                <td class="py-4 pl-4">
                  <div class="flex items-center gap-2">
                    <span class="cursor-pointer hover:underline" 
                          @dblclick="editCell(track.id, 'title', track.title || '')">
                      {{ track.title || 'Unknown Title' }}
                    </span>
                  </div>
                </td>
                
                <!-- Artist -->
                <td class="py-4 pl-4">
                  <span class="cursor-pointer hover:underline" 
                        @dblclick="editCell(track.id, 'artist', track.artist || '')">
                    {{ track.artist || 'Unknown Artist' }}
                  </span>
                </td>
                
                <!-- BPM -->
                <td v-if="visibleFields.value.includes('bpm')" class="py-4 pl-4">
                  <span v-if="track.bpm !== null && track.bpm !== undefined" 
                        class="cursor-pointer hover:underline" 
                        @dblclick="editCell(track.id, 'bpm', track.bpm)">
                    {{ track.bpm }}
                  </span>
                  <span v-else class="text-gray-500">—</span>
                </td>
                
                <!-- Key -->
                <td v-if="visibleFields.value.includes('musical_key')" class="py-4 pl-4">
                  <span v-if="track.musicalKey" 
                        class="cursor-pointer hover:underline" 
                        @dblclick="editCell(track.id, 'musicalKey', track.musicalKey)">
                    {{ track.musicalKey }}
                  </span>
                  <span v-else class="text-gray-500">—</span>
                </td>
                
                <!-- Duration -->
                <td v-if="visibleFields.value.includes('durationSecs')" class="py-4 pl-4">
                  <span v-if="track.durationSecs !== null && track.durationSecs !== undefined" 
                        class="cursor-pointer hover:underline" 
                        @dblclick="editCell(track.id, 'durationSecs', track.durationSecs)">
                    {{ Math.floor(track.durationSecs / 60) }}:{{ String(track.durationSecs % 60).padStart(2, '0') }}
                  </span>
                  <span v-else class="text-gray-500">—</span>
                </td>
                
                <!-- Artwork Upload -->
                <td class="py-4 pl-4">
                  <label 
                    class="flex items-center justify-center cursor-pointer hover:bg-gray-700/50 rounded p-2"
                    :title="track.artworkStatus === 'manual' ? 'Artwork is locked (manual)' : 'Replace artwork'"
                  >
                    <input 
                      type="file" 
                      accept="image/jpeg,image/png,image/webp" 
                      class="hidden"
                      @change="($event) => {
                        const file = $event.target.files?.[0];
                        if (file) uploadTrackArt(track.id, file);
                      }"
                    >
                    <svg 
                      v-if="track.artworkStatus !== 'manual'" 
                      class="h-4 w-4 text-gray-400" 
                      fill="none" 
                      stroke="currentColor" 
                      viewBox="0 0 24 24"
                    >
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 14l2-2-5 5 2 2 2 2-2-2 5 5-2-2z"/>
                    </svg>
                    <template v-else>
                      <span class="text-xs text-gray-400">Locked</span>
                    </template>
                  </label>
                </td>
                
                <!-- Warnings -->
                <td v-if="warnings.value.length > 0" class="py-4 pl-4 text-sm text-yellow-400">
                  {{ warnings.value.find(w => w.row === index + 1)?.message || '' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        
        <!-- Warnings Table -->
        <div v-if="warnings.value.length > 0" class="mt-4">
          <div class="mb-2 flex items-center gap-2">
            <svg class="h-4 w-4 text-yellow-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
            </svg>
            <span class="font-medium">Parse Warnings</span>
          </div>
          <div class="max-h-24 overflow-y-auto border border-gray-700 rounded bg-gray-800">
            <table class="min-w-full border-collapse text-xs">
              <thead>
                <tr class="border-b border-gray-700">
                  <th class="text-left py-1 pl-2">Row</th>
                  <th class="text-left py-1 pl-2">Message</th>
                </tr>
              </thead>
              <tbody>
                <tr 
                  v-for="(warning, index) in warnings" 
                  :key="index"
                  class="border-t border-gray-700"
                >
                  <td class="py-1 pl-2">{{ warning.row }}</td>
                  <td class="py-1 pl-2">{{ warning.message }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        
        <!-- Retry Cover Art Banner -->
        <div v-if="tracks.value.some(t => t.artworkStatus === 'pending')" 
             class="mt-4 p-3 bg-yellow-900 rounded text-yellow-300"
        >
          <div class="flex items-center gap-2">
            <svg class="h-5 w-5 text-yellow-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.14 16c-.77 1.333.192 3 1.732 3z"/>
            </svg>
            <span>
              Some track artwork is still loading. 
              <button @click="startArtworkPolling" class="ml-2 underline hover:text-yellow-200">Retry</button>
            </span>
          </div>
        </div>
      </div>
      
      <!-- RIGHT COLUMN: Customization + Preview -->
      <div class="flex-1 flex flex-col gap-6">
        <!-- Customization Panel -->
        <div class="bg-gray-800 rounded-lg p-4">
          <h2 class="text-xl font-bold mb-4 flex items-center gap-2">
            <span class="text-primary">🎨</span> Customization
          </div>
          
          <!-- Preset Selector -->
          <div class="mb-4">
            <label class="block text-sm font-medium mb-2">Preset</label>
            <div class="flex gap-2">
              <label 
                v-for="p in ['default', 'dark', 'light', 'neon', 'minimal']" 
                :key="p"
                class="flex items-center gap-2 cursor-pointer select-none p-2 rounded hover:bg-gray-700/50"
                :class="{ 'bg-primary/20': preset.value === p }"
              >
                <input 
                  type="radio" 
                  :name="'preset'" 
                  :value="p" 
                  :checked="preset.value === p"
                  class="hidden"
                  @change="updatePreset(p)"
                >
                <span class="text-sm text-capitalize">{{ p }}</span>
              </label>
            </div>
          </div>
          
          <!-- Background Mode -->
          <div class="mb-4">
            <label class="block text-sm font-medium mb-2">Background Mode</label>
            <div class="space-y-2">
              <!-- Solid -->
              <label class="flex items-center gap-3 cursor-pointer p-3 rounded hover:bg-gray-700/50"
                     :class="{ 'bg-primary/20': bgMode.value === 'solid' }">
                <input 
                  type="radio" 
                  name="bg-mode" 
                  value="solid" 
                  :checked="bgMode.value === 'solid'"
                  class="hidden"
                  @change="updateBgMode('solid')"
                >
                <div>
                  <div class="flex items-center gap-2">
                    <span class="font-medium">Solid Color</span>
                    <input 
                      type="color" 
                      :value="bgValue.value || '#1a1a2e'" 
                      @change="updateBgValue($event.target.value)"
                      class="h-8 w-12 p-0"
                    >
                  </div>
                </div>
              </label>
              
              <!-- Upload -->
              <label class="flex items-center gap-3 cursor-pointer p-3 rounded hover:bg-gray-700/50"
                     :class="{ 'bg-primary/20': bgMode.value === 'upload' }">
                <input 
                  type="radio" 
                  name="bg-mode" 
                  value="upload" 
                  :checked="bgMode.value === 'upload'"
                  class="hidden"
                  @change="updateBgMode('upload')"
                >
                <div>
                  <div class="flex items-center gap-2">
                    <span class="font-medium">Upload Image</span>
                    <input 
                      type="file" 
                      accept="image/*" 
                      class="hidden"
                      @change="($event) => {
                        const file = $event.target.files?.[0];
                        if (file) {
                          // In a real app, we'd upload this to the server
                          // For demo, we'll create an object URL
                          bgValue.value = URL.createObjectURL(file);
                          updateBgValue(bgValue.value);
                        }
                      }"
                    >
                  </div>
                </div>
              </label>
              
              <!-- Mosaic -->
              <label class="flex items-center gap-3 cursor-pointer p-3 rounded hover:bg-gray-700/50"
                     :class="{ 'bg-primary/20': bgMode.value === 'mosaic' }">
                <input 
                  type="radio" 
                  name="bg-mode" 
                  value="mosaic" 
                  :checked="bgMode.value === 'mosaic'"
                  class="hidden"
                  @change="updateBgMode('mosaic')"
                >
                <div>
                  <span class="font-medium">Mosaic (from artwork)</span>
                </div>
              </label>
            </div>
          </div>
          
          <!-- Field Visibility -->
          <div class="mb-4">
            <label class="block text-sm font-medium mb-2">Visible Fields</label>
            <div class="space-y-1">
              <label 
                v-for="field in ['title', 'artist', 'bpm', 'musical_key', 'durationSecs']" 
                :key="field"
                class="flex items-center gap-2"
              >
                <input 
                  type="checkbox" 
                  :id="`field-${field}`" 
                  :checked="visibleFields.value.includes(field)"
                  @change="($event) => {
                    const fields = [...visibleFields.value];
                    if ($event.target.checked) {
                      fields.push(field);
                    } else {
                      const index = fields.indexOf(field);
                      if (index > -1) fields.splice(index, 1);
                    }
                    updateVisibleFields(fields);
                  }"
                  class="h-4 w-4 text-primary"
                >
                <span class="text-sm">{{ field.replace('_', ' ').toUpperCase() }}</span>
              </label>
            </div>
          </div>
          
          <!-- Track Range -->
          <div v-if="tracks.value.length > maxTracks.value" class="mb-4">
            <label class="block text-sm font-medium mb-2">Track Range</label>
            <div class="flex gap-3">
              <input 
                type="number" 
                min="0" 
                :max="tracks.value.length - 1"
                :value="trackRangeStart.value ?? 0"
                @change="($event) => updateTrackRange(parseInt($event.target.value) || null, trackRangeEnd.value)"
                class="w-1/2 px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
                placeholder="From"
              >
              <input 
                type="number" 
                min="0" 
                :max="tracks.value.length - 1"
                :value="trackRangeEnd.value ?? tracks.value.length - 1"
                @change="($event) => updateTrackRange(trackRangeStart.value, parseInt($event.target.value) || null)"
                class="w-1/2 px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
                placeholder="To"
              >
            </div>
            <p class="mt-1 text-xs text-gray-500">
              Showing {{ ((trackRangeStart.value || 0) + 1) }} - {{ ((trackRangeEnd.value || tracks.value.length - 1) + 1) }} of {{ tracks.value.length }} tracks
            </p>
          </div>
          
          <!-- Logo Upload -->
          <div class="mb-4">
            <label class="block text-sm font-medium mb-2">Logo</label>
            <div class="flex items-center gap-3">
              <input 
                type="file" 
                accept="image/*" 
                class="hidden"
                @change="($event) => {
                  const file = $event.target.files?.[0];
                  if (file) {
                    // In a real app, we'd upload this to the server
                    // For demo, we'll create an object URL
                    logoPath.value = URL.createObjectURL(file);
                    updateLogoPath(logoPath.value);
                  }
                }"
              >
              <div class="flex-1 space-y-2">
                <div v-if="logoPath.value" class="flex items-center gap-2">
                  <img :src="logoPath.value" alt="Logo" class="h-10 w-auto" />
                  <button 
                    @click="updateLogoPath(null)" 
                    class="ml-auto text-xs text-red-400 hover:text-red-300"
                  >
                    Remove
                  </button>
                </div>
                <div v-else class="text-xs text-gray-500">
                  No logo selected
                </div>
              </div>
            </div>
            
            <div class="mt-2 space-y-1">
              <label class="block text-sm font-medium mb-1">Position</label>
              <div class="flex gap-2">
                <label 
                  v-for="pos in ['top-left', 'top-right', 'bottom-left', 'bottom-right']" 
                  :key="pos"
                  class="flex items-center gap-1 cursor-pointer p-1 rounded hover:bg-gray-700/50"
                  :class="{ 'bg-primary/20': logoPosition.value === pos }"
                >
                  <input 
                    type="radio" 
                    :name="'logo-position'" 
                    :value="pos" 
                    :checked="logoPosition.value === pos"
                    class="hidden"
                    @change="updateLogoPosition(pos)"
                  >
                  <span class="text-xs text-capitalize">{{ pos.replace('-', ' ') }}</span>
                </label>
              </div>
            </div>
          </div>
          
          <!-- Custom Placeholder -->
          <div class="mb-4">
            <label class="block text-sm font-medium mb-2">Custom Placeholder Image</label>
            <div class="flex items-center gap-3">
              <input 
                type="file" 
                accept="image/*" 
                class="hidden"
                @change="($event) => {
                  const file = $event.target.files?.[0];
                  if (file) {
                    // In a real app, we'd upload this to the server
                    // For demo, we'll create an object URL
                    customPlaceholderPath.value = URL.createObjectURL(file);
                    updateCustomPlaceholderPath(customPlaceholderPath.value);
                  }
                }"
              >
              <div class="flex-1 space-y-2">
                <div v-if="customPlaceholderPath.value" class="flex items-center gap-2">
                  <img :src="customPlaceholderPath.value" alt="Placeholder" class="h-10 w-auto" />
                  <button 
                    @click="updateCustomPlaceholderPath(null)" 
                    class="ml-auto text-xs text-red-400 hover:text-red-300"
                  >
                    Remove
                  </button>
                </div>
                <div v-else class="text-xs text-gray-500">
                  No placeholder selected
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <!-- Preview Area -->
        <div class="flex-1 bg-gray-800 rounded-lg p-4 flex flex-col">
          <div class="mb-3 text-sm text-gray-400 font-medium">
            Preview (50% resolution)
          </div>
          <div class="flex-1 overflow-hidden relative">
            <div 
              style="width: 540px; height: 960px; overflow: hidden; 
                     transform: scale(0.5); transform-origin: top left;"
            >
              <TrackcardPreview 
                :tracklist="tracklist"
                :tracks="tracks"
                :djName="'KlubHub DJ'"
                :logoPath="logoPath"
                :logoPosition="logoPosition"
                :preset="preset"
                :bgMode="bgMode"
                :bgValue="bgValue"
                :visibleFields="visibleFields"
                :maxTracks="maxTracks"
                :trackRangeStart="trackRangeStart"
                :trackRangeEnd="trackRangeEnd"
                :customPlaceholderPath="customPlaceholderPath"
              />
            </div>
          </div>
          
          <!-- Export Buttons -->
          <div class="mt-4 flex gap-3">
            <button 
              :disabled="exportLoading || !tracklist" 
              class="flex-1 px-4 py-3 bg-gray-700 hover:bg-gray-600 disabled:opacity-50 rounded"
              @click="exportImage('story')"
            >
              Export Story (1080×1920)
            </button>
            <button 
              :disabled="exportLoading || !tracklist" 
              class="flex-1 px-4 py-3 bg-gray-700 hover:bg-gray-600 disabled:opacity-50 rounded"
              @click="exportImage('square')"
            >
              Export Square (1080×1080)
            </button>
            <button 
              :disabled="exportLoading || !tracklist" 
              class="flex-1 px-4 py-3 bg-primary hover:bg-primary-dark disabled:opacity-50 rounded"
              @click="() => {
                exportImage('story').then(() => exportImage('square'));
              }"
            >
              Export Both
            </button>
          </div>
          
          <div v-if="exportError" class="mt-2 p-3 bg-red-900 rounded text-red-300 text-sm">
            {{ exportError }}
          </div>
        </div>
      </div>
    </div>
    
    <!-- BOTTOM SECTION: Past Tracklists -->
    <div v-if="pastTracklists.value.length > 0" class="mt-8">
      <h2 class="text-xl font-bold mb-4">Past Tracklists</h2>
      
      <div v-if="pastTracklistsLoading" class="text-center py-8">
        Loading past tracklists...
      </div>
      
      <div v-else-if="pastTracklistsError" class="p-4 bg-red-900 rounded text-red-300">
        {{ pastTracklistsError }}
      </div>
      
      <div v-else class="space-y-4">
        <div 
          v-for="tl in pastTracklists" 
          :key="tl.id"
          class="bg-gray-800 rounded-lg p-4 cursor-pointer hover:bg-gray-700/50"
          @click="loadPastTracklist(tl.id)"
        >
          <div class="flex items-center gap-4">
            <div class="w-16 h-16 flex-shrink-0">
              <div 
                class="w-16 h-16 flex-items-center justify-center rounded bg-gray-700"
              >
                <svg class="h-8 w-8 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 12l2 2 4-4M7.835 4.697a3.42 3.42 0 001.946-.806 3.42 3.42 0 014.438 0 3.42 3.42 0 001.946.806 3.42 3.42 0 013.138 3.138 3.42 3.42 0 00.806 1.946 3.42 3.42 0 010 4.438 3.42 3.42 0 00-.806 1.946 3.42 3.42 0 01-3.138 3.138z"/>
                </svg>
              </div>
            </div>
            <div class="flex-1">
              <h3 class="font-medium">{{ tl.title }}</h3>
              <p class="text-sm text-gray-400">
                {{ tl.tracks?.length || 0 }} tracks • 
                {{ new Date(tl.createdAt).toLocaleDateString() }}
              </p>
            </div>
          </div>
          
          <div class="mt-2">
            <button 
              class="w-full text-left text-sm text-red-400 hover:text-red-300"
              @click="($event) => {
                $event.stopPropagation();
                if (confirm(`Delete "${tl.title}"? This will permanently remove ${tl.tracks?.length || 0} tracks.`)) {
                  deleteTracklistHandler(tl.id);
                }
              }"
            >
              Delete
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Ensure smooth transitions */
* {
  transition: background-color 0.2s, border-color 0.2s;
}

/* Hide file inputs but keep them accessible */
input[type="file"] {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>