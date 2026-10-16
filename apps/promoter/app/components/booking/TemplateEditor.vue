<script setup lang="ts">
import { Plus, Pencil, Trash2, Star } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useBookingsStore } from '~/stores/bookings'
import type { ApiError } from '~/types/event'
import type { OutreachTemplate } from '~/types/bookings'
import { OFFER_VARIABLES } from '~/utils/bookings'

/**
 * Offer template CRUD (04.10-05): list, create/edit form with the variable
 * hints, set-default toggle, soft delete. Updates carry the loaded version
 * (optimistic concurrency — a 409 means the template changed, reload).
 */
const store = useBookingsStore()
const { templates, templatesSec } = storeToRefs(store)

// Rendered via a helper: a literal {{…}} inside a mustache breaks the template parser.
const varLabel = (v: string) => `{{${v}}}`

const editing = ref<OutreachTemplate | null>(null)
const adding = ref(false)
const saving = ref(false)
const error = ref<ApiError | null>(null)
const form = reactive({ name: '', subject: '', body: '', isDefault: false })

function startAdd() {
  error.value = null
  editing.value = null
  form.name = ''; form.subject = ''; form.body = ''; form.isDefault = false
  adding.value = true
}

function startEdit(t: OutreachTemplate) {
  error.value = null
  adding.value = false
  editing.value = t
  form.name = t.name; form.subject = t.subject; form.body = t.body; form.isDefault = t.is_default
}

async function submit() {
  saving.value = true
  error.value = null
  try {
    if (editing.value) {
      await store.updateTemplate(editing.value.id, {
        name: form.name.trim(), subject: form.subject, body: form.body,
        is_default: form.isDefault, version: editing.value.version,
      })
      editing.value = null
    } else {
      await store.createTemplate({
        name: form.name.trim(), subject: form.subject, body: form.body, is_default: form.isDefault,
      })
      adding.value = false
    }
  } catch (e) {
    error.value = e as ApiError
  } finally {
    saving.value = false
  }
}

async function remove(t: OutreachTemplate) {
  if (!window.confirm(`Delete the template "${t.name}"?`)) return
  await store.deleteTemplate(t.id)
}

const canSubmit = computed(() => !saving.value && !!form.name.trim() && !!form.subject.trim() && !!form.body.trim())
</script>

<template>
  <section aria-labelledby="templates-h" class="space-y-2">
    <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px;">
      <h2 id="templates-h" class="section-lbl">OFFER TEMPLATES</h2>
      <button type="button" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" data-testid="template-add-open" @click="startAdd">
        <Plus style="width:14px;height:14px;" aria-hidden="true" /> NEW TEMPLATE
      </button>
    </div>

    <p v-if="templatesSec.error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
      COULD NOT LOAD THE TEMPLATES.
    </p>

    <form v-if="adding || editing" class="hud-card" style="padding:12px 14px;display:grid;gap:10px;" data-testid="template-form" novalidate @submit.prevent="submit">
      <h3 class="section-lbl" style="margin:0;">{{ editing ? 'EDIT TEMPLATE' : 'NEW TEMPLATE' }}</h3>
      <label style="display:grid;gap:2px;font-size:11px;">
        NAME
        <input v-model="form.name" class="hud-input" style="min-height:40px;" data-testid="template-name">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        SUBJECT
        <input v-model="form.subject" class="hud-input" style="min-height:40px;" data-testid="template-subject">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        BODY
        <textarea v-model="form.body" class="hud-input" rows="10" style="min-height:160px;font-family:inherit;" data-testid="template-body"/>
      </label>
      <details class="data-frag" style="font-size:11px;">
        <summary style="cursor:pointer;">AVAILABLE VARIABLES</summary>
        <span style="display:flex;flex-wrap:wrap;gap:4px;margin-top:4px;">
          <code v-for="v in OFFER_VARIABLES" :key="v" class="badge-hud">{{ varLabel(v) }}</code>
        </span>
      </details>
      <label style="display:flex;align-items:center;gap:8px;font-size:12px;">
        <input v-model="form.isDefault" type="checkbox" data-testid="template-default"> DEFAULT TEMPLATE (used when an offer picks no template)
      </label>
      <p v-if="error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
        {{ error.error === 'conflict' ? 'The template changed since you loaded it — reload and try again.' : (error.error === 'invalid' ? `Invalid ${error.field ?? 'input'}: ${error.problem ?? ''}` : 'Could not save the template.') }}
      </p>
      <div style="display:flex;gap:8px;">
        <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" :disabled="!canSubmit" data-testid="template-save">SAVE TEMPLATE</button>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="adding = false; editing = null">CANCEL</button>
      </div>
    </form>

    <KhEmptyState v-if="!templates.length && !adding" title="NO TEMPLATES" hint="Create a template, or offers use the built-in default." action-label="NEW TEMPLATE" @action="startAdd" />
    <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
      <li v-for="t in templates" :key="t.id" class="hud-card" style="display:flex;justify-content:space-between;align-items:center;gap:12px;padding:10px 14px;" data-testid="template-row">
        <span style="min-width:0;">
          <span style="display:block;font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
            {{ t.name }}
            <Star v-if="t.is_default" style="width:12px;height:12px;vertical-align:-1px;" aria-label="Default template" />
          </span>
          <span class="data-frag" style="font-size:11px;">{{ t.subject }} · v{{ t.version }}</span>
        </span>
        <span style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
          <span v-if="t.is_default" class="badge-hud badge-ready">DEFAULT</span>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" aria-label="Edit template" @click="startEdit(t)">
            <Pencil style="width:13px;height:13px;" aria-hidden="true" />
          </button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" style="min-height:36px;" aria-label="Delete template" @click="remove(t)">
            <Trash2 style="width:13px;height:13px;" aria-hidden="true" />
          </button>
        </span>
      </li>
    </ul>
  </section>
</template>
