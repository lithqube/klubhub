<script setup lang="ts">
import { useGigStore } from '../../stores/gig'
import { useTracklistStore } from '../../stores/tracklist'
import { ref, computed, watch } from 'vue'
import type { Gig, GigCreate, Venue, Contact, GigStatus, PaymentStatus } from '../../types/gig'
import type { Tracklist as TracklistType } from '../../types/tracklist'
import VenueAutocomplete from './VenueAutocomplete.vue'
import ContactAutocomplete from './ContactAutocomplete.vue'

const gigStore = useGigStore()
const tracklistStore = useTracklistStore()

const props = defineProps<{
  open: boolean
  gig: Gig | null
}>()

const emit = defineEmits<{
  'update:open': [val: boolean]
  saved: []
}>()

const isEdit = computed(() => !!props.gig?.id)

const linkedTracklists = ref<Pick<TracklistType, 'id' | 'title'>[]>([])
const availableTracklists = ref<TracklistType[]>([])
const tracklistsLoaded = ref(false)
const linkingTlId = ref('')
const unlinkingTl = ref(false)

async function loadTracklists() {
  if (!props.gig?.id) return
  tracklistsLoaded.value = false
  try {
    const detail = await gigStore.fetchGigDetail(props.gig.id)
    if (detail?.tracklists && Array.isArray(detail.tracklists)) {
      linkedTracklists.value = detail.tracklists.map((t) => ({
        id: t.id,
        title: t.title,
      }))
    }
    await tracklistStore.loadPastTracklists()
    availableTracklists.value = tracklistStore.pastTracklists.filter(
      (tl) => !linkedTracklists.value.some((l) => l.id === tl.id)
    )
  } catch (e) {
    console.error('loadTracklists failed:', e)
  } finally {
    tracklistsLoaded.value = true
  }
}

async function linkTracklist(tlId: string) {
  if (!props.gig?.id) return
  linkingTlId.value = tlId
  const ok = await gigStore.linkTracklist(props.gig.id, tlId)
  if (ok) {
    const tl = await tracklistStore.fetchTracklist(tlId)
    linkedTracklists.value.push({ id: tlId, title: tl?.title || tlId })
    availableTracklists.value = availableTracklists.value.filter((t) => t.id !== tlId)
    tracklistStore.linkedGigs[tlId] = tracklistStore.linkedGigs[tlId] || []
    tracklistStore.linkedGigs[tlId].push(props.gig.id)
  }
  linkingTlId.value = ''
}

async function unlinkTracklist(tlId: string) {
  if (!props.gig?.id) return
  unlinkingTl.value = true
  const ok = await gigStore.unlinkTracklist(props.gig.id, tlId)
  if (ok) {
    linkedTracklists.value = linkedTracklists.value.filter((t) => t.id !== tlId)
    // Re-add to available tracklists so it can be re-linked
    const tl = await tracklistStore.fetchTracklist(tlId).catch(() => null)
    if (tl && !availableTracklists.value.some((t) => t.id === tlId)) {
      availableTracklists.value.push(tl)
    }
    // Remove only this gig's association, preserve other gigs'
    if (tracklistStore.linkedGigs[tlId]) {
      tracklistStore.linkedGigs[tlId] = tracklistStore.linkedGigs[tlId].filter(
        (gigId) => gigId !== props.gig!.id,
      )
    }
  }
  unlinkingTl.value = false
}

watch(() => props.gig?.id, () => { if (props.gig?.id) loadTracklists() }, { immediate: true })

// Dialog header title
const title = computed(() => {
  if (!props.gig) return ''
  if (props.gig.event_name) return props.gig.event_name
  if (props.gig.venue?.name) return props.gig.venue.name
  return ''
})

// Form state
const form = reactive({
  date: '',
  venue: null as Venue | null,
  city: '',
  country: '',
  event_name: '',
  room_details: '',
  contact: null as Contact | null,
  promoter_name: '',
  promoter_email: '',
  promoter_phone: '',
  fee_amount: null as number | null,
  fee_currency: 'EUR',
  notes: '',
  set_length_minutes: null as number | null,
  status: 'inquiry' as GigStatus,
  payment_status: 'unpaid' as PaymentStatus,
})

const showCopyFromDropdown = ref(false)
const copyQuery = ref('')
const copyResults = ref<Gig[]>([])
const showCancelConfirm = ref(false)
const isSaving = ref(false)
const saveError = ref<string | null>(null)

