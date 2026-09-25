<script setup lang="ts">
// Searchable gig combobox (ARIA 1.2 combobox + listbox). Disabled gigs stay
// listed with their reason so the user learns why a gig is not offered.
import { computed, nextTick, ref, watch } from 'vue'
import { matchesGigQuery, type GigChoice } from '../../utils/invoiceGigChoices'

const props = withDefaults(defineProps<{
  choices: GigChoice[]
  modelValue: string | null
  inputId?: string
  describedBy?: string
  invalid?: boolean
}>(), { inputId: 'inv-gig-picker', describedBy: undefined, invalid: false })

const emit = defineEmits<{ 'update:modelValue': [id: string] }>()

const listId = `${props.inputId}-list`
const open = ref(false)
const query = ref('')
const activeIndex = ref(-1)
const listEl = ref<HTMLElement | null>(null)

const selected = computed(() => props.choices.find((c) => c.id === props.modelValue) ?? null)

function display(c: GigChoice | null): string {
  return c ? `${c.label} · ${c.dateLabel}` : ''
}

watch(selected, (c) => { if (!open.value) query.value = display(c) }, { immediate: true })

const filtered = computed(() => {
  const q = query.value === display(selected.value) ? '' : query.value
  return props.choices.filter((c) => matchesGigQuery(c, q))
})
const billable = computed(() => filtered.value.filter((c) => c.group === 'billable'))
const other = computed(() => filtered.value.filter((c) => c.group === 'other'))
/** Flat order as rendered, for keyboard movement. */
const ordered = computed(() => [...billable.value, ...other.value])

function optionId(c: GigChoice): string {
  return `${props.inputId}-opt-${c.id}`
}

const activeId = computed(() => {
  const c = ordered.value[activeIndex.value]
  return open.value && c ? optionId(c) : undefined
})

function openList(): void {
  if (open.value) return
  open.value = true
  const idx = ordered.value.findIndex((c) => c.id === props.modelValue)
  activeIndex.value = idx >= 0 ? idx : ordered.value.findIndex((c) => !c.disabledReason)
}

function closeList(): void {
  open.value = false
  query.value = display(selected.value)
}

function choose(c: GigChoice): void {
  if (c.disabledReason) return
  emit('update:modelValue', c.id)
  open.value = false
  query.value = display(c)
}

function move(delta: number): void {
  openList()
  const n = ordered.value.length
  if (!n) return
  activeIndex.value = (activeIndex.value + delta + n) % n
  void nextTick(() => {
    const id = activeId.value
    if (id) listEl.value?.querySelector(`#${CSS.escape(id)}`)?.scrollIntoView({ block: 'nearest' })
  })
}

function onKeydown(e: KeyboardEvent): void {
  switch (e.key) {
    case 'ArrowDown': e.preventDefault(); move(1); break
    case 'ArrowUp': e.preventDefault(); move(-1); break
    case 'Enter': {
      if (!open.value) return
      e.preventDefault()
      const c = ordered.value[activeIndex.value]
      if (c) choose(c)
      break
    }
    case 'Escape':
      if (open.value) { e.preventDefault(); e.stopPropagation(); closeList() }
      break
  }
}

function onInput(e: Event): void {
  query.value = (e.target as HTMLInputElement).value
  open.value = true
  activeIndex.value = ordered.value.findIndex((c) => !c.disabledReason)
}
</script>

<template>
  <div class="gp">
    <input
      :id="inputId"
      class="hud-input"
      role="combobox"
      type="text"
      autocomplete="off"
      placeholder="Search gigs…"
      aria-autocomplete="list"
      :aria-expanded="open"
      :aria-controls="listId"
      :aria-activedescendant="activeId"
      :aria-describedby="describedBy"
      :aria-invalid="invalid ? 'true' : undefined"
      :value="query"
      @focus="openList"
      @click="openList"
      @input="onInput"
      @keydown="onKeydown"
      @blur="closeList"
    >
    <div v-show="open" :id="listId" ref="listEl" role="listbox" class="gp-list" aria-label="Gigs">
      <template v-for="grp in [{ key: 'billable', label: 'READY TO BILL', items: billable }, { key: 'other', label: 'OTHER GIGS', items: other }]" :key="grp.key">
        <div v-if="grp.items.length" role="group" :aria-labelledby="`${inputId}-grp-${grp.key}`">
          <div :id="`${inputId}-grp-${grp.key}`" class="section-lbl gp-grp" role="presentation">{{ grp.label }}</div>
          <div
            v-for="c in grp.items"
            :id="optionId(c)"
            :key="c.id"
            role="option"
            class="gp-opt"
            :class="{ 'gp-active': activeId === optionId(c), 'gp-disabled': !!c.disabledReason }"
            :aria-selected="c.id === modelValue"
            :aria-disabled="c.disabledReason ? 'true' : undefined"
            @mousedown.prevent="choose(c)"
          >
            <span class="gp-main">
              <span class="gp-label">{{ c.label }}</span>
              <span class="gp-meta">{{ c.dateLabel }} · {{ c.status.toUpperCase() }}</span>
            </span>
            <span class="gp-side">
              <span class="gp-fee">{{ c.feeLabel }}</span>
              <span v-if="c.disabledReason" class="gp-reason">{{ c.disabledReason }}</span>
            </span>
          </div>
        </div>
      </template>
      <div v-if="!ordered.length" class="gp-empty" role="presentation">No gigs match.</div>
    </div>
  </div>
</template>

<style scoped>
.gp { position: relative; }
.gp-list { position: absolute; z-index: 10; left: 0; right: 0; top: calc(100% + 2px); max-height: 260px; overflow-y: auto; background: var(--color-surface-container-highest); border: 1px dashed color-mix(in srgb, var(--color-primary) 25%, transparent); }
.gp-grp { padding: 8px 10px 4px; }
.gp-opt { display: flex; justify-content: space-between; gap: 10px; padding: 8px 10px; cursor: pointer; border-left: 2px solid transparent; }
.gp-active { background: color-mix(in srgb, var(--color-primary) 8%, transparent); border-left-color: var(--color-primary); }
.gp-disabled { cursor: not-allowed; }
.gp-disabled .gp-label, .gp-disabled .gp-fee { color: var(--color-tertiary); }
.gp-main, .gp-side { display: flex; flex-direction: column; min-width: 0; }
.gp-side { align-items: flex-end; flex-shrink: 0; }
.gp-label { font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.gp-meta, .gp-reason { font-family: var(--font-terminal); font-size: 8px; letter-spacing: .06em; text-transform: uppercase; color: var(--color-tertiary); margin-top: 2px; }
.gp-reason { color: var(--color-on-surface-variant); }
.gp-fee { font-family: var(--font-command); font-size: 12px; font-weight: 700; letter-spacing: -.02em; color: var(--color-on-surface); }
.gp-empty { padding: 12px 10px; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
@media (max-width: 768px) {
  .gp-opt { min-height: 44px; align-items: center; }
  .hud-input { min-height: 44px; }
}
</style>
