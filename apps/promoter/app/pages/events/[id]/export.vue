<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import type { ApiError, ExportData } from '~/types/event'
import { embedSnippet, platformFields, staticPage } from '~/utils/exportPack'
import { zip, type ZipFile } from '~/utils/zip'

type Platform = 'ra' | 'facebook' | 'dice' | 'generic'

const store = useEventStore()
const { current } = storeToRefs(store)

const data = ref<ExportData | null>(null)
const jsonld = ref<Record<string, unknown> | null>(null)
const ics = ref('')
const failure = ref<ApiError | null>(null)
const loading = ref(false)
const og = ref<{ toBlob: () => Promise<Blob | null> } | null>(null)
const platform = ref<Platform>('ra')
const unsavedDraft = ref(false)

async function build() {
  if (!current.value) return
  const id = current.value.id
  loading.value = true
  failure.value = null
  try {
    const x = await store.fetchExport(id)
    const [ld, cal] = await Promise.all([
      x.event.visibility === 'private' ? Promise.resolve(null) : store.fetchJsonLd(id),
      store.fetchIcs(id),
    ])
    data.value = x
    jsonld.value = ld
    ics.value = cal
  } catch (e) {
    data.value = null
    failure.value = e as ApiError
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  try {
    unsavedDraft.value = !!localStorage.getItem(`klubhub-promoter:lineup-draft:${current.value?.id}`)
  } catch { /* storage blocked */ }
  build()
})

const isPrivate = computed(() => data.value?.event.visibility === 'private')
const fields = computed(() => (data.value ? platformFields(data.value, platform.value) : []))
const page = computed(() => (data.value && !isPrivate.value ? staticPage(data.value, jsonld.value) : ''))
const embed = computed(() => (data.value && !isPrivate.value ? embedSnippet(data.value) : ''))
const jsonText = computed(() => (jsonld.value ? JSON.stringify(jsonld.value, null, 2) : ''))

const REQUIRED = ['name', 'startDate', 'location']
const RECOMMENDED = ['endDate', 'eventStatus', 'eventAttendanceMode', 'performer', 'organizer', 'offers', 'image', 'description']
const present = (k: string) => {
  const v = jsonld.value?.[k]
  return Array.isArray(v) ? v.length > 0 : v != null && v !== ''
}
const ldMeta = computed(() => `REQUIRED ${REQUIRED.filter(present).length}/${REQUIRED.length} · RECOMMENDED ${RECOMMENDED.filter(present).length}/${RECOMMENDED.length}`)
const ldMissing = computed(() => RECOMMENDED.filter(k => !present(k)))

const base = computed(() => `${data.value?.event.slug ?? 'event'}${data.value?.embargoed ? '-embargoed' : ''}`)