const CURRENCIES = ['EUR', 'USD', 'GBP', 'CHF', 'PLN', 'CZK', 'DKK', 'SEK', 'NOK']
const STATUS_OPTIONS: GigStatus[] = ['inquiry', 'confirmed', 'advanced', 'played', 'cancelled']
const PAYMENT_OPTIONS: PaymentStatus[] = ['unpaid', 'deposit_paid', 'paid', 'overdue', 'waived']

function initForm() {
  if (props.gig) {
    form.date = props.gig.date ? props.gig.date.split('T')[0] : ''
    form.event_name = props.gig.event_name || ''
    form.room_details = ''
    form.city = props.gig.city || ''
    form.country = props.gig.country || ''
    form.promoter_name = props.gig.promoter_name || ''
    form.promoter_email = props.gig.promoter_email || ''
    form.promoter_phone = props.gig.promoter_phone || ''
    form.fee_amount = props.gig.fee_amount || null
    form.fee_currency = props.gig.fee_currency || 'EUR'
    form.notes = props.gig.notes || ''
    form.set_length_minutes = props.gig.set_length_minutes || null
    form.status = props.gig.status
    form.payment_status = props.gig.payment_status
  } else {
    form.date = ''
    form.venue = null
    form.city = ''
    form.country = ''
    form.event_name = ''
    form.room_details = ''
    form.contact = null
    form.promoter_name = ''
    form.promoter_email = ''
    form.promoter_phone = ''
    form.fee_amount = null
    form.fee_currency = 'EUR'
    form.notes = ''
    form.set_length_minutes = null
    form.status = 'inquiry'
    form.payment_status = 'unpaid'
  }
}

watch(() => props.open, (val) => {
  if (val) initForm()
})

watch(() => props.gig, () => {
  initForm()
})

function onVenueSelected(venue: Venue) {
  form.venue = venue
  if (venue.city) form.city = venue.city
  if (venue.country) form.country = venue.country
}

function onContactSelected(contact: Contact) {
  form.contact = contact
  form.promoter_name = contact.name
  form.promoter_email = contact.email || ''
  form.promoter_phone = contact.phone || ''
}

async function searchCopyFrom(q: string) {
  if (q.length < 2) {
    copyResults.value = []
    return
  }
  copyResults.value = await gigStore.fetchGigsAutocomplete(q)
}

async function copyFromGig(gig: Gig) {
  form.city = gig.city || ''
  form.country = gig.country || ''
  form.promoter_name = gig.promoter_name || ''
  form.promoter_email = gig.promoter_email || ''
  form.promoter_phone = gig.promoter_phone || ''
  showCopyFromDropdown.value = false
  copyQuery.value = ''
  copyResults.value = []
}

async function save() {
  if (form.status === 'cancelled' && props.gig?.status !== 'cancelled') {
    showCancelConfirm.value = true
    return
  }
  await doSave()
}

async function doSave() {
  saveError.value = null
  if (!form.date) {
    saveError.value = 'Date is required.'
    showCancelConfirm.value = false
    return
  }
  if (!form.venue?.name && !form.event_name) {
    saveError.value = 'Add a venue or an event name.'
    showCancelConfirm.value = false
    return
  }

  isSaving.value = true
  try {
    const gigData: GigCreate = {
      // The Go API decodes `date` as time.Time (RFC 3339 only); a bare
      // YYYY-MM-DD from the date input is rejected as invalid JSON.
      // Midnight UTC round-trips with the `split('T')[0]` used on read.
      date: `${form.date}T00:00:00Z`,
      venue: form.venue?.name || form.event_name || '',
      city: form.city,
      country: form.country,
      event_name: form.event_name,
      promoter_name: form.promoter_name,
      promoter_email: form.promoter_email,
      promoter_phone: form.promoter_phone,
      fee_amount: form.fee_amount || 0,
      fee_currency: form.fee_currency,
      set_length_minutes: form.set_length_minutes || undefined,
      notes: form.notes || undefined,
      status: form.status,
      payment_status: form.payment_status,
    }

    // The gig store swallows request errors and resolves to null, so a
    // falsy result is the only failure signal — keep the dialog open.
    // Updates are guarded by optimistic concurrency: the API matches on
    // the `updated_at` we last saw and answers 409 without it.
    const result = isEdit.value && props.gig?.id
      ? await gigStore.updateGig(props.gig.id, { ...gigData, updated_at: props.gig.updated_at })
      : await gigStore.createGig(gigData)

    if (!result) {
      saveError.value = 'Could not save the gig. Check the fields and try again.'
      return
    }

    emit('saved')
    emit('update:open', false)
  } catch (e) {
    console.error('save failed:', e)
  } finally {
    isSaving.value = false
    showCancelConfirm.value = false
  }
}

