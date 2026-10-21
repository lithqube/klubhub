<script setup lang="ts">
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Pencil, Plus, Star, Trash2 } from 'lucide-vue-next'
import { useOutreachStore } from '~/stores/outreach'
import type { OutreachTemplate } from '~/types/outreach'
import { OUTREACH_VARIABLES } from '~/utils/outreach'

/**
 * Template editor (04.10-04): the DJ's booking-request templates. Editing
 * here is the ONLY place a template is mutated — the composer works on a
 * per-request copy (RIDER-02 precedent). PUT carries the version as the
 * optimistic-concurrency token (409 on mismatch).
 */
const store = useOutreachStore()
const { templates, templatesSec } = storeToRefs(store)

const editing = ref<OutreachTemplate | 'new' | null>(null)
const name = ref('')
const subject = ref('')
const body = ref('')
const isDefault = ref(false)
const formError = ref('')

const variableHints = OUTREACH_VARIABLES.map(v => `{{${v}}}`).join('  ')

function startNew() {
  editing.value = 'new'
  name.value = ''
  subject.value = ''
  body.value = ''
  isDefault.value = false
  formError.value = ''
}

function startEdit(t: OutreachTemplate) {
  editing.value = t
  name.value = t.name
  subject.value = t.subject
  body.value = t.body
  isDefault.value = t.is_default
  formError.value = ''
}

function close() {
  editing.value = null
  formError.value = ''
}

async function save() {
  formError.value = ''
  if (!name.value.trim()) { formError.value = 'Name is required.'; return }
  if (!subject.value.trim()) { formError.value = 'Subject is required.'; return }
  if (!body.value.trim()) { formError.value = 'Body is required.'; return }
  try {
    if (editing.value === 'new' || editing.value === null) {
      await store.saveTemplate({
        name: name.value.trim(), subject: subject.value.trim(), body: body.value.trim(), is_default: isDefault.value,
      })
    } else {
      await store.updateTemplate(editing.value.id, {
        name: name.value.trim(), subject: subject.value.trim(), body: body.value.trim(),
        is_default: isDefault.value, version: editing.value.version,
      })
    }
    close()
  } catch (e) {
    const err = e as { statusCode?: number, data?: { error?: unknown } }
    formError.value = err.statusCode === 409
      ? 'The template changed since you loaded it — reload and try again.'
      : (err?.data?.error as string) || 'Could not save the template.'
  }
}

async function remove(t: OutreachTemplate) {
  if (!window.confirm(`Delete the template "${t.name}"? Sent requests keep their rendered copy.`)) return
  await store.removeTemplate(t.id).catch(() => { /* templatesSec.error shows it */ })
}
</script>

<template>
  <section aria-labelledby="tpl-h" class="space-y-2">
    <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;">
      <h2 id="tpl-h" class="section-lbl" style="margin:0;">TEMPLATES</h2>
      <button
        v-if="editing === null" type="button" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:36px;"
        data-testid="template-add-open" @click="startNew"
      >
        <Plus style="width:12px;height:12px;" aria-hidden="true" /> NEW TEMPLATE
      </button>
    </div>

    <form v-if="editing !== null" class="hud-card glass" style="padding:14px;display:grid;gap:10px;" data-testid="template-form" @submit.prevent="save">
      <label style="display:grid;gap:4px;">
        <span class="data-frag" style="font-size:10px;">NAME</span>
        <input v-model="name" type="text" class="hud-input" data-testid="template-name" >
      </label>
      <label style="display:grid;gap:4px;">
        <span class="data-frag" style="font-size:10px;">SUBJECT</span>
        <input v-model="subject" type="text" class="hud-input" data-testid="template-subject" >
      </label>
      <label style="display:grid;gap:4px;">
        <span class="data-frag" style="font-size:10px;">BODY</span>
        <textarea v-model="body" class="hud-textarea" rows="10" data-testid="template-body"/>
        <span class="data-frag" style="font-size:10px;opacity:.7;" data-testid="template-vars">{{ variableHints }}</span>
      </label>
      <label style="display:flex;align-items:center;gap:8px;">
        <input v-model="isDefault" type="checkbox" data-testid="template-default" >
        <span class="data-frag" style="font-size:11px;">USE AS DEFAULT</span>
      </label>
      <p v-if="formError" role="alert" class="data-frag" style="font-size:11px;color:var(--color-error);margin:0;" data-testid="template-error">{{ formError }}</p>
      <div style="display:flex;gap:8px;">
        <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:36px;" data-testid="template-save">
          {{ editing === 'new' ? 'CREATE TEMPLATE' : 'SAVE TEMPLATE' }}
        </button>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:36px;" data-testid="template-cancel" @click="close">CANCEL</button>
      </div>
    </form>

    <p v-if="templatesSec.loading && !templates.length" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
    <KhEmptyState v-else-if="!templates.length" title="NO TEMPLATES" hint="Create a template to reuse the same booking-request copy." />
    <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
      <li
        v-for="t in templates" :key="t.id" class="hud-card"
        style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;"
        data-testid="template-row"
      >
        <span style="min-width:0;">
          <span style="display:block;font-size:14px;font-weight:600;">
            {{ t.name }}
            <span v-if="t.is_default" class="badge-hud badge-ready" style="margin-left:6px;">
              <Star style="width:10px;height:10px;" aria-hidden="true" /> DEFAULT
            </span>
            <span class="data-frag" style="font-size:11px;margin-left:6px;">v{{ t.version }}</span>
          </span>
          <span class="data-frag" style="font-size:11px;">{{ t.subject }}</span>
        </span>
        <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
          <button
            type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;"
            aria-label="Edit template" data-testid="template-edit" @click="startEdit(t)"
          >
            <Pencil style="width:12px;height:12px;" aria-hidden="true" />
          </button>
          <button
            type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;"
            aria-label="Delete template" data-testid="template-delete" @click="remove(t)"
          >
            <Trash2 style="width:12px;height:12px;" aria-hidden="true" />
          </button>
        </span>
      </li>
    </ul>
  </section>
</template>
