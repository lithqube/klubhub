<script setup lang="ts">
import { FileUp } from 'lucide-vue-next'
import { useGuestStore } from '~/stores/guest'
import type { ApiError } from '~/types/event'
import type { ImportField, ImportPreset, ImportResult } from '~/types/guest'
import {
  type CsvTable, decodeCsv, guessMapping, IMPORT_FIELDS, IMPORT_PRESETS, type ImportProblem, importErrorText, MAX_IMPORT_BYTES,
  parseCsv, resolveMapping, TICKET_STATUS_LABEL,
} from '~/utils/attendeeImport'

/**
 * Import attendees who bought tickets elsewhere (P2.2): pick the platform,
 * choose the CSV export, (for other CSVs) pick the columns, check the file
 * — a dry run on the server — then confirm. The browser reads the file
 * only to show its columns; the server parses and applies it. Re-importing
 * a newer export updates tickets instead of duplicating them.
 */
const emit = defineEmits<{ done: [message: string], cancel: [] }>()
const store = useGuestStore()
const uid = useId()

const preset = ref<ImportPreset>('ra')
const file = ref<File | null>(null)
const table = ref<CsvTable | null>(null)
const readError = ref('')
const mapping = ref<Partial<Record<ImportField, string>>>({})
const result = ref<ImportResult | null>(null)
const error = ref('')
const busy = ref<'check' | 'import' | null>(null)

const presetInfo = computed(() => IMPORT_PRESETS.find(p => p.id === preset.value)!)

/** Local mapping preview: which columns the preset (or the user) picked, or what is missing. */
const localMapping = computed(() => {
  if (!table.value) return null
  try {
    return { header: resolveMapping(preset.value, table.value.headers, preset.value === 'generic' ? mapping.value : {}).header, problem: '' }
  } catch (e) {
    return { header: {}, problem: (e as ImportProblem).problem }
  }
})

// Any change invalidates the last check: confirm only what was checked.
watch([preset, file, mapping], () => {
  result.value = null
  error.value = ''
}, { deep: true })

async function choose(ev: Event) {
  const f = (ev.target as HTMLInputElement).files?.[0] ?? null
  file.value = f
  table.value = null
  readError.value = ''
  if (!f) return
  if (f.size > MAX_IMPORT_BYTES) {
    readError.value = importErrorText({ error: 'too_large' })
    return
  }
  try {
    const { text, encoding } = decodeCsv(new Uint8Array(await f.arrayBuffer()))
    table.value = parseCsv(text, encoding)
    mapping.value = guessMapping(table.value.headers)
  } catch (e) {
    readError.value = importErrorText(e as ImportProblem)
  }
}

function pickColumnsMyself() {
  if (table.value) mapping.value = guessMapping(table.value.headers)
  preset.value = 'generic'
}

async function run(dryRun: boolean) {
  if (!file.value) return
  busy.value = dryRun ? 'check' : 'import'
  error.value = ''
  try {
    const r = await store.importAttendees({
      preset: preset.value, file: file.value, fileName: file.value.name, mapping: preset.value === 'generic' ? mapping.value : undefined,
    }, dryRun)
    if (dryRun) {
      result.value = r
      return
    }
    const c = r.counts
    const n = c.positions_new + c.positions_updated
    emit('done', `Imported ${n} ${n === 1 ? 'ticket' : 'tickets'} from ${presetInfo.value.label}: ${c.positions_new} new, `
      + `${c.positions_updated} updated${c.rejected ? `, ${c.rejected} ${c.rejected === 1 ? 'row' : 'rows'} skipped` : ''}.`)
  } catch (e) {
    error.value = importErrorText(e as ApiError)
  } finally {
    busy.value = null
  }
}

const changes = computed(() => (result.value ? result.value.counts.positions_new + result.value.counts.positions_updated : 0))
const ACTION_LABEL = { new: 'NEW', update: 'UPDATE', unchanged: 'SAME' } as const
</script>

