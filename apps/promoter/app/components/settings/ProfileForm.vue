<script setup lang="ts">
import type { ApiError } from '~/types/event'
import type { Organization, OrgProfile } from '~/types/org'
import { contrast, DEFAULT_ACCENT, EXPORT_BG, isHex } from '~/utils/color'

const props = defineProps<{ org: Organization, busy?: boolean, serverError?: ApiError | null, readOnly?: boolean }>()
const emit = defineEmits<{ submit: [OrgProfile] }>()

const f = reactive({
  bio: props.org.bio,
  website_url: props.org.website_url ?? '',
  instagram_url: props.org.instagram_url ?? '',
  soundcloud_url: props.org.soundcloud_url ?? '',
  ra_url: props.org.ra_url ?? '',
  accent: props.org.accent_color ?? '',
})
const LINKS = [
  { key: 'website_url', label: 'WEBSITE', placeholder: 'https://your-collective.example' },
  { key: 'instagram_url', label: 'INSTAGRAM', placeholder: 'https://instagram.com/…' },
  { key: 'soundcloud_url', label: 'SOUNDCLOUD', placeholder: 'https://soundcloud.com/…' },
  { key: 'ra_url', label: 'RESIDENT ADVISOR', placeholder: 'https://ra.co/promoters/…' },
] as const

const touched = reactive<Record<string, boolean>>({})
const errors = computed(() => {
  const e: Record<string, string> = {}
  if ([...f.bio].length > 2000) e.bio = 'At most 2000 characters.'
  for (const l of LINKS) {
    const v = f[l.key].trim()
    if (v && !/^https:\/\/[^/@\s]+/.test(v)) e[l.key] = 'Use a full https:// link.'
  }
  if (f.accent && !isHex(f.accent)) e.accent_color = 'A hex colour like #96f8ff.'
  const s = props.serverError
  if (s?.field && !e[s.field]) e[s.field] = s.problem ? `${s.problem[0]!.toUpperCase()}${s.problem.slice(1)}.` : 'Check this field.'
  return e
})
const show = (k: string) => (touched[k] || props.serverError?.field === k ? errors.value[k] : undefined)

const accent = computed(() => (isHex(f.accent) ? f.accent.toLowerCase() : DEFAULT_ACCENT))
const ratio = computed(() => contrast(accent.value, EXPORT_BG))

function submit() {
  for (const k of ['bio', 'accent_color', ...LINKS.map(l => l.key)]) touched[k] = true
  const first = Object.keys(errors.value)[0]
  if (first) {
    document.getElementById(`pf-${first}`)?.focus()
    return
  }
  emit('submit', {
    bio: f.bio.trim(),
    website_url: f.website_url.trim() || null,
    instagram_url: f.instagram_url.trim() || null,
    soundcloud_url: f.soundcloud_url.trim() || null,
    ra_url: f.ra_url.trim() || null,
    accent_color: f.accent ? f.accent.toLowerCase() : null,
  })
}
</script>

<template>
  <form novalidate class="space-y-3" @submit.prevent="submit">
    <p class="glass" style="padding:10px 14px;margin:0;font-size:12px;border-left:3px solid var(--color-secondary);">
      PUBLIC · Everything here goes into export packs and share images. Keep personal emails and phone numbers out.
    </p>
    <fieldset :disabled="readOnly" style="border:0;padding:0;margin:0;" class="space-y-3">
      <section class="glass" style="padding:18px;" aria-label="About">
        <label class="block">
          <span class="section-lbl">BIO</span>
          <textarea id="pf-bio" v-model="f.bio" class="hud-textarea" rows="4" maxlength="2000" :aria-invalid="!!show('bio')" aria-describedby="pf-bio-count" @blur="touched.bio = true" />
        </label>
        <div id="pf-bio-count" class="data-frag" style="font-size:8px;margin-top:4px;">{{ [...f.bio].length }}/2000</div>
        <span v-if="show('bio')" style="font-size:12px;color:var(--color-error);">{{ show('bio') }}</span>
      </section>

      <section class="glass" style="padding:18px;display:grid;gap:10px;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));" aria-label="Links">
        <label v-for="l in LINKS" :key="l.key">
          <span class="section-lbl">{{ l.label }}</span>
          <input
            :id="`pf-${l.key}`" v-model="f[l.key]" class="hud-input" type="url" inputmode="url" :placeholder="l.placeholder"
            :aria-invalid="!!show(l.key)" @blur="touched[l.key] = true"
          >
          <span v-if="show(l.key)" style="font-size:12px;color:var(--color-error);">{{ show(l.key) }}</span>
        </label>
      </section>

      <section class="glass" style="padding:18px;" aria-labelledby="pf-accent-h">
        <h2 id="pf-accent-h" class="section-lbl" style="margin:0 0 8px;">ACCENT COLOUR</h2>
        <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:center;">
          <input :value="accent" type="color" aria-label="Pick accent colour" style="width:44px;height:44px;padding:0;border:1px solid var(--color-outline-variant);background:none;" @input="f.accent = ($event.target as HTMLInputElement).value">
          <label>
            <span class="sr-only">Accent colour hex</span>
            <input id="pf-accent_color" v-model="f.accent" class="hud-input" placeholder="#96f8ff (default)" maxlength="7" style="width:160px;" :aria-invalid="!!show('accent_color')" @blur="touched.accent_color = true">
          </label>
          <button v-if="f.accent" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="f.accent = ''">USE DEFAULT</button>
          <div aria-hidden="true" :style="{ background: EXPORT_BG, color: accent, borderLeft: `4px solid ${accent}`, padding: '8px 14px', fontWeight: 700, textTransform: 'uppercase', fontSize: '13px' }">
            {{ org.name }} presents
          </div>
        </div>
        <span v-if="show('accent_color')" style="display:block;font-size:12px;color:var(--color-error);margin-top:6px;">{{ show('accent_color') }}</span>
        <p style="margin:8px 0 0;font-size:12px;" :style="ratio < 4.5 ? 'color:var(--color-secondary);font-weight:600;' : 'color:var(--color-on-surface-variant);'" aria-live="polite">
          <template v-if="ratio < 4.5">Contrast {{ ratio.toFixed(1) }}:1 on the dark export background — hard to read. Pick a lighter colour (4.5:1 or more).</template>
          <template v-else>Contrast {{ ratio.toFixed(1) }}:1 on the dark export background. Used for share images, pages and embeds.</template>
        </p>
      </section>
    </fieldset>
    <div v-if="!readOnly" style="display:flex;justify-content:flex-end;">
      <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="busy">{{ busy ? 'SAVING…' : 'SAVE PROFILE' }}</button>
    </div>
  </form>
</template>
