<script setup lang="ts">
import { useContactStore } from '../../stores/contact'
import { onClickOutside } from '@vueuse/core'
import type { Contact } from '../../types/gig'

const props = defineProps<{
  modelValue: Contact | null
  placeholder?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [contact: Contact | null]
  'contact-selected': [contact: Contact]
}>()

const contactStore = useContactStore()
const query = ref(props.modelValue?.name || '')
const results = ref<Contact[]>([])
const isOpen = ref(false)
const isLoading = ref(false)
const highlightedIndex = ref(-1)
const inputRef = ref<HTMLInputElement | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.modelValue,
  (val) => {
    if (val) query.value = val.name
  }
)

async function search() {
  if (query.value.length < 2) {
    results.value = []
    return
  }
  isLoading.value = true
  try {
    results.value = await contactStore.fetchAutocomplete(query.value, 5)
    isOpen.value = results.value.length > 0
    highlightedIndex.value = -1
  } catch {
    results.value = []
  } finally {
    isLoading.value = false
  }
}

function onInput() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(search, 300)
}

function selectContact(contact: Contact) {
  query.value = contact.name
  emit('update:modelValue', contact)
  emit('contact-selected', contact)
  isOpen.value = false
  results.value = []
}

function clear() {
  query.value = ''
  emit('update:modelValue', null)
  results.value = []
  isOpen.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (!isOpen.value) return
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    highlightedIndex.value = Math.min(highlightedIndex.value + 1, results.value.length)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    highlightedIndex.value = Math.max(highlightedIndex.value - 1, -1)
  } else if (e.key === 'Enter' && highlightedIndex.value >= 0) {
    e.preventDefault()
    selectContact(results.value[highlightedIndex.value])
  } else if (e.key === 'Escape') {
    isOpen.value = false
  }
}

onClickOutside(inputRef, () => {
  isOpen.value = false
})
</script>

<template>
  <div style="position:relative;">
    <div style="display:flex;gap:8px;">
      <input
        ref="inputRef"
        v-model="query"
        type="text"
        class="hud-input"
        :placeholder="placeholder || 'Search contact...'"
        style="flex:1;min-width:0;"
        @input="onInput"
        @keydown="onKeydown"
        @focus="query.length >= 2 && search()"
      >
      <button
        v-if="query && !isLoading"
        type="button"
        class="btn-hud"
        style="padding:0 8px;font-size:10px;"
        @click="clear"
      >
        ✕
      </button>
    </div>

    <!-- Loading indicator -->
    <div v-if="isLoading" style="position:absolute;top:100%;left:0;right:0;padding:8px;">
      <span class="section-lbl" style="font-size:8px;color:var(--color-tertiary);">SEARCHING...</span>
    </div>

    <!-- Results dropdown -->
    <div
      v-if="isOpen && results.length > 0"
      class="glass"
      style="position:absolute;top:100%;left:0;right:0;z-index:50;max-height:200px;overflow-y:auto;border-radius:2px;margin-top:2px;"
    >
      <div
        v-for="(contact, idx) in results"
        :key="contact.id"
        style="padding:8px 12px;cursor:pointer;border-bottom:1px solid rgba(255,255,255,0.05);"
        :style="{
          background: idx === highlightedIndex
            ? 'rgba(255,255,255,0.08)'
            : 'transparent',
        }"
        @click="selectContact(contact)"
        @mouseenter="highlightedIndex = idx"
      >
        <div style="font-size:11px;font-weight:600;">{{ contact.name }}</div>
        <div
          v-if="contact.company"
          style="font-size:9px;color:var(--color-tertiary);"
        >
          {{ contact.company }}
        </div>
      </div>
      <div
        style="padding:8px 12px;cursor:pointer;text-align:center;border-top:1px dashed rgba(150,248,255,0.15);"
        @click="isOpen = false"
      >
        <span class="section-lbl" style="font-size:8px;color:var(--color-primary);">CREATE NEW CONTACT</span>
      </div>
    </div>

    <!-- No results -->
    <div
      v-if="isOpen && !isLoading && results.length === 0 && query.length >= 2"
      class="glass"
      style="position:absolute;top:100%;left:0;right:0;z-index:50;padding:12px;text-align:center;border-radius:2px;margin-top:2px;"
    >
      <div class="section-lbl" style="font-size:9px;color:var(--color-tertiary);">NO CONTACTS FOUND</div>
      <div
        style="margin-top:8px;cursor:pointer;"
        @click="isOpen = false"
      >
        <span class="section-lbl" style="font-size:8px;color:var(--color-primary);">CREATE NEW CONTACT</span>
      </div>
    </div>
  </div>
</template>