<template>
  <form class="hud-card" style="padding:12px 14px;display:grid;gap:12px;" aria-labelledby="import-h" novalidate @submit.prevent="run(true)">
    <h3 id="import-h" class="section-lbl" style="margin:0;">IMPORT ATTENDEES</h3>
    <p style="margin:0;font-size:12px;color:var(--color-on-surface-variant);">
      Bring in people who bought tickets on another platform so the door finds them. Importing a newer export later updates
      names and refunds instead of adding people twice.
    </p>

    <fieldset style="border:0;padding:0;margin:0;display:grid;gap:6px;">
      <legend class="section-lbl" style="margin-bottom:6px;">1 · PLATFORM</legend>
      <div role="radiogroup" aria-label="Platform" style="display:flex;flex-wrap:wrap;gap:4px;">
        <button
          v-for="p in IMPORT_PRESETS" :key="p.id" type="button" role="radio" :aria-checked="preset === p.id"
          class="btn-hud btn-hud-sm" :class="preset === p.id ? 'btn-hud-cta' : 'btn-hud-ghost'" style="min-height:44px;" @click="preset = p.id"
        >
          {{ p.label }}
        </button>
      </div>
      <span style="font-size:12px;color:var(--color-on-surface-variant);">{{ presetInfo.hint }}</span>
    </fieldset>

    <div style="display:grid;gap:6px;">
      <label :for="`${uid}-file`" class="section-lbl">2 · EXPORT FILE (CSV)</label>
      <input :id="`${uid}-file`" class="hud-input" type="file" accept=".csv,.txt,text/csv,text/plain" style="padding:8px;" @change="choose">
      <p v-if="readError" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ readError }}</p>
      <p v-else-if="table" class="data-frag" style="font-size:9px;margin:0;" data-testid="import-file-summary">
        {{ table.rows.length }} ROWS · {{ table.headers.length }} COLUMNS<template v-if="table.encoding !== 'utf-8'"> · {{ table.encoding.toUpperCase() }}</template>
      </p>
    </div>

    <fieldset v-if="table && preset === 'generic'" style="border:0;padding:0;margin:0;display:grid;gap:6px;">
      <legend class="section-lbl" style="margin-bottom:6px;">3 · COLUMNS</legend>
      <span style="font-size:12px;color:var(--color-on-surface-variant);">
        Needed: a name or email, and an order number, ticket id or barcode so a later re-import can match people.
      </span>
      <div class="map-grid">
        <div v-for="f in IMPORT_FIELDS" :key="f.id" style="display:grid;gap:4px;">
          <label :for="`${uid}-map-${f.id}`" class="section-lbl">{{ f.label }}</label>
          <select :id="`${uid}-map-${f.id}`" v-model="mapping[f.id]" class="hud-input">
            <option :value="undefined">—</option>
            <option v-for="h in table.headers" :key="h" :value="h">{{ h }}</option>
          </select>
        </div>
      </div>
    </fieldset>
    <div v-else-if="table && localMapping" style="display:grid;gap:6px;">
      <span class="section-lbl">3 · COLUMNS FOUND</span>
      <p v-if="localMapping.problem" role="alert" style="margin:0;font-size:13px;color:var(--color-status-archived);">
        This does not look like a {{ presetInfo.label }} export: {{ localMapping.problem }}.
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="pickColumnsMyself">PICK COLUMNS MYSELF</button>
      </p>
      <ul v-else class="found" aria-label="Columns found">
        <li v-for="f in IMPORT_FIELDS.filter(x => localMapping!.header[x.id])" :key="f.id">
          <span class="data-frag" style="font-size:8px;">{{ f.label }}</span> {{ localMapping.header[f.id] }}
        </li>
      </ul>
    </div>

    <p v-if="error" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ error }}</p>

    <section v-if="result" aria-labelledby="import-check-h" style="display:grid;gap:10px;min-width:0;">
      <h4 id="import-check-h" class="section-lbl" style="margin:0;">CHECK · NOTHING SAVED YET</h4>
      <p role="status" style="margin:0;font-size:13px;">
        {{ result.counts.rows }} rows: <strong>{{ result.counts.positions_new }} new</strong> tickets in {{ result.counts.orders_new }} new orders,
        <strong>{{ result.counts.positions_updated }} updated</strong>, {{ result.counts.positions_unchanged }} unchanged<template v-if="result.counts.rejected">,
          <strong style="color:var(--color-status-archived);">{{ result.counts.rejected }} skipped</strong></template>.
      </p>
      <p v-if="result.counts.not_in_file" style="margin:0;font-size:12px;color:var(--color-on-surface-variant);">
        {{ result.counts.not_in_file }} tickets imported earlier from {{ presetInfo.label }} are not in this file; they stay as they are.
      </p>
      <p v-if="result.counts.on_guest_list" style="margin:0;font-size:12px;color:var(--color-on-surface-variant);">
        {{ result.counts.on_guest_list }} ticket holders are also on a guest list.
      </p>
      <ul class="found" aria-label="Ticket types">
        <li v-for="t in result.ticket_types" :key="t.name">
          {{ t.name }} <span class="data-frag" style="font-size:8px;">{{ t.tickets }}<template v-if="t.new"> · NEW TYPE</template></span>
        </li>
      </ul>
      <div v-if="result.preview.length" style="overflow-x:auto;min-width:0;">
        <table class="preview" aria-label="Preview, names shortened">
          <thead><tr><th scope="col">LINE</th><th scope="col">ORDER</th><th scope="col">NAME</th><th scope="col">TICKET</th><th scope="col">STATUS</th><th scope="col">CHANGE</th></tr></thead>
          <tbody>
            <tr v-for="p in result.preview" :key="p.line">
              <td>{{ p.line }}</td><td>{{ p.order_ref }}</td><td>{{ p.name }}</td><td>{{ p.ticket_type }}</td>
              <td>{{ TICKET_STATUS_LABEL[p.status] }}</td><td>{{ ACTION_LABEL[p.action] }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="result.rejected.length">
        <span class="section-lbl">SKIPPED ROWS</span>
        <ul aria-label="Skipped rows" style="margin:4px 0 0;padding-left:18px;font-size:12px;max-height:160px;overflow:auto;">
          <li v-for="r in result.rejected" :key="r.line">Line {{ r.line }}: {{ r.reason }}</li>
        </ul>
      </div>
    </section>

    <div style="display:flex;flex-wrap:wrap;gap:6px;">
      <button v-if="!result" type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="!table || !!busy || !!localMapping?.problem">
        <FileUp style="width:14px;height:14px;" aria-hidden="true" /> {{ busy === 'check' ? 'CHECKING…' : 'CHECK FILE' }}
      </button>
      <button v-else type="button" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="!changes || !!busy" @click="run(false)">
        {{ busy === 'import' ? 'IMPORTING…' : changes ? `IMPORT ${changes} ${changes === 1 ? 'TICKET' : 'TICKETS'}` : 'NOTHING TO CHANGE' }}
      </button>
      <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="emit('cancel')">CLOSE</button>
    </div>
  </form>
</template>

<style scoped>
.map-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 8px;
}
@media (min-width: 640px) {
  .map-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
.found {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  font-size: 12px;
}
.preview {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.preview th {
  text-align: left;
  font-family: var(--font-terminal);
  font-size: 8px;
  letter-spacing: .07em;
  color: var(--color-tertiary);
  padding: 4px 6px;
  border-bottom: 1px solid var(--color-outline-variant);
}
.preview td {
  padding: 4px 6px;
  border-bottom: 1px solid var(--color-outline-variant);
  white-space: nowrap;
}
</style>