function save(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
const saveText = (text: string, name: string, type: string) => save(new Blob([text], { type }), name)

async function downloadOg() {
  const b = await og.value?.toBlob()
  if (b) save(b, `${base.value}-og.png`)
}

function readme(x: ExportData): string {
  const lines = [
    `${x.event.title} — export pack (version ${x.event.version})`,
    x.embargoed ? 'EMBARGOED: do not post before the event goes live.' : '',
    x.location.withheld ? `Venue withheld: the pack says "${x.location.name}".` : '',
    '',
    'event.ics            calendar entry',
    'copy/*.txt           fields to paste into RA, Facebook, DICE or anywhere else',
  ]
  if (x.event.visibility !== 'private') {
    lines.push('index.html           standalone event page with og-image.png', 'embed.html           card to paste into another site', 'event.jsonld.json    schema.org MusicEvent')
  }
  return `${lines.filter((l, i) => l || i > 2).join('\n')}\n`
}

const zipping = ref(false)
async function downloadZip() {
  const x = data.value
  if (!x) return
  zipping.value = true
  try {
    const files: ZipFile[] = [{ name: 'README.txt', data: readme(x) }, { name: 'event.ics', data: ics.value }]
    for (const p of ['ra', 'facebook', 'dice', 'generic'] as Platform[]) {
      files.push({ name: `copy/${p}.txt`, data: platformFields(x, p).map(f => `${f.label}\n${f.value}`).join('\n\n') + '\n' })
    }
    if (!isPrivate.value) {
      const png = await og.value?.toBlob()
      files.push({ name: 'index.html', data: page.value }, { name: 'embed.html', data: embed.value }, { name: 'event.jsonld.json', data: jsonText.value })
      if (png) files.push({ name: 'og-image.png', data: new Uint8Array(await png.arrayBuffer()) })
    }
    save(new Blob([zip(files)], { type: 'application/zip' }), `${base.value}-export.zip`)
  } finally {
    zipping.value = false
  }
}

const showPreview = ref(false)
const PLATFORMS: { id: Platform, label: string }[] = [
  { id: 'ra', label: 'RA' }, { id: 'facebook', label: 'FACEBOOK' }, { id: 'dice', label: 'DICE' }, { id: 'generic', label: 'GENERIC' },
]
</script>

<template>
  <div v-if="current" class="space-y-3">
    <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;">
      <p class="page-sub" style="margin:0;">Everything to announce this event on other platforms and your own site.</p>
      <button type="button" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="!data || zipping" @click="downloadZip">
        {{ zipping ? 'PACKING…' : 'DOWNLOAD ZIP' }}
      </button>
    </div>

    <p v-if="loading && !data" role="status" class="data-frag" style="font-size:9px;">GENERATING PACK…</p>

    <div v-else-if="failure?.error === 'timetable_errors'" role="alert" class="glass" style="padding:12px 16px;border-left:3px solid var(--color-error);">
      <div class="section-lbl">EXPORT BLOCKED · THE TIMETABLE HAS {{ failure.issues?.filter(i => i.severity === 'error').length ?? 0 }} PROBLEMS</div>
      <p style="margin:6px 0 0;font-size:13px;">A conflicted timetable must not reach the public. Fix it, then come back.</p>
      <NuxtLink :to="`/events/${current.id}/lineup`" class="btn-hud btn-hud-ghost" style="margin-top:8px;min-height:44px;">OPEN TIMETABLE</NuxtLink>
    </div>
    <div v-else-if="failure" role="alert" class="glass" style="padding:12px 16px;border-left:3px solid var(--color-error);">
      COULD NOT BUILD THE PACK.
      <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="build">RETRY</button>
    </div>

    <template v-if="data">
      <ExportWithheldPanel :data="data" :current-version="current.version" :unsaved-draft="unsavedDraft" @rebuild="build" />

      <div class="export-grid">
        <div class="space-y-3" style="min-width:0;">
          <ExportOgImagePreview v-if="!isPrivate" ref="og" :data="data" @download="downloadOg" />
          <section v-if="!isPrivate" class="hud-card" style="padding:12px 14px;">
            <h3 class="section-lbl" style="margin:0 0 6px;">STATIC PAGE</h3>
            <p style="margin:0 0 8px;font-size:12px;color:var(--color-on-surface-variant);">
              A standalone page with share tags and structured data. Upload it with og-image.png to your own site.
            </p>
            <div style="display:flex;flex-wrap:wrap;gap:6px;">
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :aria-expanded="showPreview" @click="showPreview = !showPreview">{{ showPreview ? 'HIDE PREVIEW' : 'PREVIEW' }}</button>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="saveText(page, `${base}.html`, 'text/html')">DOWNLOAD HTML</button>
            </div>
            <!-- sandbox without allow-scripts / allow-same-origin: the preview can't run code or reach the app -->
            <iframe
              v-if="showPreview" :srcdoc="page" sandbox="" title="Static page preview" loading="lazy"
              style="display:block;width:100%;height:420px;margin-top:8px;border:1px solid var(--color-outline-variant);background:#0e0e0f;"
            />
          </section>
          <section class="hud-card" style="padding:12px 14px;">
            <h3 class="section-lbl" style="margin:0 0 6px;">CALENDAR (ICS)</h3>
            <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="saveText(ics, `${base}.ics`, 'text/calendar')">DOWNLOAD .ICS</button>
          </section>
        </div>

        <section class="hud-card" style="padding:12px 14px;min-width:0;" aria-labelledby="copy-h">
          <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;justify-content:space-between;">
            <h3 id="copy-h" class="section-lbl" style="margin:0;">COPY FIELDS</h3>
            <div role="radiogroup" aria-label="Platform" style="display:flex;flex-wrap:wrap;gap:4px;">
              <button
                v-for="p in PLATFORMS" :key="p.id" type="button" role="radio" :aria-checked="platform === p.id"
                class="btn-hud btn-hud-sm" :class="platform === p.id ? 'btn-hud-cta' : 'btn-hud-ghost'" style="min-height:44px;"
                @click="platform = p.id"
              >
                {{ p.label }}
              </button>
            </div>
          </div>
          <ExportCopyField v-for="f in fields" :key="`${platform}-${f.label}`" :field="f" />
          <div style="margin-top:8px;">
            <ExportCopyButton :text="fields.filter(f => f.value).map(f => `${f.label}\n${f.value}`).join('\n\n')" label="Copy all fields" caption="COPY ALL FIELDS" />
          </div>
        </section>
      </div>

      <div v-if="!isPrivate" class="export-grid">
        <ExportCodeBlock title="EMBED" :code="embed" filename="EMBED.HTML" @download="saveText(embed, `${base}-embed.html`, 'text/html')" />
        <ExportCodeBlock title="JSON-LD" :code="jsonText" :meta="ldMeta" filename="JSON" @download="saveText(jsonText, `${base}.jsonld.json`, 'application/ld+json')" />
      </div>
      <p v-if="!isPrivate && ldMissing.length" class="data-frag" style="font-size:9px;">
        JSON-LD RECOMMENDED BUT MISSING: {{ ldMissing.join(', ').toUpperCase() }}
      </p>
    </template>
  </div>
</template>

<style scoped>
.export-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
}
@media (min-width: 1024px) {
  .export-grid { grid-template-columns: minmax(0, 5fr) minmax(0, 7fr); }
}
</style>
