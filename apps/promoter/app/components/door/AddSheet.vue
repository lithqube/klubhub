<script setup lang="ts">
import { Minus, Plus, X } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import type { ApiError } from '~/types/event'

/**
 * On-the-spot add: name, +N, list and a manager PIN. The PIN is checked on
 * the device (it works offline); the add and the check-in are queued and
 * the server checks the PIN again when they sync. No list is preselected
 * (a wrong default would put the guest on the wrong terms). A modal:
 * focus starts on NAME, stays inside the sheet, and Escape closes it.
 */
const emit = defineEmits<{ added: [r: { id: string, nonce: string, name: string, count: number }], close: [] }>()
const store = useDoorStore()
const { bundle } = storeToRefs(store)
const uid = useId()

const name = ref('')
const plus = ref(0)
const listId = ref('')
const form = ref<HTMLFormElement | null>(null)
const nameInput = ref<HTMLInputElement | null>(null)
onMounted(() => nameInput.value?.focus())

/** Escape closes; Tab cycles inside the sheet. */
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
    return
  }
  if (e.key !== 'Tab' || !form.value) return
  const els = [...form.value.querySelectorAll<HTMLElement>('button, input, select, textarea, [tabindex]:not([tabindex="-1"])')]
    .filter(el => !(el as HTMLButtonElement).disabled)
  const first = els[0]
  const last = els.at(-1)
  if (!first || !last) return
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}
const pin = ref('')
const busy = ref(false)
const error = ref('')
const noManagerPin = computed(() => !bundle.value?.manager_pin)

async function submit() {
  error.value = ''
  if (!name.value.trim()) {
    error.value = 'Enter a name.'
    return
  }
  if (!listId.value) {
    error.value = 'Pick a list.'
    return
  }
  busy.value = true
  try {
    const count = 1 + plus.value
    const r = await store.addGuest({ name: name.value, plus_n: plus.value, list_id: listId.value, manager_pin: pin.value, count })
    emit('added', { ...r, name: name.value.replace(/\s+/g, ' ').trim(), count })
  } catch (e) {
    const err = e as ApiError
    error.value = ({
      manager_pin_invalid: 'Wrong manager PIN. Ask the manager on shift.',
      no_manager_pin: 'No manager PIN is set for this event. Generate one in the event’s DOOR tab, then log in here again.',
    } as Record<string, string>)[err.error] ?? (err.field === 'list_id' ? 'Pick a list.' : 'Could not add this guest.')
    pin.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="sheet" role="dialog" aria-modal="true" :aria-labelledby="`${uid}-h`" @keydown="onKey">
    <form ref="form" class="glass body" novalidate @submit.prevent="submit">
      <div class="head">
        <h2 :id="`${uid}-h`" class="section-lbl" style="margin:0;font-size:11px;">ADD AT THE DOOR</h2>
        <button type="button" class="btn-hud btn-hud-ghost close" aria-label="Close" @click="emit('close')">
          <X style="width:22px;height:22px;" aria-hidden="true" />
        </button>
      </div>
      <p v-if="noManagerPin" role="alert" class="err">
        No manager PIN is set for this event. Generate one in the event’s DOOR tab, then log in here again.
      </p>
      <div class="f">
        <label :for="`${uid}-name`" class="section-lbl">NAME</label>
        <input
          :id="`${uid}-name`" ref="nameInput" v-model="name" class="hud-input big" autocomplete="off" autocapitalize="words" maxlength="120"
          required
        >
      </div>
      <div class="f">
        <span :id="`${uid}-plus`" class="section-lbl">PLUS</span>
        <div class="stepper" role="group" :aria-labelledby="`${uid}-plus`">
          <button type="button" class="btn-hud btn-hud-ghost step" :disabled="plus <= 0" aria-label="One fewer plus" @click="plus--">
            <Minus style="width:20px;height:20px;" aria-hidden="true" />
          </button>
          <output class="n" :aria-label="`plus ${plus}`">+{{ plus }}</output>
          <button type="button" class="btn-hud btn-hud-ghost step" :disabled="plus >= 10" aria-label="One more plus" @click="plus++">
            <Plus style="width:20px;height:20px;" aria-hidden="true" />
          </button>
        </div>
      </div>
      <div class="f">
        <label :for="`${uid}-list`" class="section-lbl">LIST</label>
        <select :id="`${uid}-list`" v-model="listId" class="hud-input big" required>
          <option value="" disabled>Pick a list</option>
          <option v-for="l in bundle?.lists ?? []" :key="l.id" :value="l.id">{{ l.name }}</option>
        </select>
      </div>
      <div class="f">
        <label :for="`${uid}-pin`" class="section-lbl">MANAGER PIN</label>
        <input
          :id="`${uid}-pin`" v-model="pin" class="hud-input big" type="password" inputmode="numeric" pattern="\d{6}" maxlength="6"
          autocomplete="off" required
        >
      </div>
      <p v-if="error" role="alert" class="err">{{ error }}</p>
      <button type="submit" class="btn-hud btn-hud-cta go" :disabled="busy || noManagerPin || pin.length !== 6 || !listId">
        {{ busy ? 'CHECKING PIN…' : `ADD & ADMIT ${1 + plus} NOW` }}
      </button>
    </form>
  </div>
</template>

<style scoped>
.sheet {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  background: rgba(0, 0, 0, .6);
  overflow-y: auto;
}
.body {
  width: 100%;
  max-width: 520px;
  padding: 12px 16px calc(16px + env(safe-area-inset-bottom));
  display: grid;
  gap: 12px;
  background: var(--color-surface-container);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.close {
  min-width: 56px;
  min-height: 56px;
  height: 56px;
}
.f {
  display: grid;
  gap: 4px;
}
.big {
  height: 56px;
  font-size: 18px;
}
.stepper {
  display: grid;
  grid-template-columns: 64px 1fr 64px;
  align-items: center;
  gap: 8px;
}
.step {
  height: 56px;
  min-height: 56px;
}
.n {
  text-align: center;
  font-family: var(--font-command);
  font-size: 26px;
  font-weight: 700;
  color: var(--color-on-surface);
}
.err {
  margin: 0;
  font-size: 14px;
  color: var(--color-error);
}
.go {
  min-height: 64px;
  height: 64px;
  font-size: 13px;
}
</style>