function close() {
  emit('update:open', false)
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-200"
      leave-active-class="transition-opacity duration-200"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open"
        style="position:fixed;inset:0;z-index:100;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.8);"
        @click.self="close"
      >
        <div
          class="glass"
          style="width:90%;max-width:520px;max-height:90vh;overflow-y:auto;background:var(--color-surface-container);"
        >
          <!-- Dialog header -->
          <div style="display:flex;justify-content:space-between;align-items:center;padding:12px 20px;border-bottom:1px solid color-mix(in srgb, var(--color-primary) 8%, transparent);">
            <div style="font-family:var(--font-command);font-size:13px;font-weight:700;letter-spacing:-.02em;text-transform:uppercase;">{{ title }}</div>
            <button class="btn-hud btn-hud-xs" @click="close">✕</button>
          </div>

          <!-- Form body -->
          <div style="padding:20px;display:flex;flex-direction:column;gap:14px;">
            <!-- Date -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">DATE</label>
              <input
                v-model="form.date"
                type="date"
                class="hud-input"
                style="width:100%;"
              >
            </div>

            <!-- Venue autocomplete -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">VENUE</label>
              <VenueAutocomplete
                :model-value="form.venue"
                placeholder="Search venue..."
                @venue-selected="onVenueSelected"
              />
            </div>

            <!-- City / Country -->
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;">
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">CITY</label>
                <input
                  v-model="form.city"
                  type="text"
                  class="hud-input"
                  style="width:100%;"
                  placeholder="Berlin"
                >
              </div>
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">COUNTRY</label>
                <input
                  v-model="form.country"
                  type="text"
                  class="hud-input"
                  style="width:100%;"
                  placeholder="DE"
                >
              </div>
            </div>

            <!-- Event name -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">EVENT NAME</label>
              <input
                v-model="form.event_name"
                type="text"
                class="hud-input"
                style="width:100%;"
                placeholder="Saturday Night"
              >
            </div>

            <!-- Room / Details -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">ROOM / DETAILS</label>
              <input
                v-model="form.room_details"
                type="text"
                class="hud-input"
                style="width:100%;"
                placeholder="Main Stage"
              >
            </div>

            <!-- Contact autocomplete -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">PROMOTER CONTACT</label>
              <ContactAutocomplete
                :model-value="form.contact"
                placeholder="Search contact..."
                @contact-selected="onContactSelected"
              />
            </div>

            <!-- Promoter fields -->
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;">
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">PROMOTER NAME</label>
                <input
                  v-model="form.promoter_name"
                  type="text"
                  class="hud-input"
                  style="width:100%;"
                >
              </div>
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">PROMOTER EMAIL</label>
                <input
                  v-model="form.promoter_email"
                  type="email"
                  class="hud-input"
                  style="width:100%;"
                >
              </div>
            </div>

            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">PROMOTER PHONE</label>
              <input
                v-model="form.promoter_phone"
                type="tel"
                class="hud-input"
                style="width:100%;"
              >
            </div>

            <!-- Fee + Currency -->
            <div style="display:grid;grid-template-columns:1fr 100px;gap:10px;">
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">FEE AMOUNT</label>
                <input
                  v-model.number="form.fee_amount"
                  type="number"
                  class="hud-input"
                  style="width:100%;"
                  placeholder="2000"
                >
              </div>
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">CURRENCY</label>
                <select v-model="form.fee_currency" class="hud-input" style="width:100%;">
                  <option v-for="c in CURRENCIES" :key="c" :value="c">{{ c }}</option>
                </select>
              </div>
            </div>

            <!-- Set length -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">SET LENGTH (MINUTES)</label>
              <input
                v-model.number="form.set_length_minutes"
                type="number"
                class="hud-input"
                style="width:100%;"
                placeholder="180"
              >
            </div>

            <!-- Status + Payment -->
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;">
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">STATUS</label>
                <select v-model="form.status" class="hud-input" style="width:100%;">
                  <option v-for="s in STATUS_OPTIONS" :key="s" :value="s">
                    {{ s.toUpperCase() }}
                  </option>
                </select>
              </div>
              <div>
                <label class="section-lbl" style="display:block;margin-bottom:6px;">PAYMENT</label>
                <select v-model="form.payment_status" class="hud-input" style="width:100%;">
                  <option v-for="p in PAYMENT_OPTIONS" :key="p" :value="p">
                    {{ p.replace(/_/g, ' ').toUpperCase() }}
                  </option>
                </select>
              </div>
            </div>

            <!-- Notes -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">NOTES</label>
              <textarea
                v-model="form.notes"
                class="hud-textarea"
                style="width:100%;min-height:80px;"
                placeholder="Additional notes..."
              />
            </div>

            <!-- Linked tracklists -->
            <div v-if="isEdit && props.gig?.id">
              <label class="section-lbl" style="display:block;margin-bottom:6px;">LINKED TRACKLISTS</label>
              <div v-for="tl in linkedTracklists" :key="tl.id" style="display:flex;align-items:center;gap:8px;padding:6px 10px;background:color-mix(in srgb, var(--color-primary) 6%, transparent);margin-bottom:4px;">
                <span style="flex:1;font-family:var(--font-ui);font-size:13px;color:var(--green);font-weight:600;">{{ tl.title }}</span>
                <button class="btn-hud btn-hud-xs" style="color:var(--red);" @click="unlinkTracklist(tl.id)" :disabled="unlinkingTl">✕</button>
              </div>
              <div v-if="linkedTracklists.length === 0" style="font-size:12px;color:var(--muted);padding:4px 0;">No tracklists linked yet.</div>
              <div style="margin-top:8px;">
                <label class="section-lbl" style="display:block;margin-bottom:4px;font-size:11px;color:var(--muted);">SELECT TRACKLIST TO LINK</label>
                <div style="display:flex;gap:6px;flex-wrap:wrap;">
                  <button
                    v-for="tl in availableTracklists"
                    :key="tl.id"
                    class="btn-hud"
                    style="font-size:12px;padding:4px 10px;"
                    :disabled="linkingTlId === tl.id"
                    @click="linkTracklist(tl.id)"
                  >
                    <span v-if="linkingTlId === tl.id">...</span>
                    <span v-else>{{ tl.title }}</span>
                  </button>
                </div>
              </div>
              <div v-if="!tracklistsLoaded" style="font-size:12px;color:var(--muted);padding:4px 0;">Loading tracklists...</div>
            </div>

            <!-- Copy from previous gig -->
            <div>
              <label class="section-lbl" style="display:block;margin-bottom:6px;">COPY FROM PREVIOUS GIG</label>
              <input
                v-model="copyQuery"
                type="text"
                class="hud-input"
                style="width:100%;"
                placeholder="Search gigs..."
                @input="searchCopyFrom(copyQuery)"
              >
              <div
                v-if="copyResults.length > 0"
                class="glass"
                style="margin-top:4px;max-height:120px;overflow-y:auto;"
              >
                <div
                  v-for="gig in copyResults"
                  :key="gig.id"
                  style="padding:8px 12px;cursor:pointer;border-bottom:1px solid rgba(46,46,49,.2);"
                  @click="copyFromGig(gig)"
                >
                  <div style="font-size:11px;font-weight:600;">{{ gig.event_name || gig.venue }}</div>
                  <div class="section-lbl" style="margin-top:2px;">{{ gig.date?.split('T')[0] }}</div>
                </div>
              </div>
            </div>
          </div>

          <!-- Dialog footer -->
          <div style="display:flex;justify-content:flex-end;align-items:center;gap:10px;padding:12px 20px;border-top:1px solid color-mix(in srgb, var(--color-primary) 8%, transparent);">
            <div
              v-if="saveError"
              role="alert"
              class="section-lbl"
              style="margin-right:auto;color:var(--color-error);"
            >
              {{ saveError }}
            </div>
            <button class="btn-hud" @click="close">
              CANCEL
            </button>
            <button
              class="btn-hud btn-hud-cta"
              :disabled="isSaving"
              @click="save"
            >
              {{ isSaving ? 'SAVING...' : 'SAVE' }}
            </button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Cancel confirmation dialog -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition-opacity duration-200"
        leave-active-class="transition-opacity duration-200"
        enter-from-class="opacity-0"
        leave-to-class="opacity-0"
      >
        <div
          v-if="showCancelConfirm"
          style="position:fixed;inset:0;z-index:200;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.8);"
        >
          <div class="glass" style="padding:24px;max-width:360px;text-align:center;background:var(--color-surface-container);">
            <div
              style="font-family:var(--font-command);font-size:12px;font-weight:700;margin-bottom:12px;letter-spacing:-.02em;text-transform:uppercase;"
            >
              CONFIRM CANCELLATION
            </div>
            <div style="font-size:11px;color:var(--color-tertiary);margin-bottom:20px;">
              Cancelling a gig is permanent and cannot be undone. Are you sure?
            </div>
            <div style="display:flex;justify-content:center;gap:10px;">
              <button class="btn-hud" @click="showCancelConfirm = false">
                GO BACK
              </button>
              <button
                class="btn-hud btn-hud-error"
                @click="doSave"
              >
                CONFIRM CANCEL
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </Teleport>
</template>
